package main

import (
	"testing"
	"time"
)

func TestAiPlugin_ClearContextCapability(t *testing.T) {
	p := &AiPlugin{
		sessions:     make(map[string][]openAIMessage),
		sessionTimes: make(map[string]time.Time),
	}
	p.sessions["private:testuser"] = []openAIMessage{{Role: "user", Content: "hello"}}

	if len(p.sessions["private:testuser"]) == 0 {
		t.Fatal("expected session to have messages")
	}

	// 1. 测试缺少参数
	_, _, err := p.OnCall("ai.clear_context", map[string]string{})
	if err == nil {
		t.Error("expected error for missing session param, got nil")
	}

	// 2. 测试仅清空大模型短期记忆 (ai_memory 模式: 保留聊天记录，但上下文为空)
	respType, respBytes, err := p.OnCall("ai.clear_context", map[string]string{"session": "testuser", "mode": "ai_memory"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if respType != "text" || string(respBytes) != "ok" {
		t.Errorf("unexpected response: %s %s", respType, string(respBytes))
	}

	// 历史记录完整保留
	if len(p.sessions["private:testuser"]) != 1 {
		t.Errorf("expected session history to be preserved (1 message), got %d", len(p.sessions["private:testuser"]))
	}
	// 但给大模型的上下文已完全清空
	if len(p.contextMessages("private:testuser")) != 0 {
		t.Errorf("expected context to AI to be 0, got %d", len(p.contextMessages("private:testuser")))
	}

	// 3. 测试彻底清除 (complete 模式: 删除记录与记忆)
	_, _, err = p.OnCall("ai.clear_context", map[string]string{"session": "testuser", "mode": "complete"})
	if err != nil {
		t.Fatalf("unexpected error in complete mode: %v", err)
	}
	if len(p.sessions["private:testuser"]) != 0 {
		t.Errorf("expected session to be completely purged, still has %d messages", len(p.sessions["private:testuser"]))
	}
}
