package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/sbgayhub/golem/sdk/plugin"
)

type AIService struct {
	caller        plugin.CallerAbility
	pluginsConfig string
}

func NewAIService(caller plugin.CallerAbility, pluginsConfig string) *AIService {
	return &AIService{
		caller:        caller,
		pluginsConfig: pluginsConfig,
	}
}

type AIChatPayload struct {
	System         string               `json:"system"`
	Messages       []AIChatPayloadMsg   `json:"messages"`
	TimeoutSeconds int                  `json:"timeout_seconds,omitempty"`
}

type AIChatPayloadMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Analyze 执行智能分析
func (s *AIService) Analyze(targetName string, messages []ChatMessageItem, analyzeType string, hint string) (string, string, error) {
	if len(messages) == 0 {
		return "", "", errors.New("聊天历史记录为空，无法进行分析")
	}

	// 限制最多分析近期 100 条消息
	startIdx := 0
	if len(messages) > 100 {
		startIdx = len(messages) - 100
	}
	recent := messages[startIdx:]

	var historyBuilder strings.Builder
	for _, m := range recent {
		speaker := m.SenderName
		if speaker == "" {
			speaker = m.SenderID
		}
		if m.IsSelf {
			if m.SenderName != "" {
				speaker = m.SenderName + " (我)"
			} else {
				speaker = "我方"
			}
		}
		historyBuilder.WriteString(fmt.Sprintf("[%s] %s: %s\n", m.Timestamp, speaker, m.Content))
	}
	chatHistoryText := historyBuilder.String()

	var systemPrompt string
	switch analyzeType {
	case "persona":
		systemPrompt = `你是一名资深的心理学专家与社交关系分析师。请根据提供的用户与机器人的聊天历史记录，对目标用户【` + targetName + `】进行全面、多维度、有深度且幽默风趣的综合用户画像分析。
报告请使用清晰规范的 Markdown 格式，包含以下模块：
### 1. 核心性格特质 (Personality Traits)
- 语言风格与常用口头禅
- 情绪稳定性与性格倾向 (外向/内向/幽默/敏感等)
### 2. 核心关注点与兴趣领域 (Interests & Topics)
- 频繁讨论的话题、生活状态或工作领域
### 3. 对机器人的态度与情感倾向 (Relationship Dynamics)
- 亲密度评分 (1-10分) 与信任程度
- 互动模式 (随和闲聊/索取信息/挑逗撩拨/依赖倾诉)
### 4. 针对性社交与沟通建议 (Tips for Interaction)
- 怎样与他/她聊天能增进好感，有哪些沟通雷区`

	case "summary":
		systemPrompt = `你是一名出色的私人秘书与信息提炼专家。请仔细梳理【` + targetName + `】与机器人的近期对话历史，提炼出清晰的聊天脉络与核心事件总结。
报告请使用规范的 Markdown 格式，包含以下模块：
### 1. 近期核心事件与脉络梳理 (Key Events & Highlights)
- 按时间线或主题分类列出谈及的关键事情
### 2. 约定、承诺与待办事项 (Agreements & Action Items)
- 双方约定的活动、约饭、承诺或需跟进的事项
### 3. 重点情绪波动或八卦细节 (Memorable Moments)`

	case "sentiment":
		systemPrompt = `你是一名精细的情感分析专家。请对【` + targetName + `】的近期对话进行情感走势与心理状态分析。
报告请使用 Markdown 格式：
### 1. 整体情绪基调 (Emotional Tone)
- 积极/消极/疲惫/兴奋/焦虑 等情绪比例
### 2. 情感变化拐点 (Turning Points)
- 哪次对话或事件引起了明显的情绪变化
### 3. 当前心理需求与关怀建议 (Care Recommendations)`

	default:
		systemPrompt = `你是一名智能助手，请根据提供的聊天记录进行客观总结分析。`
	}

	if hint != "" {
		systemPrompt += "\n\n管理员特别要求：" + hint
	}

	userMsg := "以下是目标聊天历史记录：\n\n" + chatHistoryText + "\n\n请开始分析："

	// 1. 优先使用 Golem caller 跨插件调用 ai.chat
	if s.caller != nil {
		payloadObj := AIChatPayload{
			System: systemPrompt,
			Messages: []AIChatPayloadMsg{
				{Role: "user", Content: userMsg},
			},
			TimeoutSeconds: 60,
		}
		payloadBytes, err := json.Marshal(payloadObj)
		if err == nil {
			_, respData, callErr := s.caller.CallPlugin("ai.chat", map[string]string{
				"payload": string(payloadBytes),
			})
			if callErr == nil && len(respData) > 0 {
				return string(respData), "Golem AI Plugin (ai.chat)", nil
			}
		}
	}

	// 2. 兜底方案：直接从 plugins/config.toml 读取配置直连 LLM
	return s.fallbackDirectChat(systemPrompt, userMsg)
}

func (s *AIService) fallbackDirectChat(systemPrompt, userMsg string) (string, string, error) {
	configData, err := os.ReadFile(s.pluginsConfig)
	if err != nil {
		return "", "", fmt.Errorf("无法读取配置文件: %w", err)
	}

	var root map[string]any
	if err := toml.Unmarshal(configData, &root); err != nil {
		return "", "", fmt.Errorf("解析配置文件失败: %w", err)
	}

	aiSec, ok := root["ai"].(map[string]any)
	if !ok {
		return "", "", errors.New("配置文件中未找到 [ai] 配置")
	}
	aiCfg, ok := aiSec["config"].(map[string]any)
	if !ok {
		return "", "", errors.New("未找到 [ai.config]")
	}
	providers, ok := aiCfg["providers"].(map[string]any)
	if !ok {
		return "", "", errors.New("未找到 [ai.config.providers]")
	}

	activeProvName := "gemini"
	if p, ok := aiCfg["active_provider"].(string); ok && p != "" {
		activeProvName = p
	}

	prov, ok := providers[activeProvName].(map[string]any)
	if !ok {
		// 找第一个
		for k, v := range providers {
			activeProvName = k
			prov = v.(map[string]any)
			break
		}
	}

	baseURL, _ := prov["base_url"].(string)
	apiKey, _ := prov["api_key"].(string)
	model, _ := prov["model"].(string)

	if baseURL == "" || apiKey == "" || model == "" {
		return "", "", errors.New("AI 提供方配置不完整")
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/chat/completions"

	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMsg},
		},
		"temperature": 0.7,
	}
	reqJSON, _ := json.Marshal(reqBody)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqJSON))
	if err != nil {
		return "", "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", "", fmt.Errorf("请求 LLM 接口失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("LLM 返回状态码 %d: %s", resp.StatusCode, string(respBytes))
	}

	var openAIResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBytes, &openAIResp); err != nil {
		return "", "", fmt.Errorf("解析 LLM 响应失败: %w", err)
	}
	if len(openAIResp.Choices) == 0 {
		return "", "", errors.New("LLM 未返回有效回复")
	}

	return openAIResp.Choices[0].Message.Content, fmt.Sprintf("%s (%s)", activeProvName, model), nil
}
