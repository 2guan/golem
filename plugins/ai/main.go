package main

import (
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/sbgayhub/golem/sdk/cdn"
	"github.com/sbgayhub/golem/sdk/chatroom"
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

type cachedImage struct {
	Data      []byte
	MimeType  string
	Time      time.Time
	SpeakerID string
}

// AiPlugin AI 插件主结构
type AiPlugin struct {
	plugin.ConfigAbility[Config]
	contact  contact.Ability
	message  message.Ability
	cdn      cdn.Ability
	chatroom chatroom.Ability

	configMu        sync.RWMutex
	sessionConfigMu sync.RWMutex
	selfMu          sync.RWMutex
	self            *contact.SelfInfo
	owner           *contact.Contact
	mu              sync.Mutex
	sessions        map[string][]openAIMessage

	imageMu      sync.Mutex
	recentImages map[string]*cachedImage
}

// Config 插件配置
type Config struct {
	Providers          map[string]*Provider      `toml:"providers,omitempty" comment:"Provider 预设映射，key 为 provider 名称"`
	ActiveProvider     string                    `toml:"active_provider" comment:"当前使用的 provider 名称"`
	FallbackProvider   string                    `toml:"fallback_provider,omitempty" comment:"触发风控拦截或主模型异常时无缝切换的备用 provider"`
	ActivePrompt       string                    `toml:"active_prompt" comment:"当前使用的提示词名称"`
	Prompts            map[string]string         `toml:"prompts" comment:"提示词映射，key 为提示词名称"`
	LegacyPrompt       string                    `toml:"prompt,omitempty" comment:"旧版提示词配置，启动后迁移到 prompts.default"`
	ReplyRate          float64                   `toml:"reply_rate" comment:"普通消息回复概率，取值 0~1"`
	MaxContextMessages int                       `toml:"max_context_messages" comment:"每个会话最多保留的上下文消息数"`
	HTTPTimeoutSeconds int                       `toml:"http_timeout_seconds" comment:"大模型请求超时缺省值，单位秒"`
	Silence            bool                      `toml:"silence" comment:"全局静默模式，开启后仅在被 @ 或引用时回复"`
	SessionConfigs     map[string]*SessionConfig `toml:"session_configs,omitempty" comment:"会话级配置，key 为会话标识"`
	TTS                TTSConfig                 `toml:"tts,omitempty" comment:"TTS 语音合成配置"`
}

// newAiPlugin 创建 AI 插件实例
func newAiPlugin() (*AiPlugin, error) {
	p := &AiPlugin{
		ConfigAbility: plugin.ConfigAbility[Config]{
			Config: defaultConfig(),
		},
		sessions:     map[string][]openAIMessage{},
		recentImages: map[string]*cachedImage{},
	}
	if err := registerCommands(p); err != nil {
		return nil, err
	}
	return p, nil
}

// shouldReply 判断是否应该回复
func (p *AiPlugin) shouldReply(in incomingMessage) bool {
	if in.MentionedBot || in.QuotedBot {
		return true
	}
	if p.isSilent(in.SessionKey) {
		return false
	}
	if !in.IsChatroom {
		return true
	}
	rate := p.getReplyRate(in.SessionKey)
	if rate <= 0 {
		return false
	}
	if rate >= 1 {
		return true
	}
	return rand.Float64() < rate
}

// main 插件入口
func main() {
	p, err := newAiPlugin()
	if err != nil {
		slog.Error("[ai] 初始化失败", "err", err)
		return
	}
	plugin.Start(p)
}
