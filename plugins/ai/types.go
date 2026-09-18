package main

import (
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
)

const contactTypeChatroom = contact.ContactType_CONTACT_TYPE_CHATROOM

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type incomingMessage struct {
	SessionKey   string
	Receiver     *contact.Contact
	Text         string
	IsChatroom   bool
	MentionedBot bool
	QuotedBot    bool

	ChatroomName string
	SpeakerName  string
	SpeakerID    string
	Quote        quoteInfo

	RawMsg        *message.Message
	IsImage       bool
	ImageData     []byte
	ImageMimeType string
}

type quoteInfo struct {
	FromUser    string
	ChatUser    string
	DisplayName string
	Content     string
}

// SessionConfig 会话级配置（所有字段为指针，nil 表示使用全局配置）
type SessionConfig struct {
	ReplyRate          *float64 `toml:"reply_rate,omitempty"`
	ActivePrompt       *string  `toml:"active_prompt,omitempty"`
	MaxContextMessages *int     `toml:"max_context_messages,omitempty"`
	ActiveProvider     *string  `toml:"active_provider,omitempty"`
	Silence            *bool    `toml:"silence,omitempty"`
}

// Provider 预设的 OpenAI 兼容服务提供方配置
type Provider struct {
	BaseURL            string   `toml:"base_url" comment:"OpenAI 兼容接口地址，例如 https://api.openai.com/v1"`
	APIKey             string   `toml:"api_key" comment:"接口密钥"`
	Model              string   `toml:"model" comment:"模型名称"`
	HTTPTimeoutSeconds int      `toml:"http_timeout_seconds,omitempty" comment:"请求超时秒数，0 表示使用全局缺省"`
	Temperature        *float64 `toml:"temperature,omitempty" comment:"采样温度（默认 0.88）"`
	PresencePenalty    *float64 `toml:"presence_penalty,omitempty" comment:"存在惩罚（默认 0.35）"`
}
