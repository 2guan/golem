package main

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/sbgayhub/golem/sdk/chatroom"
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

type UndercoverPlugin struct {
	message  message.Ability
	contact  contact.Ability
	chatroom chatroom.Ability
	manager  *GameManager
}

func (p *UndercoverPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "undercover",
		Author:      "Golem Team",
		Version:     "1.0.0",
		Description: "谁是卧底聚会桌游插件，支持自动报名、私聊分发词语、轮流发言与投票放逐",
		Priority:    -40,
	}
}

func (p *UndercoverPlugin) GetSubscriptions() []string {
	return []string{
		message.TypeText.Topic,
	}
}

func (p *UndercoverPlugin) OnEvent(e *plugin.Event) (bool, error) {
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

	// 仅支持群聊
	if !strings.HasSuffix(chatroomID, "@chatroom") {
		return false, nil
	}

	receiver := p.resolveReceiver(e, msg)
	if receiver == nil {
		return false, nil
	}

	cleanText, _ := cleanMention(rawText)

	// 获取真实发送者 ID 与群昵称
	userID, userNickname := p.getUserInfo(e, msg)

	switch cleanText {
	case "谁是卧底帮助":
		p.sendText(receiver, "🕵️【谁是卧底】游戏指南：\n\n"+
			"1.【发起游戏】：群内发送【发起谁是卧底】进入报名；\n"+
			"2.【报名参赛】：群友直接在群内回复【+1】或【报名】（3-8人）；\n"+
			"3.【私聊发词】：发起人发送【开始游戏】，肉丸会私聊每位玩家各自的词语；\n"+
			"4.【轮流发言】：群内按编号顺序依次发言描述自己的词；\n"+
			"5.【投票放逐】：发送【进入投票】，存活玩家输入【投X号】（如：投1号）；\n"+
			"6.【重置退出】：随时发送【结束谁是卧底】。")
		return true, nil

	case "发起谁是卧底", "开局谁是卧底", "谁是卧底":
		_, prompt, err := p.manager.StartRecruit(chatroomID, userID, userNickname)
		if err != nil {
			p.sendText(receiver, err.Error())
		} else {
			p.sendText(receiver, prompt)
		}
		return true, nil

	case "+1", "报名", "参加", "加入":
		prompt, err := p.manager.Join(chatroomID, userID, userNickname)
		if err != nil {
			p.sendText(receiver, err.Error())
		} else {
			p.sendText(receiver, prompt)
		}
		return true, nil

	case "开始游戏", "开始谁是卧底":
		_, players, prompt, err := p.manager.StartGame(chatroomID, userID)
		if err != nil {
			p.sendText(receiver, err.Error())
			return true, nil
		}
		p.sendText(receiver, prompt)

		// 私聊发送词语
		for _, pl := range players {
			p.sendPrivateWord(pl)
		}
		return true, nil

	case "进入投票", "开始投票":
		prompt, err := p.manager.EnterVote(chatroomID)
		if err != nil {
			p.sendText(receiver, err.Error())
		} else {
			p.sendText(receiver, prompt)
		}
		return true, nil

	case "公布票数", "结算投票", "统计票数":
		res, err := p.manager.TallyVotes(chatroomID)
		if err != nil {
			p.sendText(receiver, err.Error())
		} else {
			p.sendText(receiver, res.Detail)
		}
		return true, nil

	case "结束谁是卧底", "重置谁是卧底", "关闭谁是卧底":
		if p.manager.EndGame(chatroomID) {
			p.sendText(receiver, "🛑 当前谁是卧底游戏已结束并重置！想玩随时发【发起谁是卧底】重新开局！")
		} else {
			p.sendText(receiver, "当前没有进行中的谁是卧底游戏。")
		}
		return true, nil
	}

	// 检查投票命中
	if strings.HasPrefix(cleanText, "投") || strings.HasPrefix(cleanText, "投票") {
		game := p.manager.GetGame(chatroomID)
		if game != nil && game.State == StateVoting {
			reply, allVoted, err := p.manager.Vote(chatroomID, userID, cleanText)
			if err != nil {
				p.sendText(receiver, err.Error())
				return true, nil
			}
			p.sendText(receiver, reply)

			// 若全员已投完，自动结算
			if allVoted {
				res, err := p.manager.TallyVotes(chatroomID)
				if err == nil {
					p.sendText(receiver, "\n🔔 全员投票完毕，自动开票：\n"+res.Detail)
				}
			}
			return true, nil
		}
	}

	return false, nil
}

func (p *UndercoverPlugin) sendPrivateWord(player *Player) {
	privateReceiver := &contact.Contact{
		Username: player.Username,
	}
	content := fmt.Sprintf("🤫【谁是卧底 · 私聊专属发词】\n\n"+
		"你的出场编号是：%d号\n"+
		"你的专属词语是：【%s】\n\n"+
		"👉 请立即返回群聊，等待轮到你时按顺序进行一句话描述！\n"+
		"千万不要在描述中直接说出词汇本身哦！", player.Index, player.Word)

	msg := &message.Message{
		Type:     message.TypeText,
		Receiver: privateReceiver,
		Content:  content,
		Data:     &message.Message_Text{Text: &message.TextData{Content: content}},
	}
	if _, err := p.message.Send(msg); err != nil {
		slog.Error("[undercover] 私聊发词失败", "user", player.Username, "err", err)
	}
}

func (p *UndercoverPlugin) getUserInfo(e *plugin.Event, msg *message.Message) (string, string) {
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

func (p *UndercoverPlugin) resolveReceiver(e *plugin.Event, msg *message.Message) *contact.Contact {
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

func (p *UndercoverPlugin) sendText(receiver *contact.Contact, text string) {
	msg := &message.Message{
		Type:     message.TypeText,
		Receiver: receiver,
		Content:  text,
		Data:     &message.Message_Text{Text: &message.TextData{Content: text}},
	}
	if _, err := p.message.Send(msg); err != nil {
		slog.Error("[undercover] 发送消息失败", "err", err)
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
	p := &UndercoverPlugin{
		manager: NewGameManager(),
	}
	slog.Info("[undercover] 谁是卧底插件启动中...")
	plugin.Start(p)
}
