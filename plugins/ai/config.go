package main

import "strings"

const (
	defaultPromptName         = "default"
	defaultMaxContextMessages = 200
	defaultHTTPTimeoutSeconds = 60
)

// defaultConfig 返回默认配置
func defaultConfig() Config {
	return Config{
		ActivePrompt:       defaultPromptName,
		Prompts:            map[string]string{defaultPromptName: ""},
		ReplyRate:          0.1,
		MaxContextMessages: defaultMaxContextMessages,
		HTTPTimeoutSeconds: defaultHTTPTimeoutSeconds,
		TTS:                defaultTTSConfig(),
	}
}

// normalizeConfig 标准化配置
func (p *AiPlugin) normalizeConfig() {
	p.configMu.Lock()
	defer p.configMu.Unlock()
	p.Config = normalizeConfigValue(p.Config)
}

// configSnapshot 获取配置快照
func (p *AiPlugin) configSnapshot() Config {
	p.configMu.RLock()
	config := p.Config
	p.configMu.RUnlock()
	return normalizeConfigValue(config)
}

// normalizeConfigValue 标准化配置值
func normalizeConfigValue(config Config) Config {
	config.ActivePrompt = strings.TrimSpace(config.ActivePrompt)
	if config.ActivePrompt == "" {
		config.ActivePrompt = defaultPromptName
	}
	config.Prompts = normalizePrompts(config.Prompts, config.LegacyPrompt, config.ActivePrompt)
	config.LegacyPrompt = ""
	config.ActiveProvider = strings.TrimSpace(config.ActiveProvider)
	config.FallbackProvider = strings.TrimSpace(config.FallbackProvider)
	config.Providers = normalizeProviders(config.Providers)
	if config.MaxContextMessages <= 0 {
		config.MaxContextMessages = defaultMaxContextMessages
	}
	if config.HTTPTimeoutSeconds <= 0 {
		config.HTTPTimeoutSeconds = defaultHTTPTimeoutSeconds
	}
	if config.ReplyRate < 0 {
		config.ReplyRate = 0
	}
	if config.ReplyRate > 1 {
		config.ReplyRate = 1
	}
	config.TTS.Model = strings.TrimSpace(config.TTS.Model)
	if config.TTS.Model == "" {
		config.TTS.Model = "mimo-v2.5-tts-voicedesign"
	}
	config.TTS.VoiceDesign = strings.TrimSpace(config.TTS.VoiceDesign)
	if config.TTS.VoiceDesign == "" {
		config.TTS.VoiceDesign = "一位三十多岁的成熟男性朋友。嗓音富有磁性有质感，但音调自然轻松不沉闷。说话亲切温和、随性自如，语速轻快，带有自然的口语起伏和笑意，像日常随手拿起手机给朋友发微信语音闲聊。"
	}
	config.TTS.Mode = strings.TrimSpace(config.TTS.Mode)
	if config.TTS.Mode == "" {
		config.TTS.Mode = "both"
	}
	return config
}

// normalizeProviders 标准化 provider 映射
func normalizeProviders(providers map[string]*Provider) map[string]*Provider {
	normalized := make(map[string]*Provider, len(providers))
	for name, prov := range providers {
		name = strings.TrimSpace(name)
		if name == "" || prov == nil {
			continue
		}
		prov.BaseURL = strings.TrimSpace(prov.BaseURL)
		prov.APIKey = strings.TrimSpace(prov.APIKey)
		prov.Model = strings.TrimSpace(prov.Model)
		if prov.HTTPTimeoutSeconds < 0 {
			prov.HTTPTimeoutSeconds = 0
		}
		normalized[name] = prov
	}
	return normalized
}

// normalizePrompts 标准化提示词映射
func normalizePrompts(prompts map[string]string, legacyPrompt, activePrompt string) map[string]string {
	normalized := make(map[string]string, len(prompts)+1)
	for name, prompt := range prompts {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		normalized[name] = strings.TrimSpace(prompt)
	}
	if legacyPrompt = strings.TrimSpace(legacyPrompt); legacyPrompt != "" {
		if _, ok := normalized[defaultPromptName]; !ok {
			normalized[defaultPromptName] = legacyPrompt
		}
	}
	if _, ok := normalized[activePrompt]; !ok {
		normalized[activePrompt] = ""
	}
	return normalized
}

// activePromptContent 获取活动提示词内容
func activePromptContent(config Config) string {
	return strings.TrimSpace(config.Prompts[config.ActivePrompt])
}
