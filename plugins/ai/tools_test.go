package main

import (
	"strings"
	"testing"
	"time"

	"github.com/sbgayhub/golem/sdk/contact"
)

func TestIsOwnerPrivateSession(t *testing.T) {
	p := &AiPlugin{
		owner: &contact.Contact{
			Username: "wxid_owner",
			Nickname: "主人",
		},
	}

	tests := []struct {
		sessionKey string
		expected   bool
	}{
		{"private:wxid_owner", true},
		{"private:wxid_owner_fake", false},
		{"private:neu_lyk2010", false},
		{"chatroom:18351364439@chatroom", false},
		{"private:", false},
		{"wxid_owner", false},
	}

	for _, tc := range tests {
		got := p.isOwnerPrivateSession(tc.sessionKey)
		if got != tc.expected {
			t.Errorf("isOwnerPrivateSession(%q) = %v; want %v", tc.sessionKey, got, tc.expected)
		}
	}
}

func TestBuildOwnerBriefing(t *testing.T) {
	now := time.Now()
	p := &AiPlugin{
		owner: &contact.Contact{
			Username: "wxid_owner",
			Nickname: "主人",
		},
		sessionTimes: map[string]time.Time{
			"private:wxid_owner":            now,
			"private:neu_lyk2010":           now.Add(-10 * time.Minute),
			"chatroom:18351364439@chatroom": now.Add(-5 * time.Minute),
		},
		sessions: map[string][]openAIMessage{
			"private:wxid_owner": {
				{Role: "user", Content: "早上好啊"},
			},
			"private:neu_lyk2010": {
				{Role: "user", Content: "今天中午吃什么呢？"},
				{Role: "assistant", Content: "去吃大食堂吧"},
			},
			"chatroom:18351364439@chatroom": {
				{Role: "user", Content: "周五晚上开黑走起"},
			},
			"private:empty_user": {},
		},
	}

	briefing := p.buildOwnerBriefing()

	if strings.Contains(briefing, "wxid_owner") {
		t.Errorf("briefing should NOT include owner's private session: %s", briefing)
	}
	if strings.Contains(briefing, "empty_user") {
		t.Errorf("briefing should NOT include empty sessions: %s", briefing)
	}
	if !strings.Contains(briefing, "neu_lyk2010") {
		t.Errorf("briefing should include neu_lyk2010 session: %s", briefing)
	}
	if !strings.Contains(briefing, "18351364439@chatroom") {
		t.Errorf("briefing should include chatroom session: %s", briefing)
	}
	if !strings.Contains(briefing, "周五晚上开黑走起") {
		t.Errorf("briefing should contain snippet '周五晚上开黑走起': %s", briefing)
	}
}

func TestExecuteOwnerTool(t *testing.T) {
	now := time.Now()
	p := &AiPlugin{
		owner: &contact.Contact{
			Username: "wxid_owner",
			Nickname: "主人",
		},
		sessionTimes: map[string]time.Time{
			"private:wxid_owner":          now,
			"private:alice":               now.Add(-30 * time.Minute),
			"chatroom:game_room@chatroom": now.Add(-15 * time.Minute),
		},
		sessions: map[string][]openAIMessage{
			"private:wxid_owner": {
				{Role: "user", Content: "帮我看看，其他人聊啥呢"},
			},
			"private:alice": {
				{Role: "user", Content: "北京今天风好大，冷死了"},
				{Role: "assistant", Content: "多穿点衣服，别冻感冒了"},
			},
			"chatroom:game_room@chatroom": {
				{Role: "user", Content: "【Bob】: 今晚打瓦不打？"},
				{Role: "user", Content: "【Charlie】: 打打打，拉我！"},
			},
		},
	}

	// 1. 测试 list_all_chats
	listRes := p.executeTool("list_all_chats", `{}`)
	if !strings.Contains(listRes, "alice") || !strings.Contains(listRes, "game_room@chatroom") {
		t.Fatalf("list_all_chats output missing target sessions: %s", listRes)
	}

	// 2. 测试 get_chat_history
	histRes := p.executeTool("get_chat_history", `{"target": "alice", "limit": 10}`)
	if !strings.Contains(histRes, "北京今天风好大") || !strings.Contains(histRes, "多穿点衣服") {
		t.Fatalf("get_chat_history failed to get alice messages: %s", histRes)
	}

	// 3. 测试 search_chat_messages
	searchRes := p.executeTool("search_chat_messages", `{"keyword": "打瓦", "limit": 5}`)
	if !strings.Contains(searchRes, "game_room@chatroom") || !strings.Contains(searchRes, "今晚打瓦不打") {
		t.Fatalf("search_chat_messages failed to match keyword: %s", searchRes)
	}

	// 4. 测试未知工具
	unknownRes := p.executeTool("non_existent_tool", `{}`)
	if !strings.Contains(unknownRes, "未知工具") {
		t.Fatalf("executeTool should return error for unknown tool, got: %s", unknownRes)
	}
}

func TestResolveDisplayNameWithContact(t *testing.T) {
	p := &AiPlugin{
		owner: &contact.Contact{
			Username: "wxid_owner",
			Nickname: "主人",
		},
	}

	// 当 contact 为 nil 时，应安全回退到原始 ID
	name, isChat := p.resolveDisplayName("private:unknown_user")
	if name != "unknown_user" || isChat {
		t.Errorf("expected 'unknown_user', false; got %q, %v", name, isChat)
	}

	chatName, isChat := p.resolveDisplayName("chatroom:12345@chatroom")
	if chatName != "12345@chatroom" || !isChat {
		t.Errorf("expected '12345@chatroom', true; got %q, %v", chatName, isChat)
	}
}
