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

type chatCompletionChoiceMessage struct {
	Role             string     `json:"role"`
	Content          string     `json:"content"`
	ReasoningContent string     `json:"reasoning_content"`
	Refusal          string     `json:"refusal"`
	ToolCalls        []toolCall `json:"tool_calls,omitempty"`
	ExtraContent     any        `json:"extra_content,omitempty"`
}

type chatCompletionRequest struct {
	Model           string          `json:"model"`
	Messages        []openAIMessage `json:"messages"`
	Tools           []chatTool      `json:"tools,omitempty"`
	ToolChoice      any             `json:"tool_choice,omitempty"`
	Thinking        *ThinkingConfig `json:"thinking,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	PresencePenalty *float64        `json:"presence_penalty,omitempty"`
}

func isOwnerInquiry(text string) bool {
	lower := strings.ToLower(text)
	kw := []string{
		"和谁聊", "跟谁聊", "谁找过你", "谁找你", "聊过天", "聊天记录", "记录",
		"查岗", "仔细说说", "前天呢", "昨天呢", "今天呢", "大前天", "跟谁在聊",
		"搜一下", "搜搜索", "谁提过", "谁说的", "聊啥了", "聊了啥", "翻翻后台", "看记录",
		"谁跟你", "跟你说", "跟你聊", "查你", "查查", "清白", "交代", "跟别人聊",
		"聊什么", "聊了什么", "聊过什么", "说的什么", "说了什么", "说了啥", "聊过", "说话", "聊天", "对话", "找你", "联系",
	}
	for _, k := range kw {
		if strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

func isMiMo(model, baseURL string) bool {
	m := strings.ToLower(model)
	u := strings.ToLower(baseURL)
	return strings.Contains(m, "mimo") || strings.Contains(u, "xiaomi")
}

func isGoogleGemini(model, baseURL string) bool {
	m := strings.ToLower(model)
	u := strings.ToLower(baseURL)
	return strings.Contains(u, "googleapis.com") || strings.Contains(m, "gemini") || strings.Contains(m, "gemma")
}

type chatCompletionResponse struct {
	Choices []struct {
		FinishReason string                      `json:"finish_reason"`
		Message      chatCompletionChoiceMessage `json:"message"`
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

// resolveFallbackProvider 获取备用 provider（用于主通道风控或异常时无缝热备切换）
func (p *AiPlugin) resolveFallbackProvider(sessionKey string) (*Provider, error) {
	config := p.configSnapshot()
	name := strings.TrimSpace(config.FallbackProvider)
	if name == "" {
		return nil, errors.New("未配置 fallback provider")
	}
	prov, ok := config.Providers[name]
	if !ok || prov == nil {
		return nil, fmt.Errorf("fallback provider 不存在：%s", name)
	}
	if strings.TrimSpace(prov.BaseURL) == "" {
		return nil, fmt.Errorf("fallback provider %s 缺少 base_url", name)
	}
	if strings.TrimSpace(prov.APIKey) == "" {
		return nil, fmt.Errorf("fallback provider %s 缺少 api_key", name)
	}
	if strings.TrimSpace(prov.Model) == "" {
		return nil, fmt.Errorf("fallback provider %s 缺少 model", name)
	}
	return prov, nil
}

func (p *AiPlugin) chat(sessionKey string) (string, error) {
	prov, err := p.resolveProvider(sessionKey)
	if err != nil {
		return "", err
	}

	reply, err := p.chatWithProvider(sessionKey, prov)
	// 如果主力通道失败或返回风控拦截（如 high risk），尝试无缝切换到备用 provider (Plan B)
	if err != nil || reply == "" || isLeakedReasoningOrRefusal(reply) {
		fallbackProv, fbErr := p.resolveFallbackProvider(sessionKey)
		if fbErr == nil && fallbackProv != nil {
			slog.Info("[ai] 主力通道触发风控或候选模型均不可用，自动切换到备用 provider 请求", "fallback_provider_model", fallbackProv.Model, "session", sessionKey)
			fbReply, fbChatErr := p.chatWithProvider(sessionKey, fallbackProv)
			if fbChatErr == nil && strings.TrimSpace(fbReply) != "" && !isLeakedReasoningOrRefusal(fbReply) {
				return strings.TrimSpace(fbReply), nil
			}
			slog.Warn("[ai] 备用 provider 请求亦未成功", "err", fbChatErr)
		}
	}

	if err != nil {
		// 如果是因为风控拦截或敏感话题受限/配额耗尽，兜底使用沉浸式亲密回复，绝不报错破坏体验
		errMsg := err.Error()
		if isLeakedReasoningOrRefusal(errMsg) || strings.Contains(errMsg, "high risk") || strings.Contains(errMsg, "429") || strings.Contains(errMsg, "RESOURCE_EXHAUSTED") {
			lastText := ""
			isImg := false
			if ctxMsgs := p.contextMessages(sessionKey); len(ctxMsgs) > 0 {
				lastMsg := ctxMsgs[len(ctxMsgs)-1]
				if s, ok := lastMsg.Content.(string); ok {
					lastText = s
				} else if parts, ok := lastMsg.Content.([]contentPart); ok {
					for _, part := range parts {
						if part.Type == "text" {
							lastText = part.Text
						} else if part.Type == "image_url" {
							isImg = true
						}
					}
				}
			}
			slog.Warn("[ai] 触发风控或配额耗尽且通道不可用，使用拟人化智能兜底", "err", err, "is_image", isImg, "text", lastText)
			return getFallbackReply(lastText, isImg), nil
		}
		return "", err
	}

	return reply, nil
}

func (p *AiPlugin) chatWithProvider(sessionKey string, prov *Provider) (string, error) {
	config := p.configSnapshot()
	messages := make([]openAIMessage, 0, p.getMaxContextMessages(sessionKey)+1)
	activePrompt := p.getActivePrompt(sessionKey)
	if prompt, ok := config.Prompts[activePrompt]; ok && strings.TrimSpace(prompt) != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: prompt + p.getPreMadePrompts(sessionKey)})
	}

	rawContext := p.contextMessages(sessionKey)
	// 平滑转译：将直白动作敏感词转为富有感官情调的文学表达，避开云端违规审查
	for _, m := range rawContext {
		if s, ok := m.Content.(string); ok {
			m.Content = softenHighRiskTerms(s)
		}
		messages = append(messages, m)
	}
	if len(messages) == 0 {
		return "", errors.New("AI 上下文为空")
	}

	isOwnerPrivate := p.isOwnerPrivateSession(sessionKey)
	var tools []chatTool
	if isOwnerPrivate {
		tools = p.getOwnerTools()
		slog.Info("[ai] 管理员专属私聊会话已挂载情报工具与简报", "session", sessionKey, "tools_count", len(tools))
	}

	timeout := prov.HTTPTimeoutSeconds
	if timeout <= 0 {
		timeout = config.HTTPTimeoutSeconds
	}
	if timeout <= 0 {
		timeout = 60
	}

	temp := 0.88
	if prov.Temperature != nil {
		temp = *prov.Temperature
	}
	var penalty *float64
	if prov.PresencePenalty != nil {
		penalty = prov.PresencePenalty
	} else {
		defaultPenalty := 0.35
		penalty = &defaultPenalty
	}

	candidateModels := []string{prov.Model}
	for _, m := range prov.FallbackModels {
		m = strings.TrimSpace(m)
		if m != "" && m != prov.Model {
			candidateModels = append(candidateModels, m)
		}
	}

	var lastErr error
	for idx, modelName := range candidateModels {
		currentMessages := append([]openAIMessage(nil), messages...)
		var finalReply string
		var runErr error

		// 支持最多 3 轮工具调用闭环
		for step := 0; step < 4; step++ {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			var reqPenalty *float64
			if !isGoogleGemini(modelName, prov.BaseURL) {
				reqPenalty = penalty
			}

			// 查找最后一条用户发言，判断是否为管理员打听情报/查岗
			lastUserText := ""
			for i := len(currentMessages) - 1; i >= 0; i-- {
				if currentMessages[i].Role == "user" {
					if s, ok := currentMessages[i].Content.(string); ok {
						lastUserText = s
						break
					}
				}
			}

			reqPayload := chatCompletionRequest{
				Model:           modelName,
				Messages:        currentMessages,
				Tools:           tools,
				Temperature:     &temp,
				PresencePenalty: reqPenalty,
			}
			// 当管理员在私聊中明确打听/查岗且为首轮请求时，强制要求模型调用工具核查真实数据
			if isOwnerPrivate && step == 0 && len(tools) > 0 && isOwnerInquiry(lastUserText) {
				reqPayload.ToolChoice = "required"
			}
			// 日常闲聊对话默认关闭思维链，获得极速响应体验
			if isMiMo(modelName, prov.BaseURL) {
				reqPayload.Thinking = &ThinkingConfig{Type: "disabled"}
			}

			choiceMsg, err := callOpenAI(ctx, http.DefaultClient, prov.BaseURL, prov.APIKey, reqPayload)
			cancel()

			if err != nil {
				runErr = err
				break
			}

			// 如果模型发起了工具调用
			if len(choiceMsg.ToolCalls) > 0 {
				slog.Info("[ai] 模型发起管理员专属情报工具调用", "tools_count", len(choiceMsg.ToolCalls), "model", modelName, "session", sessionKey)
				currentMessages = append(currentMessages, openAIMessage{
					Role:         "assistant",
					Content:      choiceMsg.Content,
					ToolCalls:    choiceMsg.ToolCalls,
					ExtraContent: choiceMsg.ExtraContent,
				})
				for _, tc := range choiceMsg.ToolCalls {
					slog.Info("[ai] 正在执行情报工具", "name", tc.Function.Name, "args", tc.Function.Arguments)
					res := p.executeTool(tc.Function.Name, tc.Function.Arguments)
					currentMessages = append(currentMessages, openAIMessage{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content:    res,
					})
				}
				continue
			}

			finalReply = choiceMsg.Content
			break
		}

		cleanCandidate := stripThinkingContent(finalReply)
		if runErr == nil && strings.TrimSpace(cleanCandidate) != "" && !isLeakedReasoningOrRefusal(cleanCandidate) {
			if idx > 0 {
				slog.Info("[ai] 主模型超限或异常，备用模型调用成功", "primary_model", prov.Model, "used_model", modelName, "session", sessionKey)
			}
			return finalReply, nil
		}

		lastErr = runErr
		if lastErr == nil {
			lastErr = errors.New("模型返回内容异常或触发过滤")
		}
		slog.Warn("[ai] 模型调用失败或受限，尝试下一个候选模型",
			"provider_model", modelName,
			"session", sessionKey,
			"attempt", idx+1,
			"total", len(candidateModels),
			"err", lastErr,
		)
	}

	return "", fmt.Errorf("Provider 所有候选模型均调用失败: %w", lastErr)
}

func callOpenAI(ctx context.Context, client *http.Client, baseURL, apiKey string, payload chatCompletionRequest) (*chatCompletionChoiceMessage, error) {
	if client == nil {
		client = http.DefaultClient
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化 AI 请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionURL(baseURL), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建 AI 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36 Edg/141.0.0.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 AI 接口失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 AI 响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errObj struct {
			Error *struct {
				Message string `json:"message"`
				Code    any    `json:"code"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal(body, &errObj); err == nil && errObj.Error != nil && errObj.Error.Message != "" {
			return nil, fmt.Errorf("AI 接口返回错误 (状态码 %d): %s", resp.StatusCode, errObj.Error.Message)
		}
		var errArr []struct {
			Error *struct {
				Message string `json:"message"`
				Code    any    `json:"code"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal(body, &errArr); err == nil && len(errArr) > 0 && errArr[0].Error != nil && errArr[0].Error.Message != "" {
			return nil, fmt.Errorf("AI 接口返回错误 (状态码 %d): %s", resp.StatusCode, errArr[0].Error.Message)
		}
		return nil, fmt.Errorf("AI 接口返回状态码: %d, body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result chatCompletionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		slog.Warn("[ai] 解析 AI 成功响应失败", "raw_body", string(body), "status", resp.StatusCode)
		return nil, fmt.Errorf("解析 AI 响应失败: %w", err)
	}
	if result.Error != nil && result.Error.Message != "" {
		return nil, fmt.Errorf("AI 接口返回错误: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return nil, errors.New("AI 响应缺少 choices")
	}

	choice := result.Choices[0]
	if choice.Message.Refusal != "" {
		return nil, fmt.Errorf("模型拒绝回答 (API Refusal): %s", choice.Message.Refusal)
	}
	if choice.FinishReason == "content_filter" {
		return nil, errors.New("模型触发内容安全拦截 (finish_reason=content_filter)")
	}

	// 如果模型调用了工具，直接返回 choice.Message（可能此时 Content 为空）
	if len(choice.Message.ToolCalls) > 0 {
		return &choice.Message, nil
	}

	content := strings.TrimSpace(choice.Message.Content)
	if content == "" {
		return nil, errors.New("模型返回空内容")
	}

	// 剥离思维链思考过程
	content = stripThinkingContent(content)

	// 检测剥离后是否为空或是否包含思维链泄漏、提示词泄漏或 API 安全拦截提示
	if content == "" || isLeakedReasoningOrRefusal(content) {
		return nil, fmt.Errorf("模型输出触发思维链/提示词泄漏或安全拦截: %s", choice.Message.Content)
	}

	choice.Message.Content = content
	return &choice.Message, nil
}

func chatCompletionURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}
