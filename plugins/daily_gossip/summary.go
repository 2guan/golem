package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

type ChatMessage struct {
	SenderID   string
	SenderName string
	Content    string
	Time       time.Time
}

type GossipTracker struct {
	mu       sync.RWMutex
	messages map[string][]ChatMessage // chatroomID -> messages
	lastPush map[string]string        // chatroomID -> date string "2006-01-02"
}

func NewGossipTracker() *GossipTracker {
	return &GossipTracker{
		messages: make(map[string][]ChatMessage),
		lastPush: make(map[string]string),
	}
}

func (t *GossipTracker) Record(chatroomID, senderID, senderName, content string) {
	// 忽略系统提示与指令
	c := strings.TrimSpace(content)
	if c == "" || strings.HasPrefix(c, "/") || strings.HasPrefix(c, "今日八卦") || strings.HasPrefix(c, "群八卦") {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	msgs := t.messages[chatroomID]
	// 保留最近 24 小时
	cutoff := time.Now().Add(-24 * time.Hour)
	valid := make([]ChatMessage, 0, len(msgs)+1)
	for _, m := range msgs {
		if m.Time.After(cutoff) {
			valid = append(valid, m)
		}
	}

	// 限制最多 400 条
	if len(valid) >= 400 {
		valid = valid[len(valid)-399:]
	}

	valid = append(valid, ChatMessage{
		SenderID:   senderID,
		SenderName: senderName,
		Content:    c,
		Time:       time.Now(),
	})

	t.messages[chatroomID] = valid
}

func (t *GossipTracker) GetMessages(chatroomID string) []ChatMessage {
	t.mu.RLock()
	defer t.mu.RUnlock()
	msgs := t.messages[chatroomID]
	res := make([]ChatMessage, len(msgs))
	copy(res, msgs)
	return res
}

func (t *GossipTracker) ShouldPush(chatroomID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if t.lastPush[chatroomID] == today {
		return false
	}
	// 检查是否有足够的聊天记录（至少 5 条）
	if len(t.messages[chatroomID]) < 5 {
		return false
	}
	t.lastPush[chatroomID] = today
	return true
}

func (p *DailyGossipPlugin) generateGossipReport(chatroomID string) string {
	msgs := p.tracker.GetMessages(chatroomID)
	if len(msgs) < 3 {
		return "🍵 今日群里太安静啦，大家都在认真摸鱼，暂无八卦大瓜可吃！群友们多聊两句再来发【今日八卦】吧！"
	}

	// 统计活跃发信人
	counts := make(map[string]int)
	nameMap := make(map[string]string)
	for _, m := range msgs {
		counts[m.SenderID]++
		nameMap[m.SenderID] = m.SenderName
	}

	type userStat struct {
		name  string
		count int
	}
	var stats []userStat
	for uid, cnt := range counts {
		stats = append(stats, userStat{name: nameMap[uid], count: cnt})
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].count > stats[j].count
	})

	topSpammer := "神秘群友"
	topCount := 0
	if len(stats) > 0 {
		topSpammer = stats[0].name
		topCount = stats[0].count
	}

	// 组装文本供 AI 总结
	var sb strings.Builder
	for i, m := range msgs {
		if i > 150 {
			break // 截取前 150 条精选
		}
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", m.Time.Format("15:04"), m.SenderName, m.Content))
	}

	// 优先调用 ai.chat 能力
	if p.caller != nil {
		report, err := p.callAIGossip(sb.String(), topSpammer, topCount, len(msgs))
		if err == nil && strings.TrimSpace(report) != "" {
			return report
		}
		slog.Warn("[daily_gossip] AI 提炼失败，降级统计报表", "err", err)
	}

	// 降级模版生成
	return fmt.Sprintf("📰【今日群聊八卦日报】\n\n"+
		"📊 今日活跃数据：\n"+
		"• 本群今日累计发言：%d 条\n"+
		"• 🏆 今日水神之王：【%s】（疯狂输出 %d 条）\n\n"+
		"💬 随机抓包金句：\n“%s”\n\n"+
		"🕵️‍♂️ 肉丸锐评：\n本群今天蒸蒸日上，大家摸鱼热情高涨，继续保持！",
		len(msgs), topSpammer, topCount, msgs[len(msgs)/2].Content)
}

func (p *DailyGossipPlugin) callAIGossip(chatContext, topSpammer string, topCount, total int) (string, error) {
	systemPrompt := `你现在是《今日群聊吃瓜大戏·八卦晚报》的资深总编“肉丸”（北京老哥，前职业电竞老将，幽默风趣、嘴碎诙谐、略带调侃但充满善意）。
你的名字叫“肉丸”，平时自称“肉丸”或“我”，只有在明确遇到年轻人、学生或小孩时才可以自称“叔叔”，其它任何正常情况下绝不要自称叔叔。
请根据今天群友的真实聊天记录，撰写一份排版清晰、笑点拉满的【今日群聊吃瓜日报】。

要求输出格式必须严格包含以下几个模块（直接输出，不要带markdown代码块）：
🗞️【今日群聊吃瓜日报 · 肉丸晚报】
📅 日期：2026年X月X日

🔥【今日头条大瓜】：
（用极其夸张搞笑、港媒/娱乐小报的笔触，浓缩总结今天群里最核心、争论最多或最离谱的一件事）

🏆【今日水神劳模榜】：
（点名水神发言最多的群友，结合其发言幽默调侃一句）

💬【爆笑语录大赏】：
（从聊天记录里摘抄 1-2 句最具杀伤力、最搞笑或最具哲理的原话，并附带一句简评）

🕵️‍♂️【肉丸独家结语】：
（用肉丸标志性的老将口吻，写 2 句总结性吐槽与鼓励，自称肉丸或我，不要自称叔叔）`

	type openAIMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type aiChatPayload struct {
		System   string      `json:"system"`
		Messages []openAIMsg `json:"messages"`
	}

	payload := aiChatPayload{
		System: systemPrompt,
		Messages: []openAIMsg{
			{
				Role:    "user",
				Content: fmt.Sprintf("今日统计：总消息数 %d 条，最高发言者 %s 发了 %d 条。\n聊天记录如下：\n%s", total, topSpammer, topCount, chatContext),
			},
		},
	}

	b, _ := json.Marshal(payload)
	_, resBytes, err := p.caller.CallPlugin("ai.chat", map[string]string{
		"payload": string(b),
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(resBytes)), nil
}
