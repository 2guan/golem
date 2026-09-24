package main

import (
	"log/slog"
	"strings"
	"time"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

type Config struct {
	AutoPushChatrooms []string `toml:"auto_push_chatrooms" comment:"工作日10:00自动推送的群聊ID列表"`
}

type MoyuDailyPlugin struct {
	plugin.ConfigAbility[Config]
	message  message.Ability
	contact  contact.Ability
	stopCh   chan struct{}
	lastPush string
}

func (p *MoyuDailyPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "moyu_daily",
		Author:      "Golem Team",
		Version:     "1.0.0",
		Description: "打工人摸鱼办日报插件，提供周末/发薪/假期倒计时与防内卷摸鱼箴言",
		Priority:    -10,
	}
}

func (p *MoyuDailyPlugin) GetSubscriptions() []string {
	return []string{
		message.TypeText.Topic,
	}
}

func (p *MoyuDailyPlugin) OnLoad() error {
	p.stopCh = make(chan struct{})
	go p.scheduleRoutine()
	return nil
}

func (p *MoyuDailyPlugin) OnUnload() error {
	if p.stopCh != nil {
		close(p.stopCh)
	}
	return nil
}

func (p *MoyuDailyPlugin) scheduleRoutine() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case now := <-ticker.C:
			// 周一至周五工作日 10:00 自动推送
			if now.Weekday() >= time.Monday && now.Weekday() <= time.Friday {
				if now.Hour() == 10 && now.Minute() == 0 {
					today := now.Format("2006-01-02")
					if p.lastPush != today {
						p.lastPush = today
						p.pushToConfiguredChatrooms()
					}
				}
			}
		}
	}
}

func (p *MoyuDailyPlugin) pushToConfiguredChatrooms() {
	if len(p.Config.AutoPushChatrooms) == 0 {
		return
	}
	content := GenerateMoyuDaily()
	for _, cid := range p.Config.AutoPushChatrooms {
		cid = strings.TrimSpace(cid)
		if cid != "" {
			receiver := &contact.Contact{Username: cid}
			p.sendText(receiver, content)
			slog.Info("[moyu_daily] 工作日10:00摸鱼日报推送成功", "chatroom", cid)
		}
	}
}

func (p *MoyuDailyPlugin) OnEvent(e *plugin.Event) (bool, error) {
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

	cleanText, _ := cleanMention(rawText)

	receiver := p.resolveReceiver(e, msg)
	if receiver == nil {
		return false, nil
	}

	switch cleanText {
	case "摸鱼帮助", "摸鱼办帮助":
		p.sendText(receiver, "🐟【打工人摸鱼办】说明：\n\n"+
			"直接发送【摸鱼办】/【摸鱼日报】/【摸鱼日历】/【摸鱼倒计时】：\n"+
			"机器人会立即为你测算距离本周末、发工资日、下一个法定假期的精准天数，并附赠一条防内卷扎心箴言！")
		return true, nil

	case "摸鱼办", "摸鱼日报", "摸鱼日历", "摸鱼倒计时", "今日摸鱼", "摸鱼周报", "摸鱼":
		report := GenerateMoyuDaily()
		p.sendText(receiver, report)
		return true, nil
	}

	return false, nil
}

func (p *MoyuDailyPlugin) resolveReceiver(e *plugin.Event, msg *message.Message) *contact.Contact {
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

func (p *MoyuDailyPlugin) sendText(receiver *contact.Contact, text string) {
	msg := &message.Message{
		Type:     message.TypeText,
		Receiver: receiver,
		Content:  text,
		Data:     &message.Message_Text{Text: &message.TextData{Content: text}},
	}
	if _, err := p.message.Send(msg); err != nil {
		slog.Error("[moyu_daily] 发送消息失败", "err", err)
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
	p := &MoyuDailyPlugin{
		ConfigAbility: plugin.ConfigAbility[Config]{
			Config: Config{
				AutoPushChatrooms: []string{},
			},
		},
	}
	slog.Info("[moyu_daily] 打工人摸鱼办插件启动中...")
	plugin.Start(p)
}
