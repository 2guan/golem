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

func (p *TurtleSoupPlugin) getBotName() string {
	if p.contact != nil {
		if self := p.contact.GetSelf(); self != nil {
			if name := strings.TrimSpace(self.GetNickname()); name != "" {
				return name
			}
		}
	}
	return "AI 主持人"
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
	cmd := strings.ToLower(cleanText)

	speakerName := "探长"
	if msg.Member != nil && msg.Member.DisplayName != "" {
		speakerName = msg.Member.DisplayName
	} else if msg.Member != nil && msg.Member.Nickname != "" {
		speakerName = msg.Member.Nickname
	} else if msg.Sender != nil && msg.Sender.Nickname != "" {
		speakerName = msg.Sender.Nickname
	}

	switch cmd {
	case "海龟汤帮助", "海龟汤玩法":
		helpText := fmt.Sprintf("🍲【海龟汤玩法指南】\n\n"+
			"📚 当前题库储备：%d 道经典名案 + AI 现场熬制全新好汤！\n\n"+
			"1. 发送【来碗海龟汤】/【开局海龟汤】：默认由 AI 现场熬制一碗全新好汤（生成失败自动回退经典题库）；\n"+
			"2. 发送【来碗海龟汤 [主题]】/【AI海龟汤 [主题]】：支持定制主题（如【来碗海龟汤 温馨爱情】、【AI海龟汤 职场商战】）；\n"+
			"3. 发送【经典海龟汤】：直接从经典名案题库随机开局；\n"+
			"4. 玩家自由提问（如：死者是自杀吗？），主持人会根据汤底判定【是】/【不是】/【与此无关】/【关键线索！】；\n"+
			"5. 思考受阻时发【海龟汤提示】：获取渐进线索；\n"+
			"6. 最终还原真相发【看汤底】：揭晓全部故事细节！", len(p.manager.stories))
		p.sendText(receiver, helpText)
		return true, nil

	case "经典海龟汤", "题库海龟汤", "随机经典汤":
		_, welcome := p.manager.StartGame(sessionID)
		p.sendText(receiver, welcome)
		p.recordToAI(sessionID, cleanText, speakerName, welcome)
		return true, nil

	case "海龟汤提示", "要提示", "给个提示":
		tip := p.manager.GetClue(sessionID)
		p.sendText(receiver, tip)
		p.recordToAI(sessionID, cleanText, speakerName, tip)
		return true, nil

	case "看汤底", "放弃海龟汤", "揭晓汤底", "结束海龟汤":
		story, ok := p.manager.EndGame(sessionID)
		if !ok {
			msgReply := "当前没有进行中的海龟汤，发送【来碗海龟汤】开始新游戏！"
			p.sendText(receiver, msgReply)
			p.recordToAI(sessionID, cleanText, speakerName, msgReply)
			return true, nil
		}
		reveal := fmt.Sprintf("📖【海龟汤真相揭底 · %s】\n\n"+
			"📜 汤面：\n%s\n\n"+
			"🍲 完整汤底：\n%s\n\n"+
			"本轮海龟汤结束，想再玩发送【来碗海龟汤】随时再开！", story.Title, story.Surface, story.Bottom)
		p.sendText(receiver, reveal)
		p.recordToAI(sessionID, cleanText, speakerName, reveal)
		return true, nil
	}

	// 匹配开局海龟汤（支持不带主题或带任意主题定制，优先截留防止外泄到 AI 插件）
	if isAIStory, topic := parseAIStoryCmd(cleanText); isAIStory {
		p.startAIGame(sessionID, receiver, cleanText, speakerName, topic)
		return true, nil
	}

	// 检查当前是否有进行中的游戏
	session := p.manager.GetActiveSession(sessionID)
	if session != nil {
		// 如果是疑问句，或者显式 @了机器人，或者包含常见海龟汤提问关键字
		if wasMentioned || isLikelyQuestion(cleanText) {
			// 异步判定，避免阻塞总线与并发提问
			go func() {
				reply := p.judgeQuestion(session, cleanText, speakerName)
				if strings.Contains(reply, "【破案了！】") {
					p.manager.EndGame(sessionID)
				}
				p.sendText(receiver, reply)
				p.recordToAI(sessionID, cleanText, speakerName, reply)
			}()
			return true, nil
		}
	}

	return false, nil
}

func (p *TurtleSoupPlugin) startAIGame(sessionID string, receiver *contact.Contact, cleanText, speakerName, topic string) {
	sanitizedTopic := strings.TrimSpace(topic)
	if isSuspiciousTopic(sanitizedTopic) {
		slog.Warn("[turtle_soup] 🚨 检测到主题中包含敏感或注入词汇，已强制重置为安全推理主题", "topic", sanitizedTopic, "speaker", speakerName)
		sanitizedTopic = ""
	}

	waitMsg := "🍲 收到！正在现场现熬一碗新鲜海龟汤，稍等片刻..."
	if sanitizedTopic != "" {
		waitMsg = fmt.Sprintf("🍲 正在根据主题【%s】现场熬制一碗专属海龟汤，稍等片刻...", sanitizedTopic)
	} else if topic != "" {
		waitMsg = "🍲 收到！已过滤非故事性指令，正在为你熬制一碗悬疑推理海龟汤，稍等片刻..."
	}
	p.sendText(receiver, waitMsg)
	go func() {
		story, err := p.generateAIStory(45, sanitizedTopic)
		if err != nil {
			slog.Warn("[turtle_soup] AI出题超时或失败，降级使用本地经典题库", "topic", sanitizedTopic, "err", err)
			_, welcome := p.manager.StartGame(sessionID)
			out := "⚠️ 现场现熬海龟汤失败，先为你端上一碗经典名案：\n\n" + welcome
			p.sendText(receiver, out)
			p.recordToAI(sessionID, cleanText, speakerName, out)
			return
		}
		_, welcome := p.manager.StartCustomStory(sessionID, story, sanitizedTopic)
		p.sendText(receiver, welcome)
		p.recordToAI(sessionID, cleanText, speakerName, welcome)
	}()
}

func isSuspiciousTopic(topic string) bool {
	t := strings.TrimSpace(topic)
	if t == "" {
		return false
	}
	lower := strings.ToLower(t)
	forbidden := []string{
		"聊天记录", "聊天内容", "聊天的昵称", "用户昵称", "好友昵称", "群友昵称",
		"聊天次数", "互动次数", "私聊", "真实内容", "真实聊天", "谁和你聊", "和谁聊",
		"json", "system prompt", "提示词", "ignore", "override", "指令", "设定",
	}
	for _, kw := range forbidden {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func parseAIStoryCmd(text string) (bool, string) {
	t := strings.TrimSpace(text)
	lower := strings.ToLower(t)

	// 排除保留的控制命令，避免被误判为主题
	reserved := []string{"海龟汤帮助", "海龟汤玩法", "海龟汤提示", "海龟汤线索", "经典海龟汤", "题库海龟汤", "看汤底", "揭晓汤底", "放弃海龟汤", "结束海龟汤"}
	for _, r := range reserved {
		if lower == r {
			return false, ""
		}
	}

	prefixes := []string{
		"来碗海龟汤", "开局海龟汤", "换一碗海龟汤", "来一碗海龟汤", "再来一碗海龟汤",
		"私房海龟汤", "来碗私房海龟汤", "来碗私房汤", "私房汤", "独家海龟汤", "来碗独家汤", "来碗独家海龟汤",
		"ai海龟汤", "来碗ai海龟汤", "开局ai海龟汤", "ai汤", "来碗ai汤",
		"原创海龟汤", "来碗原创海龟汤", "来碗原创汤", "原创汤",
		"新海龟汤", "来碗新海龟汤", "来碗新汤", "新汤",
		"智能海龟汤", "生成海龟汤", "定制海龟汤",
		"海龟汤",
	}

	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			rest := strings.TrimSpace(t[len(p):])
			// 剥离可能的分隔符：冒号、逗号、空格、破折号等
			rest = strings.TrimLeft(rest, " :：,，-—")
			return true, strings.TrimSpace(rest)
		}
	}
	return false, ""
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

func (p *TurtleSoupPlugin) recordToAI(session, userText, speakerName, assistantText string) {
	if p.caller == nil || strings.TrimSpace(session) == "" {
		return
	}
	args := map[string]string{
		"session":        session,
		"user_text":      userText,
		"speaker_name":   speakerName,
		"assistant_text": assistantText,
	}
	_, _, err := p.caller.CallPlugin("ai.record_interaction", args)
	if err != nil {
		slog.Warn("[turtle_soup] 同步海龟汤记忆到 ai 插件失败", "err", err, "session", session)
	} else {
		slog.Info("[turtle_soup] 成功同步海龟汤记忆到 ai 插件", "session", session)
	}
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
