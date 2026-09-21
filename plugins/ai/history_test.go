package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestHistory_IsolationAndPersistenceAcrossRestart(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "test_ai_history.json")

	// === 阶段 1：模拟服务运行，有多个用户和群聊发送消息 ===
	p1 := &AiPlugin{
		sessions:     map[string][]openAIMessage{},
		sessionTimes: map[string]time.Time{},
		recentImages: map[string]*cachedImage{},
	}
	p1.Config = Config{
		HistoryFile:        historyFile,
		HistoryExpireHours: 72,
		MaxContextMessages: 10,
	}

	sessionA := "private:wxid_tino"
	sessionB := "private:wxid_guan"
	sessionGroup := "chatroom:12345678@chatroom"

	// 用户 A (Tino) 发送消息
	p1.appendContext(sessionA, openAIMessage{Role: "user", Content: "你好肉丸，我是Tino"})
	p1.appendContext(sessionA, openAIMessage{Role: "assistant", Content: "你好Tino，今天过得怎么样？"})

	// 用户 B (管管) 发送消息
	p1.appendContext(sessionB, openAIMessage{Role: "user", Content: "肉丸，今晚去推胸吗？"})
	p1.appendContext(sessionB, openAIMessage{Role: "assistant", Content: "好嘞，老时间老地方见！"})

	// 群聊 发送消息
	p1.appendContext(sessionGroup, openAIMessage{Role: "user", Content: "群友: 今天天气真好"})
	p1.appendContext(sessionGroup, openAIMessage{Role: "assistant", Content: "确实，适合出去走走"})

	// 验证内存中各会话严格独立
	msgsA := p1.contextMessages(sessionA)
	msgsB := p1.contextMessages(sessionB)
	msgsGroup := p1.contextMessages(sessionGroup)

	if len(msgsA) != 2 || msgsA[0].Content != "你好肉丸，我是Tino" {
		t.Fatalf("sessionA messages mismatch: %v", msgsA)
	}
	if len(msgsB) != 2 || msgsB[0].Content != "肉丸，今晚去推胸吗？" {
		t.Fatalf("sessionB messages mismatch: %v", msgsB)
	}
	if len(msgsGroup) != 2 || msgsGroup[0].Content != "群友: 今天天气真好" {
		t.Fatalf("sessionGroup messages mismatch: %v", msgsGroup)
	}

	// 等待异步落盘写入完成
	time.Sleep(100 * time.Millisecond)

	// === 阶段 2：模拟服务重启（重新初始化 AiPlugin 实例，内存归零，从磁盘加载） ===
	p2 := &AiPlugin{
		sessions:     map[string][]openAIMessage{},
		sessionTimes: map[string]time.Time{},
		recentImages: map[string]*cachedImage{},
	}
	p2.Config = Config{
		HistoryFile:        historyFile,
		HistoryExpireHours: 72,
		MaxContextMessages: 10,
	}
	p2.ensureHistoryLoaded()

	// 验证重启后，各会话历史完整恢复且互相不串台
	restoredA := p2.contextMessages(sessionA)
	restoredB := p2.contextMessages(sessionB)
	restoredGroup := p2.contextMessages(sessionGroup)

	if len(restoredA) != 2 {
		t.Fatalf("expected 2 messages for sessionA after restart, got %d", len(restoredA))
	}
	if restoredA[0].Content != "你好肉丸，我是Tino" || restoredA[1].Content != "你好Tino，今天过得怎么样？" {
		t.Errorf("sessionA content mismatch: %v", restoredA)
	}

	if len(restoredB) != 2 {
		t.Fatalf("expected 2 messages for sessionB after restart, got %d", len(restoredB))
	}
	if restoredB[0].Content != "肉丸，今晚去推胸吗？" || restoredB[1].Content != "好嘞，老时间老地方见！" {
		t.Errorf("sessionB content mismatch: %v", restoredB)
	}

	if len(restoredGroup) != 2 {
		t.Fatalf("expected 2 messages for sessionGroup after restart, got %d", len(restoredGroup))
	}
	if restoredGroup[0].Content != "群友: 今天天气真好" {
		t.Errorf("sessionGroup content mismatch: %v", restoredGroup)
	}

	// === 阶段 3：清理某单个会话，验证不影响其他会话 ===
	p2.clearContext(sessionA)
	time.Sleep(100 * time.Millisecond)

	p3 := &AiPlugin{
		sessions:     map[string][]openAIMessage{},
		sessionTimes: map[string]time.Time{},
		recentImages: map[string]*cachedImage{},
	}
	p3.Config = Config{
		HistoryFile:        historyFile,
		HistoryExpireHours: 72,
		MaxContextMessages: 10,
	}
	p3.ensureHistoryLoaded()

	if len(p3.contextMessages(sessionA)) != 0 {
		t.Errorf("sessionA should be cleared, but got: %v", p3.contextMessages(sessionA))
	}
	if len(p3.contextMessages(sessionB)) != 2 {
		t.Errorf("sessionB should remain intact, but got: %v", p3.contextMessages(sessionB))
	}
	if len(p3.contextMessages(sessionGroup)) != 2 {
		t.Errorf("sessionGroup should remain intact, but got: %v", p3.contextMessages(sessionGroup))
	}
}

func TestHistory_Expiration(t *testing.T) {
	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "test_expire_history.json")

	p1 := &AiPlugin{
		sessions:     map[string][]openAIMessage{},
		sessionTimes: map[string]time.Time{},
		recentImages: map[string]*cachedImage{},
	}
	p1.Config = Config{
		HistoryFile:        historyFile,
		HistoryExpireHours: 24, // 24小时过期
		MaxContextMessages: 10,
	}

	activeSession := "private:wxid_active"
	expiredSession := "private:wxid_expired"

	p1.appendContext(activeSession, openAIMessage{Role: "user", Content: "最新消息"})
	p1.appendContext(expiredSession, openAIMessage{Role: "user", Content: "老旧消息"})

	// 手动将 expiredSession 的时间篡改为 50 小时前
	p1.mu.Lock()
	p1.sessionTimes[expiredSession] = time.Now().Add(-50 * time.Hour)
	p1.mu.Unlock()

	p1.saveHistorySync()

	// 重启实例
	p2 := &AiPlugin{
		sessions:     map[string][]openAIMessage{},
		sessionTimes: map[string]time.Time{},
		recentImages: map[string]*cachedImage{},
	}
	p2.Config = Config{
		HistoryFile:        historyFile,
		HistoryExpireHours: 24,
		MaxContextMessages: 10,
	}
	p2.ensureHistoryLoaded()

	if len(p2.contextMessages(activeSession)) != 1 {
		t.Errorf("activeSession should be preserved, got %d", len(p2.contextMessages(activeSession)))
	}
	if len(p2.contextMessages(expiredSession)) != 0 {
		t.Errorf("expiredSession should be pruned, got %d", len(p2.contextMessages(expiredSession)))
	}
}
