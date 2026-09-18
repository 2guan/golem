package main

import (
	"log/slog"
	"strings"

	"github.com/sbgayhub/golem/sdk/chatroom"
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

type Config struct {
	BaseURL string `toml:"base_url" comment:"大模型接口地址"`
	APIKey  string `toml:"api_key" comment:"大模型 API Key"`
	Model   string `toml:"model" comment:"视觉模型名称"`
}

type AvatarReviewPlugin struct {
	plugin.ConfigAbility[Config]
	message  message.Ability
	contact  contact.Ability
	chatroom chatroom.Ability
}

func (p *AvatarReviewPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "avatar_review",
		Author:      "Golem Team",
		Version:     "1.0.0",
		Description: "头像锐评鉴赏家，提取群友头像调用多模态大模型进行趣味毒舌打分",
		Priority:    -20,
	}
}

func (p *AvatarReviewPlugin) GetSubscriptions() []string {
	return []string{
		message.TypeText.Topic,
	}
}

func (p *AvatarReviewPlugin) OnEvent(e *plugin.Event) (bool, error) {
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
	case "头像评测帮助", "头像帮助":
		p.sendText(receiver, "🎨【头像锐评家】用法：\n\n"+
			"直接发送【评测头像】/【锐评头像】/【头像打分】（可 @肉丸）：\n"+
			"肉丸会调取你的专属高清头像，从构图、色调、气质等多维度进行 0-100 分毒舌鉴赏与改造建议！")
		return true, nil

	case "评测头像", "锐评头像", "看头像", "头像打分", "测头像", "打分头像", "我的头像", "看看头像":
		userID, nickname, avatarURL := p.getUserInfoAndAvatar(e, msg)
		slog.Info("[avatar_review] 收到头像评测请求", "user", userID, "nickname", nickname, "avatar", avatarURL)

		report := p.reviewAvatar(avatarURL, nickname)
		p.sendText(receiver, report)
		return true, nil
	}

	return false, nil
}

func (p *AvatarReviewPlugin) getUserInfoAndAvatar(e *plugin.Event, msg *message.Message) (string, string, string) {
	userID := ""
	nickname := ""
	avatarURL := ""

	if msg.Member != nil {
		userID = msg.Member.Username
		nickname = msg.Member.DisplayName
		if nickname == "" {
			nickname = msg.Member.Nickname
		}
		if msg.Member.Avatar != "" {
			avatarURL = msg.Member.Avatar
		}
	}

	if userID == "" {
		userID = e.GetSender()
	}

	if c := p.contact.Get(userID); c != nil {
		if nickname == "" {
			nickname = c.GetNickname()
		}
		if avatarURL == "" {
			avatarURL = c.GetAvatar()
		}
	}

	// 检查当前群聊里的成员列表
	chatroomID := e.GetSender()
	if strings.HasSuffix(chatroomID, "@chatroom") && p.chatroom != nil {
		if member := p.chatroom.GetMember(chatroomID, userID); member != nil {
			if nickname == "" {
				nickname = member.DisplayName
				if nickname == "" {
					nickname = member.Nickname
				}
			}
			if avatarURL == "" {
				avatarURL = member.Avatar
			}
		}
	}

	if nickname == "" {
		nickname = "群友"
	}

	return userID, nickname, avatarURL
}

func (p *AvatarReviewPlugin) resolveReceiver(e *plugin.Event, msg *message.Message) *contact.Contact {
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

func (p *AvatarReviewPlugin) sendText(receiver *contact.Contact, text string) {
	msg := &message.Message{
		Type:     message.TypeText,
		Receiver: receiver,
		Content:  text,
		Data:     &message.Message_Text{Text: &message.TextData{Content: text}},
	}
	if _, err := p.message.Send(msg); err != nil {
		slog.Error("[avatar_review] 发送消息失败", "err", err)
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
	p := &AvatarReviewPlugin{
		ConfigAbility: plugin.ConfigAbility[Config]{
			Config: Config{
				BaseURL: "https://token-plan-cn.xiaomimimo.com/v1",
				APIKey:  "tp-ckyf7ojdhr6al7jx9o7wdycdbbfhrmz6wogkvwjocjt8xjkd",
				Model:   "mimo-v2.5",
			},
		},
	}
	slog.Info("[avatar_review] 头像锐评插件启动中...")
	plugin.Start(p)
}
