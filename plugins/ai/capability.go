package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// aiChatPayload ai.chat 能力的请求载荷：调用方提供完整消息列表（可选 system 提示词），
// 插件只负责透传给 LLM，不带入任何会话上下文或预置提示词，以便其他插件复用 LLM 能力。
type aiChatPayload struct {
	System         string          `json:"system"`
	Messages       []openAIMessage `json:"messages"`
	TimeoutSeconds int             `json:"timeout_seconds,omitempty"`
	Thinking       *bool           `json:"thinking,omitempty"`
}

// GetCapabilities 声明本插件可被其他插件调用的能力
func (p *AiPlugin) GetCapabilities() []string {
	return []string{"ai.chat"}
}

// OnCall 处理其他插件的调用请求
func (p *AiPlugin) OnCall(capability string, args map[string]string) (string, []byte, error) {
	switch capability {
	case "ai.chat":
		raw, ok := args["payload"]
		if !ok || strings.TrimSpace(raw) == "" {
			return "", nil, fmt.Errorf("ai.chat 缺少 payload")
		}
		var req aiChatPayload
		if err := json.Unmarshal([]byte(raw), &req); err != nil {
			return "", nil, fmt.Errorf("ai.chat payload 解析失败: %w", err)
		}
		if len(req.Messages) == 0 {
			return "", nil, fmt.Errorf("ai.chat 消息为空")
		}
		reply, err := p.chatRaw(req.System, req.Messages, req.TimeoutSeconds, req.Thinking)
		if err != nil {
			return "", nil, err
		}
		return "text", []byte(reply), nil
	default:
		return "", nil, fmt.Errorf("不支持的能力: %s", capability)
	}
}

// chatRaw 直接以给定消息调用 LLM（供跨插件调用），不带入会话上下文与预置提示词
func (p *AiPlugin) chatRaw(system string, messages []openAIMessage, customTimeout int, thinking *bool) (string, error) {
	config := p.configSnapshot()
	prov, err := p.resolveProvider("")
	if err != nil {
		return "", err
	}

	full := make([]openAIMessage, 0, len(messages)+1)
	if strings.TrimSpace(system) != "" {
		full = append(full, openAIMessage{Role: "system", Content: strings.TrimSpace(system)})
	}
	full = append(full, messages...)

	timeout := customTimeout
	if timeout <= 0 {
		timeout = prov.HTTPTimeoutSeconds
	}
	if timeout <= 0 {
		timeout = config.HTTPTimeoutSeconds
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	reqPayload := chatCompletionRequest{
		Model:    prov.Model,
		Messages: full,
	}

	// 处理思维链控制
	if thinking != nil {
		if *thinking {
			reqPayload.Thinking = &ThinkingConfig{Type: "enabled"}
		} else {
			reqPayload.Thinking = &ThinkingConfig{Type: "disabled"}
		}
	} else if isMiMo(prov.Model, prov.BaseURL) {
		// 未指定时，默认关闭思维链以保证极速响应
		reqPayload.Thinking = &ThinkingConfig{Type: "disabled"}
	}

	slog.Debug("[ai] ai.chat 被调用", "messages", len(full), "timeout", timeout, "thinking", reqPayload.Thinking != nil && reqPayload.Thinking.Type == "enabled")
	return callOpenAI(ctx, http.DefaultClient, prov.BaseURL, prov.APIKey, reqPayload)
}
