package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type StoreManager struct {
	mu       sync.RWMutex
	filePath string
	data     *DashboardStore
}

func NewStoreManager(filePath string) (*StoreManager, error) {
	sm := &StoreManager{
		filePath: filePath,
	}
	if err := sm.load(); err != nil {
		return nil, err
	}
	return sm, nil
}

func (sm *StoreManager) load() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	dir := filepath.Dir(sm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	content, err := os.ReadFile(sm.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 初始化默认配置
			defaultSecret := GenerateRandomSecret(32)
			hashedPass, _ := HashPassword("admin123")
			now := time.Now()

			store := &DashboardStore{
				Admin: AdminCredentials{
					Username:     "admin",
					PasswordHash: hashedPass,
					JWTSecret:    defaultSecret,
					CreatedAt:    now,
					UpdatedAt:    now,
				},
				Overrides: make(map[string]*TargetOverride),
			}
			sm.data = store
			return sm.saveLocked()
		}
		return err
	}

	var store DashboardStore
	if err := json.Unmarshal(content, &store); err != nil {
		return err
	}
	if store.Overrides == nil {
		store.Overrides = make(map[string]*TargetOverride)
	}
	if store.Admin.JWTSecret == "" {
		store.Admin.JWTSecret = GenerateRandomSecret(32)
	}
	sm.data = &store
	return nil
}

func (sm *StoreManager) saveLocked() error {
	bytes, err := json.MarshalIndent(sm.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sm.filePath, bytes, 0644)
}

func (sm *StoreManager) Save() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.saveLocked()
}

func (sm *StoreManager) GetAdmin() AdminCredentials {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.data.Admin
}

func (sm *StoreManager) UpdateAdminPassword(newPasswordHash string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.data.Admin.PasswordHash = newPasswordHash
	sm.data.Admin.UpdatedAt = time.Now()
	return sm.saveLocked()
}

func (sm *StoreManager) GetOverrides() map[string]*TargetOverride {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make(map[string]*TargetOverride, len(sm.data.Overrides))
	for k, v := range sm.data.Overrides {
		clone := *v
		result[k] = &clone
	}
	return result
}

func (sm *StoreManager) GetOverride(targetID string) (*TargetOverride, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	v, ok := sm.data.Overrides[targetID]
	if !ok || v == nil {
		return nil, false
	}
	clone := *v
	return &clone, true
}

func (sm *StoreManager) SetOverride(override *TargetOverride) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cleanID := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(override.TargetID, "chatroom:"), "contact:"), "private:")
	override.TargetID = cleanID
	override.UpdatedAt = time.Now()
	sm.data.Overrides[cleanID] = override
	slog.Info("[dashboard] 保存对象定制配置", "target", cleanID, "type", override.TargetType)
	return sm.saveLocked()
}

func (sm *StoreManager) DeleteOverride(targetID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cleanID := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(targetID, "chatroom:"), "contact:"), "private:")
	delete(sm.data.Overrides, targetID)
	delete(sm.data.Overrides, cleanID)
	delete(sm.data.Overrides, "chatroom:"+cleanID)
	delete(sm.data.Overrides, "contact:"+cleanID)
	delete(sm.data.Overrides, "private:"+cleanID)
	slog.Info("[dashboard] 移除对象定制配置", "target", targetID)
	return sm.saveLocked()
}
