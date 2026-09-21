package main

import (
	"strings"
	"time"
)

func isContentEmpty(c any) bool {
	if c == nil {
		return true
	}
	if s, ok := c.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	if parts, ok := c.([]contentPart); ok {
		return len(parts) == 0
	}
	return false
}

func (p *AiPlugin) appendContext(key string, msg openAIMessage) {
	if key == "" || isContentEmpty(msg.Content) {
		return
	}
	// 如果是文本消息，严禁将包含思维链泄漏或安全拦截的内容存入历史上下文
	if s, ok := msg.Content.(string); ok {
		s = stripThinkingContent(s)
		if isLeakedReasoningOrRefusal(s) {
			return
		}
		msg.Content = s
	}
	p.ensureHistoryLoaded()

	p.mu.Lock()
	if p.sessions == nil {
		p.sessions = map[string][]openAIMessage{}
	}
	if p.sessionTimes == nil {
		p.sessionTimes = map[string]time.Time{}
	}
	items := append(p.sessions[key], msg)
	limit := p.getMaxContextMessages(key)
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	p.sessions[key] = items
	p.sessionTimes[key] = time.Now()
	p.mu.Unlock()

	p.saveHistoryAsync()
}

// slimContext 将指定会话中消息的图片 Base64 替换为精简占位符，防止后续轮次重复发送巨量 Base64 数据
func (p *AiPlugin) slimContext(key string) {
	p.ensureHistoryLoaded()

	p.mu.Lock()
	items := p.sessions[key]
	modified := false
	for i := range items {
		if parts, ok := items[i].Content.([]contentPart); ok {
			var textParts []string
			for _, part := range parts {
				if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
					textParts = append(textParts, part.Text)
				} else if part.Type == "image_url" {
					textParts = append(textParts, "[图片]")
				}
			}
			items[i].Content = strings.Join(textParts, " ")
			modified = true
		}
	}
	p.mu.Unlock()

	if modified {
		p.saveHistoryAsync()
	}
}

func (p *AiPlugin) contextMessages(key string) []openAIMessage {
	p.ensureHistoryLoaded()
	p.mu.Lock()
	defer p.mu.Unlock()
	items := p.sessions[key]
	if len(items) == 0 {
		return nil
	}
	return append([]openAIMessage(nil), items...)
}

func (p *AiPlugin) clearContext(key string) {
	p.ensureHistoryLoaded()
	p.mu.Lock()
	delete(p.sessions, key)
	if p.sessionTimes != nil {
		delete(p.sessionTimes, key)
	}
	p.mu.Unlock()

	p.saveHistoryAsync()
}

func (p *AiPlugin) popLastContext(key string) {
	p.ensureHistoryLoaded()
	p.mu.Lock()
	items := p.sessions[key]
	if len(items) > 0 {
		p.sessions[key] = items[:len(items)-1]
	}
	p.mu.Unlock()

	p.saveHistoryAsync()
}
