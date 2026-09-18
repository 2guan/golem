package main

import (
	"log/slog"
	"strings"
	"time"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

type DailyGossipPlugin struct {
	message message.Ability
	contact contact.Ability
	caller  plugin.CallerAbility
	tracker *GossipTracker
	stopCh  chan struct{}
}

func (p *DailyGossipPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "daily_gossip",
		Author:      "Golem Team",
		Version:     "1.0.0",
		Description: "每日群聊八卦小报，自动监听提炼群内精彩吃瓜话题与水群榜",
		Priority:    0,
	}
}

func (p *DailyGossipPlugin) GetSubscriptions() []string {
	return []string{
		message.TypeText.Topic,
	}
}

func (p *DailyGossipPlugin) OnLoad() error {
	p.stopCh = make(chan struct{})
	go p.scheduleRoutine()
	return nil
}

func (p *DailyGossipPlugin) OnUnload() error {
	if p.stopCh != nil {
		close(p.stopCh)
	}
	return nil
}

func (p *DailyGossipPlugin) scheduleRoutine() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case now := <-ticker.C:
			// 每天 22:00 定时自动推送
			if now.Hour() == 22 && now.Minute() == 0 {
				p.pushAllChatrooms()
			}
		}
	}
}

func (p *DailyGossipPlugin) pushAllChatrooms() {
	p.tracker.mu.RLock()
	var chatrooms []string
	for cid := range p.tracker.messages {
		chatrooms = append(chatrooms, cid)
	}
	p.tracker.mu.RUnlock()

	for _, cid := range chatrooms {
		if p.tracker.ShouldPush(cid) {
			report := p.generateGossipReport(cid)
			receiver := &contact.Contact{Username: cid}
			p.sendText(receiver, report)
			slog.Info("[daily_gossip] 22:00 定时推送八卦日报成功", "chatroom", cid)
		}
	}
}

func (p *DailyGossipPlugin) OnEvent(e *plugin.Event) (bool, error) {
	msg := e.Payload.(*plugin.Event_Message).Message
	if msg == nil {
		return false, nil
	}

	rawText := strings.TrimSpace(msg.GetContent())
	if rawText == "" {
		if td := msg.GetText(); td != nil {
			rawText = strings.TrimSpace(td.Content)
		}
	}
	if rawText == "" {
		return false, nil
	}

	chatroomID := e.GetSender()
	if chatroomID == "" && msg.Sender != nil {
		chatroomID = msg.Sender.GetUsername()
	}

	// 仅群聊场景记录与触发
	if !strings.HasSuffix(chatroomID, "@chatroom") {
		return false, nil
	}

	senderID, senderName := p.getUserInfo(e, msg)
	cleanText, _ := cleanMention(rawText)

	receiver := p.resolveReceiver(e, msg)
	if receiver == nil {
		return false, nil
	}

	switch cleanText {
	case "八卦帮助", "吃瓜帮助":
		p.sendText(receiver, "🗞️【群聊八卦晚报说明】\n\n"+
			"1. 发送【今日八卦】/【群八卦】/【吃瓜日报】：随时生成并查看本群今日专属八卦吃瓜报！\n"+
			"2. 每晚 22:00 肉丸会自动为全天聊过天的小群排版推送当天的吃瓜晚报！")
		return true, nil

	case "今日八卦", "群八卦", "吃瓜日报", "八卦日报", "吃瓜晚报", "群报":
		report := p.generateGossipReport(chatroomID)
		p.sendText(receiver, report)
		return true, nil
	}

	// 记录日常群聊消息到 tracker
	p.tracker.Record(chatroomID, senderID, senderName, rawText)

	return false, nil
}

func (p *DailyGossipPlugin) getUserInfo(e *plugin.Event, msg *message.Message) (string, string) {
	userID := ""
	nickname := ""

	if msg.Member != nil && msg.Member.Username != "" {
		userID = msg.Member.Username
		nickname = msg.Member.DisplayName
		if nickname == "" {
			nickname = msg.Member.Nickname
		}
	}

	if userID == "" {
		userID = e.GetSender()
	}
	if nickname == "" {
		if c := p.contact.Get(userID); c != nil {
			nickname = c.GetNickname()
		}
	}
	if nickname == "" {
		nickname = "群友"
	}

	return userID, nickname
}

func (p *DailyGossipPlugin) resolveReceiver(e *plugin.Event, msg *message.Message) *contact.Contact {
	receiver := p.contact.Get(e.GetSender())
	if receiver == nil {
		if msg.Sender != nil && msg.Sender.GetUsername() == e.GetSender() {
			receiver = msg.Sender
		} else if msg.Receiver != nil && msg.Receiver.GetUsername() == e.GetSender() {
			receiver = msg.Receiver
		} else {
			receiver = &contact.Contact{Username: e.GetSender()}
		}
	}
	return receiver
}

func (p *DailyGossipPlugin) sendText(receiver *contact.Contact, text string) {
	msg := &message.Message{
		Type:     message.TypeText,
		Receiver: receiver,
		Content:  text,
		Data:     &message.Message_Text{Text: &message.TextData{Content: text}},
	}
	if _, err := p.message.Send(msg); err != nil {
		slog.Error("[daily_gossip] 发送消息失败", "err", err)
	}
}

func cleanMention(content string) (string, bool) {
	text := strings.TrimSpace(content)
	hadMention := false
	for strings.HasPrefix(text, "@") {
		hadMention = true
		idx := strings.IndexAny(text, " \t\r\n\u2005\u00a0:：,，")
		if idx > 0 {
			text = strings.TrimSpace(text[idx:])
			text = strings.TrimLeft(text, " :：,，\t\r\n\u2005\u00a0")
		} else {
			break
		}
	}
	if idx := strings.LastIndex(text, "@"); idx > 0 {
		hadMention = true
		text = strings.TrimSpace(text[:idx])
	}
	return strings.TrimSpace(text), hadMention
}

func main() {
	p := &DailyGossipPlugin{
		tracker: NewGossipTracker(),
	}
	slog.Info("[daily_gossip] 每日群聊八卦插件启动中...")
	plugin.Start(p)
}
