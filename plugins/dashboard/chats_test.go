package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestChatEngine_ClearSessionContext(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "golem-test-clear-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	aiHistFile := filepath.Join(tmpDir, "ai_history.json")
	statDBFile := filepath.Join(tmpDir, "statistics.db")
	sentFile := filepath.Join(tmpDir, "dashboard_sent.json")

	// 1. 初始化模拟的 ai_history.json
	initialHist := AIHistoryFile{
		Version: 1,
		Sessions: map[string]AIHistorySession{
			"private:user123": {
				UpdatedAt: "2026-09-24T08:00:00+08:00",
				Messages: []AIHistoryMsg{
					{Role: "user", Content: "你好"},
					{Role: "assistant", Content: "你好呀"},
				},
			},
			"private:other": {
				UpdatedAt: "2026-09-24T08:00:00+08:00",
				Messages: []AIHistoryMsg{
					{Role: "user", Content: "留下的会话"},
				},
			},
		},
	}
	histBytes, _ := json.Marshal(initialHist)
	_ = os.WriteFile(aiHistFile, histBytes, 0644)

	// 2. 初始化模拟的 statistics.db
	db, err := sql.Open("sqlite", statDBFile)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE statistics (
			id INTEGER PRIMARY KEY,
			type TEXT,
			content TEXT,
			sender TEXT,
			receiver TEXT,
			member TEXT,
			raw TEXT,
			timestamp DATETIME
		);
		INSERT INTO statistics (id, type, content, sender, receiver, timestamp)
		VALUES (1, '文本', '你好', 'user123', 'bot', '2026-09-24 08:00:00');
		INSERT INTO statistics (id, type, content, sender, receiver, timestamp)
		VALUES (2, '文本', '你好呀', 'bot', 'user123', '2026-09-24 08:00:01');
		INSERT INTO statistics (id, type, content, sender, receiver, timestamp)
		VALUES (3, '文本', '其他人的消息', 'other', 'bot', '2026-09-24 08:00:02');
	`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}

	ce := NewChatEngine(aiHistFile, statDBFile, sentFile)
	ce.sentMessages["user123"] = []SentRecord{{Type: "text", Content: "代发消息"}}
	ce.saveSentStoreLocked()

	// 3. 测试模式一：ai_memory（清除AI记忆，保留聊天记录）
	msg, err := ce.ClearSessionContext(nil, "user123", "ai_memory")
	if err != nil {
		t.Fatalf("ClearSessionContext ai_memory error: %v", err)
	}
	if msg != "AI 记忆已成功清除，聊天记录已完整保留" {
		t.Errorf("unexpected msg: %s", msg)
	}

	// 验证 ai_history.json: user123 绝不应该被移除！且消息完整，设置了 ContextCutoff
	readData, _ := os.ReadFile(aiHistFile)
	var checkHist AIHistoryFile
	_ = json.Unmarshal(readData, &checkHist)
	sess, ok := checkHist.Sessions["private:user123"]
	if !ok {
		t.Fatal("expected private:user123 to remain in ai_history.json in ai_memory mode")
	}
	if len(sess.Messages) != 2 {
		t.Errorf("expected 2 messages in session, got %d", len(sess.Messages))
	}
	if sess.ContextCutoff != 2 {
		t.Errorf("expected ContextCutoff to be 2, got %d", sess.ContextCutoff)
	}
	if _, ok := checkHist.Sessions["private:other"]; !ok {
		t.Error("expected private:other to remain in ai_history.json")
	}

	// 验证 GetMessages 读取会话，消息完整（2条历史 + 1条代发）且附带 cutoff 和 cutoffAtEnd
	msgs, total, cutoff, cutoffAtEnd, err := ce.GetMessages(nil, "user123", 100, 0)
	if err != nil {
		t.Fatalf("GetMessages error: %v", err)
	}
	if total != 3 || len(msgs) != 3 {
		t.Errorf("expected 3 messages from GetMessages, got %d (total: %d)", len(msgs), total)
	}
	if cutoff != 2 {
		t.Errorf("expected cutoff to be 2, got %d", cutoff)
	}
	if !cutoffAtEnd {
		t.Errorf("expected cutoffAtEnd to be true, got %v", cutoffAtEnd)
	}

	// 验证 statistics.db: user123 的记录依然存在（保留聊天记录）
	db, _ = sql.Open("sqlite", statDBFile)
	var count int
	_ = db.QueryRow("SELECT COUNT(*) FROM statistics WHERE sender='user123' OR receiver='user123'").Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 messages preserved in statistics.db, got %d", count)
	}
	db.Close()

	// 4. 测试模式二：complete（彻底清除，同时删除聊天记录）
	msg, err = ce.ClearSessionContext(nil, "user123", "complete")
	if err != nil {
		t.Fatalf("ClearSessionContext complete error: %v", err)
	}
	if msg != "会话聊天记录与 AI 记忆已彻底清除" {
		t.Errorf("unexpected msg: %s", msg)
	}

	// 验证 ai_history.json 再次被清理
	readData, _ = os.ReadFile(aiHistFile)
	var checkHist2 AIHistoryFile
	_ = json.Unmarshal(readData, &checkHist2)
	if _, ok := checkHist2.Sessions["private:user123"]; ok {
		t.Error("expected private:user123 to be removed in complete mode")
	}

	// 验证 statistics.db: user123 的记录彻底被删除，other 的记录保留
	db, _ = sql.Open("sqlite", statDBFile)
	_ = db.QueryRow("SELECT COUNT(*) FROM statistics WHERE sender='user123' OR receiver='user123'").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 messages in statistics.db for user123, got %d", count)
	}
	var otherCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM statistics WHERE sender='other'").Scan(&otherCount)
	if otherCount != 1 {
		t.Errorf("expected other's message to remain, got %d", otherCount)
	}
	db.Close()

	// 验证 sentStore: user123 已被清理
	if len(ce.sentMessages["user123"]) != 0 {
		t.Error("expected sentMessages for user123 to be cleared")
	}
}
