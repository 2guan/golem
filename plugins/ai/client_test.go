package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
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

func TestResolveProvidersInOrder(t *testing.T) {
	ai := &AiPlugin{
		sessions: map[string][]openAIMessage{},
	}
	ai.Config = Config{
		Providers: map[string]*Provider{
			"p1": {BaseURL: "https://p1.com", APIKey: "k1", Model: "m1"},
			"p2": {BaseURL: "https://p2.com", APIKey: "k2", Model: "m2"},
			"p3": {BaseURL: "https://p3.com", APIKey: "k3", Model: "m3"},
		},
		ProviderOrder: []string{"p3", "p1", "p2"},
	}

	// 1. 全局顺序
	provs := ai.resolveProvidersInOrder("normal-session")
	if len(provs) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(provs))
	}
	if provs[0].Model != "m3" || provs[1].Model != "m1" || provs[2].Model != "m2" {
		t.Errorf("order mismatch: got [%s, %s, %s]", provs[0].Model, provs[1].Model, provs[2].Model)
	}

	// 2. 会话级专属覆盖，将指定 provider 置顶
	sessionKey := "custom-session"
	p2Name := "p2"
	ai.Config.SessionConfigs = map[string]*SessionConfig{
		sessionKey: {
			ActiveProvider: &p2Name,
		},
	}
	provs = ai.resolveProvidersInOrder(sessionKey)
	if len(provs) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(provs))
	}
	if provs[0].Model != "m2" {
		t.Errorf("expected session override model m2 at first rank, got %s", provs[0].Model)
	}
}

func TestChatCompletionURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "http://127.0.0.1:8000/",
			expected: "http://127.0.0.1:8000/v1/chat/completions",
		},
		{
			input:    "http://127.0.0.1:8000",
			expected: "http://127.0.0.1:8000/v1/chat/completions",
		},
		{
			input:    "http://127.0.0.1:8000/v1",
			expected: "http://127.0.0.1:8000/v1/chat/completions",
		},
		{
			input:    "http://127.0.0.1:8000/v1/",
			expected: "http://127.0.0.1:8000/v1/chat/completions",
		},
		{
			input:    "https://api.openai.com/v1",
			expected: "https://api.openai.com/v1/chat/completions",
		},
		{
			input:    "https://generativelanguage.googleapis.com/v1beta/openai",
			expected: "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
		},
		{
			input:    "http://127.0.0.1:8000/v1/chat/completions",
			expected: "http://127.0.0.1:8000/v1/chat/completions",
		},
	}

	for _, tt := range tests {
		actual := chatCompletionURL(tt.input)
		if actual != tt.expected {
			t.Errorf("chatCompletionURL(%q) = %q; expected %q", tt.input, actual, tt.expected)
		}
	}
}

func TestResolveNamedProvidersInOrder_GrokXiaomiGemini(t *testing.T) {
	ai := &AiPlugin{
		sessions: map[string][]openAIMessage{},
	}
	ai.Config = Config{
		ActiveProvider:   "grok",
		FallbackProvider: "xiaomi",
		ProviderOrder:    []string{"grok", "xiaomi", "gemini"},
		Providers: map[string]*Provider{
			"grok": {
				BaseURL: "http://127.0.0.1:8000/",
				APIKey:  "g2a_xxx",
				Model:   "grok-4.7",
			},
			"xiaomi": {
				BaseURL: "https://token-plan-cn.xiaomimimo.com/v1",
				APIKey:  "tp-xxx",
				Model:   "mimo-v2.6-flash",
			},
			"gemini": {
				BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai",
				APIKey:  "AQ.xxx",
				Model:   "gemini-3.5-flash-lite",
			},
		},
	}

	named := ai.resolveNamedProvidersInOrder("test-session")
	if len(named) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(named))
	}
	if named[0].Name != "grok" || named[0].Model != "grok-4.7" {
		t.Errorf("1st provider must be grok, got %s / %s", named[0].Name, named[0].Model)
	}
	if named[1].Name != "xiaomi" || named[1].Model != "mimo-v2.6-flash" {
		t.Errorf("2nd provider must be xiaomi, got %s / %s", named[1].Name, named[1].Model)
	}
	if named[2].Name != "gemini" || named[2].Model != "gemini-3.5-flash-lite" {
		t.Errorf("3rd provider must be gemini, got %s / %s", named[2].Name, named[2].Model)
	}
}

func TestChat_FallbackThreeTierGrokXiaomiGemini(t *testing.T) {
	var grokCalls, xiaomiCalls, geminiCalls int
	var mu sync.Mutex

	tsGrok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		grokCalls++
		mu.Unlock()
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"grok upstream error"}}`))
	}))
	defer tsGrok.Close()

	tsXiaomi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		xiaomiCalls++
		mu.Unlock()
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"high risk"}}`))
	}))
	defer tsXiaomi.Close()

	tsGemini := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		geminiCalls++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(chatCompletionResponse{
			Choices: []struct {
				FinishReason string                      `json:"finish_reason"`
				Message      chatCompletionChoiceMessage `json:"message"`
			}{
				{
					FinishReason: "stop",
					Message: chatCompletionChoiceMessage{
						Role:    "assistant",
						Content: "gemini兜底回复成功",
					},
				},
			},
		})
	}))
	defer tsGemini.Close()

	ai := &AiPlugin{
		sessions: map[string][]openAIMessage{
			"test-chain": {
				{Role: "user", Content: "你好"},
			},
		},
		sessionTimes: map[string]time.Time{},
	}
	ai.Config = Config{
		ActiveProvider:   "grok",
		FallbackProvider: "xiaomi",
		ProviderOrder:    []string{"grok", "xiaomi", "gemini"},
		Providers: map[string]*Provider{
			"grok": {
				BaseURL: tsGrok.URL,
				APIKey:  "grok-key",
				Model:   "grok-4.7",
			},
			"xiaomi": {
				BaseURL: tsXiaomi.URL,
				APIKey:  "xiaomi-key",
				Model:   "mimo-v2.6-flash",
			},
			"gemini": {
				BaseURL: tsGemini.URL,
				APIKey:  "gemini-key",
				Model:   "gemini-3.5-flash-lite",
			},
		},
	}

	reply, err := ai.chat("test-chain")
	if err != nil {
		t.Fatalf("expected fallback to gemini to succeed, got err: %v", err)
	}
	if reply != "gemini兜底回复成功" {
		t.Errorf("expected 'gemini兜底回复成功', got %q", reply)
	}
	if grokCalls != 1 {
		t.Errorf("expected grok called 1 time, got %d", grokCalls)
	}
	if xiaomiCalls != 1 {
		t.Errorf("expected xiaomi called 1 time, got %d", xiaomiCalls)
	}
	if geminiCalls != 1 {
		t.Errorf("expected gemini called 1 time, got %d", geminiCalls)
	}
}




