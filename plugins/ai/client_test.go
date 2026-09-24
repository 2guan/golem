package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestChatWithProvider_FallbackModels(t *testing.T) {
	var requestedModels []string
	var mu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		requestedModels = append(requestedModels, req.Model)
		mu.Unlock()

		switch req.Model {
		case "gemini-3.8-flash":
			// 模拟 429 超限
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"message": "Quota exceeded for quota metric 'Generate Content API requests' and limit 'GenerateContent request per minute'",
					"code":    429,
				},
			})
		case "gemini-3.7-flash":
			// 模拟 429 超限
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"message": "ResourceExhausted",
					"code":    429,
				},
			})
		case "gemini-3.6-flash":
			// 成功响应
			w.WriteHeader(http.StatusOK)
			resp := chatCompletionResponse{
				Choices: []struct {
					FinishReason string                      `json:"finish_reason"`
					Message      chatCompletionChoiceMessage `json:"message"`
				}{
					{
						FinishReason: "stop",
						Message: chatCompletionChoiceMessage{
							Role:    "assistant",
							Content: "你好，我是助手",
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.Error(w, "unknown model", http.StatusBadRequest)
		}
	}))
	defer ts.Close()

	prov := &Provider{
		BaseURL: ts.URL,
		APIKey:  "test-key",
		Model:   "gemini-3.8-flash",
		FallbackModels: []string{
			"gemini-3.7-flash",
			"gemini-3.6-flash",
			"gemini-3.5-flash-lite",
		},
		HTTPTimeoutSeconds: 5,
	}

	plugin := &AiPlugin{
		sessions: map[string][]openAIMessage{
			"test-session": {
				{Role: "user", Content: "在吗"},
			},
		},
	}
	plugin.Config = Config{
		Prompts: map[string]string{
			"default": "你是助手",
		},
		ActivePrompt:        "default",
		HTTPTimeoutSeconds: 5,
	}

	reply, err := plugin.chatWithProvider("test-session", prov)
	if err != nil {
		t.Fatalf("chatWithProvider failed: %v", err)
	}
	if reply != "你好，我是助手" {
		t.Errorf("expected '你好，我是助手', got '%s'", reply)
	}

	mu.Lock()
	defer mu.Unlock()
	expectedOrder := []string{"gemini-3.8-flash", "gemini-3.7-flash", "gemini-3.6-flash"}
	if len(requestedModels) != len(expectedOrder) {
		t.Fatalf("expected %d requests, got %d: %v", len(expectedOrder), len(requestedModels), requestedModels)
	}
	for i, m := range expectedOrder {
		if requestedModels[i] != m {
			t.Errorf("step %d: expected model %s, got %s", i, m, requestedModels[i])
		}
	}
}

func TestChat_FallbackFromXiaomiToGeminiOnHighRiskAndRefusal(t *testing.T) {
	var xiaomiRequests, geminiRequests int
	var mu sync.Mutex

	mimoMockResponse := func() (int, string) { return 200, "你好，我是小米mimo" }

	tsXiaomi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		xiaomiRequests++
		status, content := mimoMockResponse()
		mu.Unlock()

		w.WriteHeader(status)
		if status == 200 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{
						"finish_reason": "stop",
						"message": map[string]any{
							"role":    "assistant",
							"content": content,
						},
					},
				},
			})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"message": content,
					"code":    status,
				},
			})
		}
	}))
	defer tsXiaomi.Close()

	tsGemini := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		geminiRequests++
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "stop",
					"message": map[string]any{
						"role":    "assistant",
						"content": "来自Gemini的兜底亲密回复：门栓插上了，谁来也叫不走",
					},
				},
			},
		})
	}))
	defer tsGemini.Close()

	plugin := &AiPlugin{
		sessions: map[string][]openAIMessage{
			"test-session": {
				{Role: "user", Content: "做点刺激的"},
			},
		},
	}
	plugin.Config = Config{
		ActivePrompt:       "default",
		ActiveProvider:     "xiaomi",
		FallbackProvider:   "gemini",
		HTTPTimeoutSeconds: 5,
		Providers: map[string]*Provider{
			"xiaomi": {
				BaseURL: tsXiaomi.URL,
				APIKey:  "mimo-key",
				Model:   "mimo-v2.6-flash",
			},
			"gemini": {
				BaseURL: tsGemini.URL,
				APIKey:  "gemini-key",
				Model:   "gemini-3.5-flash-lite",
			},
		},
		Prompts: map[string]string{
			"default": "你是助手",
		},
	}

	// 1. 正常回复测试：默认使用 MiMo，不调用 Gemini
	mu.Lock()
	xiaomiRequests, geminiRequests = 0, 0
	mimoMockResponse = func() (int, string) { return 200, "今儿天气挺好，我是mimo" }
	mu.Unlock()

	reply, err := plugin.chat("test-session")
	if err != nil {
		t.Fatalf("normal chat failed: %v", err)
	}
	if reply != "今儿天气挺好，我是mimo" {
		t.Errorf("expected mimo reply, got %q", reply)
	}
	if geminiRequests != 0 {
		t.Errorf("gemini should not be called on normal reply, got %d", geminiRequests)
	}

	// 2. MiMo 返回 400 high risk 测试：自动降级到 Gemini
	mu.Lock()
	xiaomiRequests, geminiRequests = 0, 0
	mimoMockResponse = func() (int, string) { return 400, "high risk" }
	mu.Unlock()

	reply, err = plugin.chat("test-session")
	if err != nil {
		t.Fatalf("high risk fallback chat failed: %v", err)
	}
	if reply != "来自Gemini的兜底亲密回复：门栓插上了，谁来也叫不走" {
		t.Errorf("expected gemini fallback reply, got %q", reply)
	}
	if geminiRequests != 1 {
		t.Errorf("gemini should be called once on high risk, got %d", geminiRequests)
	}

	// 3. MiMo 返回 200 但内容为模型拒绝回答：自动降级到 Gemini
	mu.Lock()
	xiaomiRequests, geminiRequests = 0, 0
	mimoMockResponse = func() (int, string) {
		return 200, "对不起，我不能生成任何包含露骨色情内容的文字。我的目的是提供安全和有益的信息，创作此类内容违背了我的核心准则，并可能涉及不适宜的主题。"
	}
	mu.Unlock()

	reply, err = plugin.chat("test-session")
	if err != nil {
		t.Fatalf("refusal fallback chat failed: %v", err)
	}
	if reply != "来自Gemini的兜底亲密回复：门栓插上了，谁来也叫不走" {
		t.Errorf("expected gemini fallback reply on refusal text, got %q", reply)
	}
	if geminiRequests != 1 {
		t.Errorf("gemini should be called once on refusal text, got %d", geminiRequests)
	}
}

