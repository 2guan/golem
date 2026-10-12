package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sbgayhub/golem/sdk/contact"
)


func TestDailyStatsRollover(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "test_stats.json")

	p := &AddFriendPlugin{}
	p.Config.DailyLimit = 3
	p.Config.DataFile = dataFile

	// 模拟昨天的记录
	p.stats = DailyStats{
		Date:  "2026-10-08",
		Count: 3,
		Logs:  []string{"昨天的记录"},
	}
	p.saveStatsLocked()

	// 加载并触发日期滚动检查
	p.loadStats()
	p.checkDateRolloverLocked()

	today := time.Now().Format("2006-01-02")
	if p.stats.Date != today {
		t.Fatalf("expected date %s, got %s", today, p.stats.Date)
	}
	if p.stats.Count != 0 {
		t.Fatalf("expected count 0 after rollover, got %d", p.stats.Count)
	}
	if len(p.stats.Logs) != 0 {
		t.Fatalf("expected empty logs after rollover, got %d", len(p.stats.Logs))
	}
}

func TestQuotaAcquireAndRollback(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "test_stats.json")

	p := &AddFriendPlugin{}
	p.Config.DailyLimit = 2
	p.Config.DataFile = dataFile

	p.loadStats()

	// 第一次申请
	ok, remaining := p.acquireQuota()
	if !ok || remaining != 1 {
		t.Fatalf("expected acquire success with remaining 1, got ok=%v, remaining=%d", ok, remaining)
	}

	// 第二次申请
	ok, remaining = p.acquireQuota()
	if !ok || remaining != 0 {
		t.Fatalf("expected acquire success with remaining 0, got ok=%v, remaining=%d", ok, remaining)
	}

	// 第三次申请超限
	ok, _ = p.acquireQuota()
	if ok {
		t.Fatalf("expected acquire fail when limit reached")
	}

	// 回退一次
	p.rollbackQuota()
	if p.stats.Count != 1 {
		t.Fatalf("expected count 1 after rollback, got %d", p.stats.Count)
	}

	// 再次申请应成功
	ok, remaining = p.acquireQuota()
	if !ok || remaining != 0 {
		t.Fatalf("expected acquire success after rollback, got ok=%v, remaining=%d", ok, remaining)
	}
}

func TestGetDefaultGreeting(t *testing.T) {
	p := &AddFriendPlugin{}
	// 当未设置 contact ability 时默认回退到 Bot
	if p.getDefaultGreeting() != "你好，我是Bot" {
		t.Fatalf("unexpected default greeting: %s", p.getDefaultGreeting())
	}

	p.Config.DefaultGreeting = "你好，我是{nickname}"
	if p.getDefaultGreeting() != "你好，我是Bot" {
		t.Fatalf("template greeting not replaced: %s", p.getDefaultGreeting())
	}

	p.Config.DefaultGreeting = "你好我是主人小助手"
	if p.getDefaultGreeting() != "你好我是主人小助手" {
		t.Fatalf("custom greeting not used: %s", p.getDefaultGreeting())
	}
}

type mockContactAbility struct {
	contact.Ability
	friends []*contact.Contact
}

func (m *mockContactAbility) List() []*contact.Contact {
	return m.friends
}

func TestIsAlreadyFriend(t *testing.T) {
	p := &AddFriendPlugin{
		contact: &mockContactAbility{
			friends: []*contact.Contact{
				{Username: "wxid_111", Alias: "alias_aaa"},
				{Username: "wxid_222", Nickname: "小李"},
				{Username: ""}, // 无效联系人
			},
		},
	}

	if !p.isAlreadyFriend("wxid_111") {
		t.Errorf("expected wxid_111 to be friend")
	}
	if !p.isAlreadyFriend("alias_aaa") {
		t.Errorf("expected alias_aaa to be friend")
	}
	if p.isAlreadyFriend("zhao_raul") {
		t.Errorf("expected zhao_raul NOT to be friend")
	}
	if p.isAlreadyFriend("") {
		t.Errorf("expected empty string NOT to be friend")
	}
}

func TestCleanGreeting(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"[你好，我是管管]", "你好，我是管管"},
		{"【你好，我是管管】", "你好，我是管管"},
		{"[小赵你好，我是管管的肉丸]", "小赵你好，我是管管的肉丸"},
		{"[[嵌套括号]]", "嵌套括号"},
		{"普通附言", "普通附言"},
		{"", ""},
	}

	for _, c := range cases {
		got := cleanGreeting(c.input)
		if got != c.expected {
			t.Errorf("cleanGreeting(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

