package main

import "strings"

// getReplyRate 获取会话的回复概率（优先使用会话配置）
func (p *AiPlugin) getReplyRate(sessionKey string) float64 {
	if cfg := p.getSessionConfig(sessionKey); cfg != nil && cfg.ReplyRate != nil {
		return *cfg.ReplyRate
	}
	return p.configSnapshot().ReplyRate
}

// getActivePrompt 获取会话的活动提示词（优先使用会话配置）
func (p *AiPlugin) getActivePrompt(sessionKey string) string {
	if cfg := p.getSessionConfig(sessionKey); cfg != nil && cfg.ActivePrompt != nil {
		return *cfg.ActivePrompt
	}
	return p.configSnapshot().ActivePrompt
}

// getMaxContextMessages 获取会话的上下文消息数（优先使用会话配置）
func (p *AiPlugin) getMaxContextMessages(sessionKey string) int {
	if cfg := p.getSessionConfig(sessionKey); cfg != nil && cfg.MaxContextMessages != nil {
		return *cfg.MaxContextMessages
	}
	return p.configSnapshot().MaxContextMessages
}

// getActiveProvider 获取会话的活动 provider（优先使用会话配置，回退全局）
func (p *AiPlugin) getActiveProvider(sessionKey string) string {
	if cfg := p.getSessionConfig(sessionKey); cfg != nil && cfg.ActiveProvider != nil {
		return *cfg.ActiveProvider
	}
	return p.configSnapshot().ActiveProvider
}

// isSilent 判断会话是否处于静默模式（优先使用会话配置，回退全局）
func (p *AiPlugin) isSilent(sessionKey string) bool {
	if cfg := p.getSessionConfig(sessionKey); cfg != nil && cfg.Silence != nil {
		return *cfg.Silence
	}
	return p.configSnapshot().Silence
}

// getCustomNickname 获取会话配置的自定义昵称
func (p *AiPlugin) getCustomNickname(sessionKey string) string {
	if cfg := p.getSessionConfig(sessionKey); cfg != nil && cfg.CustomNickname != nil {
		return *cfg.CustomNickname
	}
	return ""
}

// resolveTargetCustomNickname 解析联系人的自定义昵称/称呼
// 优先级：会话配置中明确设置的 custom_nickname > 微信好友备注名 (Remark)
// contactID 为空时，若 sessionKey 是私聊则从 sessionKey 提取 contactID
func (p *AiPlugin) resolveTargetCustomNickname(sessionKey string, contactID string) string {
	// 1. 尝试从会话配置获取自定义昵称
	if nick := p.getCustomNickname(sessionKey); nick != "" {
		return nick
	}

	targetID := contactID
	if targetID == "" {
		clean := sessionKey
		if idx := strings.Index(clean, ":"); idx != -1 {
			clean = clean[idx+1:]
		}
		if !strings.HasSuffix(clean, "@chatroom") {
			targetID = clean
		}
	}

	if targetID != "" {
		// 也尝试匹配 targetID 相关的 session 配置
		if nick := p.getCustomNickname("private:" + targetID); nick != "" {
			return nick
		}
		if nick := p.getCustomNickname("contact:" + targetID); nick != "" {
			return nick
		}
		if nick := p.getCustomNickname(targetID); nick != "" {
			return nick
		}

		// 2. 尝试从微信通讯录获取备注名 Remark
		if p.contact != nil {
			if contact := p.contact.Get(targetID); contact != nil {
				if remark := contact.GetRemark(); remark != "" {
					return remark
				}
			}
		}
	}

	return ""
}

// getSessionConfig 线程安全地获取会话配置，支持私聊/联系人/群聊等前缀兼容匹配
func (p *AiPlugin) getSessionConfig(sessionKey string) *SessionConfig {
	p.sessionConfigMu.RLock()
	defer p.sessionConfigMu.RUnlock()
	if p.Config.SessionConfigs == nil {
		return nil
	}

	// 1. 精确匹配
	if cfg, ok := p.Config.SessionConfigs[sessionKey]; ok && cfg != nil {
		return cfg
	}

	// 2. 提取不带前缀的 cleanKey (如 "private:wxid_xxx" -> "wxid_xxx")
	cleanKey := sessionKey
	if idx := strings.Index(sessionKey, ":"); idx != -1 {
		cleanKey = sessionKey[idx+1:]
	}

	// 尝试其他常见前缀组合
	candidates := []string{
		cleanKey,
		"private:" + cleanKey,
		"contact:" + cleanKey,
		"chatroom:" + cleanKey,
	}
	for _, cand := range candidates {
		if cand == sessionKey {
			continue
		}
		if cfg, ok := p.Config.SessionConfigs[cand]; ok && cfg != nil {
			return cfg
		}
	}

	return nil
}

// setSessionConfig 线程安全地设置会话配置
func (p *AiPlugin) setSessionConfig(sessionKey string, cfg *SessionConfig) {
	p.sessionConfigMu.Lock()
	defer p.sessionConfigMu.Unlock()
	if p.Config.SessionConfigs == nil {
		p.Config.SessionConfigs = make(map[string]*SessionConfig)
	}
	p.Config.SessionConfigs[sessionKey] = cfg
}

// deleteSessionConfig 线程安全地删除会话配置
func (p *AiPlugin) deleteSessionConfig(sessionKey string) {
	p.sessionConfigMu.Lock()
	defer p.sessionConfigMu.Unlock()
	if p.Config.SessionConfigs != nil {
		delete(p.Config.SessionConfigs, sessionKey)
	}
}

// listSessionKeys 获取所有已配置的会话 key
func (p *AiPlugin) listSessionKeys() []string {
	p.sessionConfigMu.RLock()
	defer p.sessionConfigMu.RUnlock()
	if p.Config.SessionConfigs == nil {
		return nil
	}
	keys := make([]string, 0, len(p.Config.SessionConfigs))
	for k := range p.Config.SessionConfigs {
		keys = append(keys, k)
	}
	return keys
}
