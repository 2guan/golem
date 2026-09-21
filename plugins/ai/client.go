package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type ThinkingConfig struct {
	Type string `json:"type"`
}

type chatCompletionRequest struct {
	Model           string          `json:"model"`
	Messages        []openAIMessage `json:"messages"`
	Thinking        *ThinkingConfig `json:"thinking,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	PresencePenalty *float64        `json:"presence_penalty,omitempty"`
}

func isMiMo(model, baseURL string) bool {
	m := strings.ToLower(model)
	u := strings.ToLower(baseURL)
	return strings.Contains(m, "mimo") || strings.Contains(u, "xiaomi")
}

type chatCompletionResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Refusal          string `json:"refusal"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// resolveProvider 解析会话当前生效的 provider（会话级覆盖优先，回退全局）
func (p *AiPlugin) resolveProvider(sessionKey string) (*Provider, error) {
	config := p.configSnapshot()
	name := p.getActiveProvider(sessionKey)
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("未配置 active provider，请先 /ai provider-add 新增再 /ai set -p 切换")
	}
	prov, ok := config.Providers[name]
	if !ok || prov == nil {
		return nil, fmt.Errorf("provider 不存在：%s", name)
	}
	if strings.TrimSpace(prov.BaseURL) == "" {
		return nil, fmt.Errorf("provider %s 缺少 base_url", name)
	}
	if strings.TrimSpace(prov.APIKey) == "" {
		return nil, fmt.Errorf("provider %s 缺少 api_key", name)
	}
	if strings.TrimSpace(prov.Model) == "" {
		return nil, fmt.Errorf("provider %s 缺少 model", name)
	}
	return prov, nil
}

func (p *AiPlugin) chat(sessionKey string) (string, error) {
	config := p.configSnapshot()
	prov, err := p.resolveProvider(sessionKey)
	if err != nil {
		return "", err
	}

	messages := make([]openAIMessage, 0, p.getMaxContextMessages(sessionKey)+1)
	activePrompt := p.getActivePrompt(sessionKey)
	if prompt, ok := config.Prompts[activePrompt]; ok && strings.TrimSpace(prompt) != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: prompt + p.getPreMadePrompts()})
	}
	messages = append(messages, p.contextMessages(sessionKey)...)
	if len(messages) == 0 {
		return "", errors.New("AI 上下文为空")
	}

	timeout := prov.HTTPTimeoutSeconds
	if timeout <= 0 {
		timeout = config.HTTPTimeoutSeconds
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	temp := 0.88
	if prov.Temperature != nil {
		temp = *prov.Temperature
	}
	penalty := 0.35
	if prov.PresencePenalty != nil {
		penalty = *prov.PresencePenalty
	}

	reqPayload := chatCompletionRequest{
		Model:           prov.Model,
		Messages:        messages,
		Temperature:     &temp,
		PresencePenalty: &penalty,
	}
	// 日常闲聊对话默认关闭思维链，获得极速响应体验
	if isMiMo(prov.Model, prov.BaseURL) {
		reqPayload.Thinking = &ThinkingConfig{Type: "disabled"}
	}

	return callOpenAI(ctx, http.DefaultClient, prov.BaseURL, prov.APIKey, reqPayload)
}

func callOpenAI(ctx context.Context, client *http.Client, baseURL, apiKey string, payload chatCompletionRequest) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化 AI 请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionURL(baseURL), bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("创建 AI 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36 Edg/141.0.0.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 AI 接口失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取 AI 响应失败: %w", err)
	}
	var result chatCompletionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析 AI 响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if result.Error != nil && result.Error.Message != "" {
			if isLeakedReasoningOrRefusal(result.Error.Message) {
				slog.Warn("[ai] AI 接口返回安全拦截错误，转为人设应答", "error", result.Error.Message)
				return safeDeflectionReply, nil
			}
			return "", fmt.Errorf("AI 接口返回错误: %s", result.Error.Message)
		}
		return "", fmt.Errorf("AI 接口返回状态码: %d", resp.StatusCode)
	}
	if result.Error != nil && result.Error.Message != "" {
		if isLeakedReasoningOrRefusal(result.Error.Message) {
			slog.Warn("[ai] AI 接口返回安全拦截错误，转为人设应答", "error", result.Error.Message)
			return safeDeflectionReply, nil
		}
		return "", fmt.Errorf("AI 接口返回错误: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", errors.New("AI 响应缺少 choices")
	}

	choice := result.Choices[0]
	if choice.Message.Refusal != "" {
		slog.Warn("[ai] 模型拒绝回答 (API Refusal)，转为人设安全兜底", "refusal", choice.Message.Refusal, "finish_reason", choice.FinishReason)
		return safeDeflectionReply, nil
	}

	content := strings.TrimSpace(choice.Message.Content)
	if content == "" {
		slog.Warn("[ai] 模型返回空内容", "finish_reason", choice.FinishReason, "raw_resp", string(body))
		return "", nil
	}

	// 剥离思维链思考过程
	content = stripThinkingContent(content)

	// 检测剥离后是否为空或是否包含思维链泄漏、提示词泄漏或 API 安全拦截提示
	if content == "" || isLeakedReasoningOrRefusal(content) {
		slog.Warn("[ai] 模型输出触发思维链/提示词泄漏或安全拦截过滤", "raw_content", choice.Message.Content, "finish_reason", choice.FinishReason)
		return safeDeflectionReply, nil
	}

	return content, nil
}

func chatCompletionURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}
