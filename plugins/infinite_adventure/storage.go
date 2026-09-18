package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StorageData 持久化数据结构
type StorageData struct {
	SoloStates  map[string]*SoloAdventureState  `json:"solo_states"`
	GroupStates map[string]*GroupAdventureState `json:"group_states"`
}

// StorageManager 状态与存档管理器
type StorageManager struct {
	mu          sync.RWMutex
	filePath    string
	soloStates  map[string]*SoloAdventureState
	groupStates map[string]*GroupAdventureState
}

// NewStorageManager 创建存档管理器
func NewStorageManager(filePath string) *StorageManager {
	if filePath == "" {
		filePath = filepath.Join("data", "infinite_adventure.json")
	}
	sm := &StorageManager{
		filePath:    filePath,
		soloStates:  make(map[string]*SoloAdventureState),
		groupStates: make(map[string]*GroupAdventureState),
	}
	sm.load()
	return sm
}

func (sm *StorageManager) load() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	data, err := os.ReadFile(sm.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("[infinite_adventure] 读取存档文件失败", "err", err, "path", sm.filePath)
		}
		return
	}

	var sd StorageData
	if err := json.Unmarshal(data, &sd); err != nil {
		slog.Warn("[infinite_adventure] 反序列化存档失败", "err", err)
		return
	}

	if sd.SoloStates != nil {
		sm.soloStates = sd.SoloStates
	}
	if sd.GroupStates != nil {
		sm.groupStates = sd.GroupStates
	}
	slog.Info("[infinite_adventure] 存档数据加载成功",
		"solo_count", len(sm.soloStates),
		"group_count", len(sm.groupStates),
	)
}

func (sm *StorageManager) saveLocked() {
	sd := StorageData{
		SoloStates:  sm.soloStates,
		GroupStates: sm.groupStates,
	}
	bytes, err := json.MarshalIndent(sd, "", "  ")
	if err != nil {
		slog.Error("[infinite_adventure] 序列化存档失败", "err", err)
		return
	}

	dir := filepath.Dir(sm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.Error("[infinite_adventure] 创建存档目录失败", "err", err, "dir", dir)
		return
	}

	if err := os.WriteFile(sm.filePath, bytes, 0644); err != nil {
		slog.Error("[infinite_adventure] 写入存档文件失败", "err", err, "path", sm.filePath)
	}
}

// GetSolo 获取私聊存档
func (sm *StorageManager) GetSolo(sessionID string) *SoloAdventureState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.soloStates[sessionID]
}

// SetSolo 保存私聊存档
func (sm *StorageManager) SetSolo(state *SoloAdventureState) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	state.UpdatedAt = time.Now()
	sm.soloStates[state.SessionID] = state
	sm.saveLocked()
}

// DeleteSolo 删除私聊存档
func (sm *StorageManager) DeleteSolo(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.soloStates, sessionID)
	sm.saveLocked()
}

// GetGroup 获取群聊存档
func (sm *StorageManager) GetGroup(chatroomID string) *GroupAdventureState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.groupStates[chatroomID]
}

// SetGroup 保存群聊存档
func (sm *StorageManager) SetGroup(state *GroupAdventureState) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	state.UpdatedAt = time.Now()
	sm.groupStates[state.ChatroomID] = state
	sm.saveLocked()
}

// DeleteGroup 删除群聊存档
func (sm *StorageManager) DeleteGroup(chatroomID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.groupStates, chatroomID)
	sm.saveLocked()
}
