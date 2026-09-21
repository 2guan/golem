package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultHistoryFile        = "data/ai_history.json"
	defaultHistoryExpireHours = 72 // 默认 72 小时无更新自动过期淘汰
)

type sessionHistoryEntry struct {
	UpdatedAt time.Time       `json:"updated_at"`
	Messages  []openAIMessage `json:"messages"`
}

type historyData struct {
	Version  int                            `json:"version"`
	Sessions map[string]*sessionHistoryEntry `json:"sessions"`
}

func (p *AiPlugin) getHistoryFile() string {
	config := p.configSnapshot()
	if strings.TrimSpace(config.HistoryFile) != "" {
		return strings.TrimSpace(config.HistoryFile)
	}
	return defaultHistoryFile
}

func (p *AiPlugin) getHistoryExpireHours() int {
	config := p.configSnapshot()
	if config.HistoryExpireHours > 0 {
		return config.HistoryExpireHours
	}
	return defaultHistoryExpireHours
}

func (p *AiPlugin) ensureHistoryLoaded() {
	p.historyOnce.Do(func() {
		p.loadHistory()
	})
}

// loadHistory 从本地文件加载并恢复会话历史记录
func (p *AiPlugin) loadHistory() {
	filePath := p.getHistoryFile()
	raw, err := os.ReadFile(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("[ai] 读取历史对话文件失败", "err", err, "path", filePath)
		}
		return
	}

	var data historyData
	if err := json.Unmarshal(raw, &data); err != nil {
		slog.Warn("[ai] 反序列化历史对话文件失败", "err", err, "path", filePath)
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sessions == nil {
		p.sessions = map[string][]openAIMessage{}
	}
	if p.sessionTimes == nil {
		p.sessionTimes = map[string]time.Time{}
	}

	now := time.Now()
	expireDuration := time.Duration(p.getHistoryExpireHours()) * time.Hour
	restoredSessions := 0
	restoredMessages := 0

	for key, entry := range data.Sessions {
		if entry == nil || len(entry.Messages) == 0 {
			continue
		}
		// 检查是否过期
		if !entry.UpdatedAt.IsZero() && now.Sub(entry.UpdatedAt) > expireDuration {
			continue
		}

		limit := p.getMaxContextMessages(key)
		msgs := entry.Messages
		if len(msgs) > limit {
			msgs = msgs[len(msgs)-limit:]
		}

		p.sessions[key] = msgs
		if !entry.UpdatedAt.IsZero() {
			p.sessionTimes[key] = entry.UpdatedAt
		} else {
			p.sessionTimes[key] = now
		}

		restoredSessions++
		restoredMessages += len(msgs)
	}

	slog.Info("[ai] 历史对话上下文恢复成功",
		"file", filePath,
		"restored_sessions", restoredSessions,
		"restored_messages", restoredMessages,
	)
}

// saveHistoryAsync 异步保存历史会话至本地文件（极速快照+后台安全原子写入）
func (p *AiPlugin) saveHistoryAsync() {
	filePath, snapshot, timesSnapshot, expireDuration, seq := p.snapshotHistory()
	go p.persistHistorySnapshot(filePath, snapshot, timesSnapshot, expireDuration, seq)
}

// saveHistorySync 同步保存历史会话至本地文件
func (p *AiPlugin) saveHistorySync() {
	filePath, snapshot, timesSnapshot, expireDuration, seq := p.snapshotHistory()
	p.persistHistorySnapshot(filePath, snapshot, timesSnapshot, expireDuration, seq)
}

func (p *AiPlugin) snapshotHistory() (string, map[string][]openAIMessage, map[string]time.Time, time.Duration, uint64) {
	filePath := p.getHistoryFile()
	expireDuration := time.Duration(p.getHistoryExpireHours()) * time.Hour

	p.mu.Lock()
	p.historySeq++
	seq := p.historySeq
	// 内存浅拷贝快照，释放锁耗时 < 0.1ms
	snapshot := make(map[string][]openAIMessage, len(p.sessions))
	for k, v := range p.sessions {
		if len(v) > 0 {
			snapshot[k] = append([]openAIMessage(nil), v...)
		}
	}
	timesSnapshot := make(map[string]time.Time, len(p.sessionTimes))
	for k, v := range p.sessionTimes {
		timesSnapshot[k] = v
	}
	p.mu.Unlock()

	return filePath, snapshot, timesSnapshot, expireDuration, seq
}

// persistHistorySnapshot 安全原子写入历史文件
func (p *AiPlugin) persistHistorySnapshot(filePath string, sessions map[string][]openAIMessage, times map[string]time.Time, expireDuration time.Duration, seq uint64) {
	p.historyWriteMu.Lock()
	defer p.historyWriteMu.Unlock()

	// 严格按序号落盘：若已有更高版本的快照完成写入，丢弃旧版快照，避免多协程写入时序错乱
	if seq < p.lastPersistedSeq {
		return
	}

	data := historyData{
		Version:  1,
		Sessions: make(map[string]*sessionHistoryEntry, len(sessions)),
	}

	now := time.Now()
	for key, msgs := range sessions {
		if len(msgs) == 0 {
			continue
		}
		updatedAt, ok := times[key]
		if !ok || updatedAt.IsZero() {
			updatedAt = now
		}
		if now.Sub(updatedAt) > expireDuration {
			continue
		}

		data.Sessions[key] = &sessionHistoryEntry{
			UpdatedAt: updatedAt,
			Messages:  msgs,
		}
	}

	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		slog.Error("[ai] 序列化历史会话失败", "err", err)
		return
	}

	dir := filepath.Dir(filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			slog.Error("[ai] 创建历史会话目录失败", "err", err, "dir", dir)
			return
		}
	}

	tmpFile := filePath + ".tmp"
	if err := os.WriteFile(tmpFile, encoded, 0644); err != nil {
		slog.Error("[ai] 写入临时历史文件失败", "err", err, "tmp", tmpFile)
		return
	}

	if err := os.Rename(tmpFile, filePath); err != nil {
		slog.Error("[ai] 重命名持久化历史文件失败", "err", err, "path", filePath)
		return
	}

	p.lastPersistedSeq = seq
}
