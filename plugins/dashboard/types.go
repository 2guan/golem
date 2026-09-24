package main

import (
	"time"
)

// AdminCredentials 管理员认证信息
type AdminCredentials struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	JWTSecret    string    `json:"jwt_secret"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TargetOverride 单个联系人或群组的专属 AI 策略
type TargetOverride struct {
	TargetID    string    `json:"target_id"`              // 如 "wxid_xxx" 或 "18351364439@chatroom"
	TargetType  string    `json:"target_type"`            // "friend" 或 "chatroom"
	Name        string    `json:"name"`                   // 昵称或群名
	Remark      string    `json:"remark"`                 // 备注名
	CustomPrompt string   `json:"custom_prompt,omitempty"`// 专属提示词
	Provider    string    `json:"provider,omitempty"`     // 专属模型提供方 (如 "gemini", "xiaomi")
	Model       string    `json:"model,omitempty"`        // 专属模型名
	ReplyRate   *float64  `json:"reply_rate,omitempty"`   // 专属回复率 (0.0 - 1.0)
	Silence     *bool     `json:"silence,omitempty"`      // 是否静默模式 (仅被@或私聊触发)
	Notes       string    `json:"notes,omitempty"`        // 管理员备注
	UpdatedAt   time.Time `json:"updated_at"`
}

// DashboardStore 本地持久化数据存储结构 (data/dashboard.json)
type DashboardStore struct {
	Admin     AdminCredentials           `json:"admin"`
	Overrides map[string]*TargetOverride `json:"overrides"`
}

// API 请求与响应结构体
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	ExpireAt int64  `json:"expire_at"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type OverviewResponse struct {
	Bot struct {
		Username  string `json:"username"`
		Nickname  string `json:"nickname"`
		AvatarURL string `json:"avatar_url"`
		Owner     string `json:"owner"`
		Status    string `json:"status"`
		Uptime    int64  `json:"uptime_seconds"`
	} `json:"bot"`
	Stats struct {
		TotalContacts int `json:"total_contacts"`
		TotalGroups   int `json:"total_groups"`
		TotalMessages int `json:"total_messages"`
		ActiveSessions int `json:"active_sessions"`
		PluginCount   int `json:"plugin_count"`
	} `json:"stats"`
}

type LogItem struct {
	ID        int    `json:"id"`
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Tag       string `json:"tag"`
	Message   string `json:"message"`
	Raw       string `json:"raw"`
}

type LogQueryResponse struct {
	Total    int       `json:"total"`
	Filtered int       `json:"filtered"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
	Items    []LogItem `json:"items"`
}

type ChatSessionItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"` // "friend" | "chatroom"
	AvatarURL   string `json:"avatar_url"`
	LastMessage string `json:"last_message"`
	LastTime    string `json:"last_time"`
	MsgCount    int    `json:"message_count"`
}

type ChatMessageItem struct {
	ID         int64  `json:"id"`
	SessionID  string `json:"session_id"`
	SenderID   string `json:"sender_id"`
	SenderName string `json:"sender_name"`
	AvatarURL  string `json:"avatar_url"`
	IsSelf     bool   `json:"is_self"`
	Role       string `json:"role"` // "user" | "assistant"
	Type       string `json:"type"` // "text", "image", "voice", "quote"
	Content    string `json:"content"`
	MediaURL      string `json:"media_url,omitempty"`
	Duration      int    `json:"duration,omitempty"`
	Timestamp     string `json:"timestamp"`
	IsCutoffPoint bool   `json:"is_cutoff_point,omitempty"`
}

type AIAnalyzeRequest struct {
	TargetID   string `json:"target_id"`
	Type       string `json:"type"` // "persona" | "summary" | "sentiment"
	PromptHint string `json:"prompt_hint,omitempty"`
}

type AIAnalyzeResponse struct {
	TargetID  string `json:"target_id"`
	Type      string `json:"type"`
	Result    string `json:"result"`
	ModelUsed string `json:"model_used"`
}
