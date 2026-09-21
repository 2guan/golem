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
					FinishReason string `json:"finish_reason"`
					Message      struct {
						Role             string `json:"role"`
						Content          string `json:"content"`
						ReasoningContent string `json:"reasoning_content"`
						Refusal          string `json:"refusal"`
					} `json:"message"`
				}{
					{
						FinishReason: "stop",
						Message: struct {
							Role             string `json:"role"`
							Content          string `json:"content"`
							ReasoningContent string `json:"reasoning_content"`
							Refusal          string `json:"refusal"`
						}{
							Role:    "assistant",
							Content: "你好，我是肉丸",
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
			"default": "你是肉丸",
		},
		ActivePrompt:        "default",
		HTTPTimeoutSeconds: 5,
	}

	reply, err := plugin.chatWithProvider("test-session", prov)
	if err != nil {
		t.Fatalf("chatWithProvider failed: %v", err)
	}
	if reply != "你好，我是肉丸" {
		t.Errorf("expected '你好，我是肉丸', got '%s'", reply)
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
