package main

import (
	"fmt"
	"sync"
	"time"
)

// HistoryMsg 互动历史
type HistoryMsg struct {
	Role    string // "user" or "assistant"
	Content string
}

// SoloSession 私聊 1V1 沉浸存档
type SoloSession struct {
	UserID       string
	Nickname     string
	Scene        SceneType
	HeartRate    int // 60 ~ 180 bpm
	Tension      int // 0 ~ 100 %
	ExtraStatus  string
	History      []HistoryMsg
	LastActive   time.Time
}

// SessionManager 状态管理器
type SessionManager struct {
	mu           sync.RWMutex
	soloSessions map[string]*SoloSession
	stopCh       chan struct{}
}

// NewSessionManager 初始化
func NewSessionManager() *SessionManager {
	sm := &SessionManager{
		soloSessions: make(map[string]*SoloSession),
		stopCh:       make(chan struct{}),
	}
	go sm.cleanupRoutine()
	return sm
}

// GetSolo 获取用户会话（若过期则重置）
func (sm *SessionManager) GetSolo(userID string) *SoloSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	s, ok := sm.soloSessions[userID]
	if !ok {
		return nil
	}
	// 20 分钟超时清理
	if time.Since(s.LastActive) > 20*time.Minute {
		delete(sm.soloSessions, userID)
		return nil
	}
	s.LastActive = time.Now()
	return s
}

// StartSolo 开启新场景
func (sm *SessionManager) StartSolo(userID, nickname string, sceneType SceneType) *SoloSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	extra := ""
	hr := 75
	switch sceneType {
	case SceneGym:
		extra = "肌肉微充血 · 汗水蒸腾 37.3℃"
		hr = 85
	case SceneBar:
		extra = "微醺 15% · 爵士余韵"
		hr = 78
	case SceneDorm:
		extra = "被窝温度 36.9℃ · 睡意全无"
		hr = 72
	}

	s := &SoloSession{
		UserID:      userID,
		Nickname:    nickname,
		Scene:       sceneType,
		HeartRate:   hr,
		Tension:     15,
		ExtraStatus: extra,
		History:     make([]HistoryMsg, 0),
		LastActive:  time.Now(),
	}
	sm.soloSessions[userID] = s
	return s
}

// EndSolo 离开当前场景
func (sm *SessionManager) EndSolo(userID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, ok := sm.soloSessions[userID]; ok {
		delete(sm.soloSessions, userID)
		return true
	}
	return false
}

// UpdateSolo 更新互动后状态
func (sm *SessionManager) UpdateSolo(userID string, hrDelta, tensionDelta int, newExtra, userAct, assistantReply string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	s, ok := sm.soloSessions[userID]
	if !ok {
		return
	}
	s.HeartRate += hrDelta
	if s.HeartRate < 60 {
		s.HeartRate = 60
	}
	if s.HeartRate > 185 {
		s.HeartRate = 185
	}

	s.Tension += tensionDelta
	if s.Tension < 0 {
		s.Tension = 0
	}
	if s.Tension > 100 {
		s.Tension = 100
	}

	if newExtra != "" {
		s.ExtraStatus = newExtra
	}

	s.History = append(s.History,
		HistoryMsg{Role: "user", Content: userAct},
		HistoryMsg{Role: "assistant", Content: assistantReply},
	)
	// 保留最近 8 条对话历史，防止上下文膨胀
	if len(s.History) > 8 {
		s.History = s.History[len(s.History)-8:]
	}
	s.LastActive = time.Now()
}

// FormatStatusCard 格式化末尾简短指数
func (s *SoloSession) FormatStatusCard() string {
	return fmt.Sprintf("📊【身体指标】：❤️ 心率 %d bpm | 🔥 暧昧度 %d%%", s.HeartRate, s.Tension)
}

// 定期清理过期会话
func (sm *SessionManager) cleanupRoutine() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-sm.stopCh:
			return
		case <-ticker.C:
			sm.mu.Lock()
			now := time.Now()
			for uid, s := range sm.soloSessions {
				if now.Sub(s.LastActive) > 20*time.Minute {
					delete(sm.soloSessions, uid)
				}
			}
			sm.mu.Unlock()
		}
	}
}

// Close 停止清理
func (sm *SessionManager) Close() {
	close(sm.stopCh)
}
