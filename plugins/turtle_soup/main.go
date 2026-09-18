package main

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

type TurtleSoupPlugin struct {
	message message.Ability
	contact contact.Ability
	caller  plugin.CallerAbility
	manager *GameManager
}

func (p *TurtleSoupPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "turtle_soup",
		Author:      "Golem Team",
		Version:     "1.0.0",
		Description: "海龟汤推理派对小游戏，支持出题、提问判定、线索提示与揭开汤底",
		Priority:    -50,
	}
}

func (p *TurtleSoupPlugin) GetSubscriptions() []string {
	return []string{
		message.TypeText.Topic,
	}
}

func (p *TurtleSoupPlugin) OnEvent(e *plugin.Event) (bool, error) {
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

	sessionID := e.GetSender()
	if sessionID == "" && msg.Sender != nil {
		sessionID = msg.Sender.GetUsername()
	}

	receiver := p.resolveReceiver(e, msg)
	if receiver == nil {
		return false, nil
	}

	cleanText, wasMentioned := cleanMention(rawText)

	switch cleanText {
	case "海龟汤帮助":
		p.sendText(receiver, fmt.Sprintf("🍲【海龟汤玩法指南】\n\n"+
			"📚 当前题库储备：%d 道经典名案 + 肉丸独家私房好汤（随时现熬）！\n\n"+
			"1. 发送【来碗海龟汤】/【开局海龟汤】：随机抽取一碗经典故事名案；\n"+
			"2. 发送【肉丸私房汤】/【来碗新汤】：肉丸当场现熬一碗未公开过的独家密案；\n"+
			"3. 玩家自由提问（如：死者是自杀吗？），肉丸会根据汤底判定【是】/【不是】/【与此无关】/【关键线索！】；\n"+
			"4. 思考受阻时发【海龟汤提示】：获取渐进线索；\n"+
			"5. 最终还原真相发【看汤底】：揭晓全部故事细节！", len(p.manager.stories)))
		return true, nil

	case "来碗海龟汤", "开局海龟汤", "海龟汤", "换一碗海龟汤":
		_, welcome := p.manager.StartGame(sessionID)
		p.sendText(receiver, welcome)
		return true, nil

	case "肉丸私房汤", "来碗私房汤", "来碗独家汤", "来碗新汤", "新汤", "来碗原创海龟汤", "原创海龟汤", "新海龟汤":
		p.sendText(receiver, "⏳ 肉丸正在翻箱倒柜琢磨一碗绝密私房汤，稍等片刻...")
		go func() {
			story, err := p.generateAIStory(120)
			if err != nil {
				slog.Warn("[turtle_soup] 私房出题超时或失败，降级题库随机", "err", err)
				_, welcome := p.manager.StartGame(sessionID)
				p.sendText(receiver, "⚠️ 刚才那碗太烧脑没熬好，肉丸先给你端一碗经典名案垫垫：\n\n"+welcome)
				return
			}
			_, welcome := p.manager.StartCustomStory(sessionID, story)
			p.sendText(receiver, welcome)
		}()
		return true, nil

	case "海龟汤提示", "要提示", "给个提示":
		tip := p.manager.GetClue(sessionID)
		p.sendText(receiver, tip)
		return true, nil

	case "看汤底", "放弃海龟汤", "揭晓汤底", "结束海龟汤":
		story, ok := p.manager.EndGame(sessionID)
		if !ok {
			p.sendText(receiver, "当前没有进行中的海龟汤，发送【来碗海龟汤】开始新游戏！")
			return true, nil
		}
		reveal := fmt.Sprintf("📖【海龟汤真相揭底 · %s】\n\n"+
			"📜 汤面：\n%s\n\n"+
			"🍲 完整汤底：\n%s\n\n"+
			"本轮海龟汤结束，想再玩发送【来碗海龟汤】随时再开！", story.Title, story.Surface, story.Bottom)
		p.sendText(receiver, reveal)
		return true, nil
	}

	// 检查当前是否有进行中的游戏
	session := p.manager.GetActiveSession(sessionID)
	if session != nil {
		// 如果是疑问句，或者显式 @了机器人，或者包含常见海龟汤提问关键字
		if wasMentioned || isLikelyQuestion(cleanText) {
			speakerName := "探长"
			if msg.Member != nil && msg.Member.DisplayName != "" {
				speakerName = msg.Member.DisplayName
			} else if msg.Member != nil && msg.Member.Nickname != "" {
				speakerName = msg.Member.Nickname
			} else if msg.Sender != nil && msg.Sender.Nickname != "" {
				speakerName = msg.Sender.Nickname
			}

			// 异步判定，避免阻塞总线与并发提问
			go func() {
				reply := p.judgeQuestion(session, cleanText, speakerName)
				if strings.Contains(reply, "【破案了！】") {
					p.manager.EndGame(sessionID)
				}
				p.sendText(receiver, reply)
			}()
			return true, nil
		}
	}

	return false, nil
}

func isLikelyQuestion(text string) bool {
	t := strings.TrimSpace(text)
	if strings.HasSuffix(t, "?") || strings.HasSuffix(t, "？") {
		return true
	}
	keywords := []string{"是不是", "有没有", "为什么", "是吗", "有人", "有关吗", "相关吗", "死者", "他是", "她是", "他们", "是不是因为", "自杀", "他杀"}
	for _, kw := range keywords {
		if strings.Contains(t, kw) {
			return true
		}
	}
	return false
}

func (p *TurtleSoupPlugin) resolveReceiver(e *plugin.Event, msg *message.Message) *contact.Contact {
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

func (p *TurtleSoupPlugin) sendText(receiver *contact.Contact, text string) {
	msg := &message.Message{
		Type:     message.TypeText,
		Receiver: receiver,
		Content:  text,
		Data:     &message.Message_Text{Text: &message.TextData{Content: text}},
	}
	if _, err := p.message.Send(msg); err != nil {
		slog.Error("[turtle_soup] 发送消息失败", "err", err)
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
	p := &TurtleSoupPlugin{
		manager: NewGameManager(),
	}
	slog.Info("[turtle_soup] 海龟汤插件启动中...")
	plugin.Start(p)
}
