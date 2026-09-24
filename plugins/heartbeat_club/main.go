package main

import (
	"log/slog"
	"strings"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

type HeartbeatClubPlugin struct {
	plugin.ConfigAbility[struct{}]
	message  message.Ability
	contact  contact.Ability
	caller   plugin.CallerAbility
	sessions *SessionManager
}

func (p *HeartbeatClubPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "heartbeat_club",
		Author:      "Golem Team",
		Version:     "1.0.0",
		Description: "心跳沉浸馆：集铁馆更衣室、微醺小酒馆、男生宿舍熄灯后三大场景于一体的双端文字互动RPG，支持私聊1V1剧情与群聊荷尔蒙大乱斗",
		Priority:    10, // 优先拦截处于沉浸模式下的私聊指令
	}
}

func (p *HeartbeatClubPlugin) GetSubscriptions() []string {
	return []string{
		message.TypeText.Topic,
	}
}

func (p *HeartbeatClubPlugin) OnEvent(e *plugin.Event) (bool, error) {
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

	isChatroom := strings.HasSuffix(sessionID, "@chatroom")
	userID, userNickname, targetNickname := p.getUserAndTargetInfo(e, msg)
	cleanText, _ := cleanMention(rawText)

	receiver := p.resolveReceiver(e, msg)
	if receiver == nil {
		return false, nil
	}

	// 1. 群聊场景路由
	if isChatroom {
		return p.handleChatroomEvent(receiver, cleanText, userNickname, targetNickname)
	}

	// 2. 私聊 1V1 沉浸场景路由
	return p.handlePrivateEvent(receiver, cleanText, userID, userNickname)
}

// handleChatroomEvent 群聊派对玩法
func (p *HeartbeatClubPlugin) handleChatroomEvent(receiver *contact.Contact, text, senderName, targetName string) (bool, error) {
	switch {
	case text == "心跳俱乐部" || text == "心跳沉浸馆" || text == "心动俱乐部" || text == "心跳帮助":
		p.sendText(receiver, "🔥【心跳沉浸馆 · 群聊荷尔蒙大乱斗】\n\n"+
			"群内专属互动指令：\n"+
			"1. 🏋️‍♂️【力量对抗】：发送【@某人 掰手腕】或【@某人 比拼身材】；\n"+
			"2. 🍸【微醺大冒险】：发送【微醺大冒险】或【酒吧摇骰】；\n"+
			"3. 🎽【突袭查寝】：发送【宿管查寝】查看本寝室荷尔蒙纪要；\n"+
			"4. 💘【契合测算】：发送【契合度 @某人】测算双人性张力契合指数！\n\n"+
			"💡 私聊发送【心跳沉浸馆】，可开启 1V1 专属电影级沉浸互动剧场（铁馆/酒吧/宿舍）！")
		return true, nil

	case strings.Contains(text, "掰手腕") || strings.Contains(text, "比拼身材") || strings.Contains(text, "身材比拼"):
		res := HandleArmWrestle(senderName, targetName)
		p.sendText(receiver, res)
		return true, nil

	case text == "微醺大冒险" || text == "酒吧摇骰" || text == "酒吧大冒险" || text == "摇骰子":
		res := HandleBarDare(senderName)
		p.sendText(receiver, res)
		return true, nil

	case text == "宿管查寝" || text == "查寝" || text == "突袭查寝":
		res := HandleDormInspection(senderName)
		p.sendText(receiver, res)
		return true, nil

	case strings.Contains(text, "契合度") || strings.Contains(text, "配对"):
		res := HandleCompatibility(senderName, targetName)
		p.sendText(receiver, res)
		return true, nil

	case text == "离开场景" || text == "退出场景" || text == "退出心跳" || text == "退出沉浸馆" || text == "退出俱乐部":
		p.sendText(receiver, "💡 心跳沉浸馆在群聊中为单次即时互动（即玩即结，无需退出）；\n若你在私聊开启了 1V1 专属剧场，请在私聊中发送【离开场景】或【退出】即可离开！")
		return true, nil
	}

	return false, nil
}

// handlePrivateEvent 私聊 1V1 沉浸剧情
func (p *HeartbeatClubPlugin) handlePrivateEvent(receiver *contact.Contact, text, userID, userNickname string) (bool, error) {
	// 场景选择指令
	switch text {
	case "心跳俱乐部", "心跳沉浸馆", "心动俱乐部", "沉浸馆", "心跳帮助":
		p.sendText(receiver, "🔥【心跳沉浸馆 · 私聊专属 1V1 剧场】\n\n"+
			"请选择你想进入的心动场景：\n\n"+
			"1. 🏋️‍♂️ 发送【去健身房】：进入热气与汗水弥漫的铁馆更衣室，让微壮私教贴身保驾；\n"+
			"2. 🍸 发送【去小酒馆】：进入暖光爵士清吧，看主理人微挽衬衫为你摇晃古典杯；\n"+
			"3. 🎽 发送【回宿舍】：回到熄灯断电后的体校男寝，挤进糙汉室友温暖的被窝；\n\n"+
			"💡 进入后，你可以自由打字输入你想做的事情，AI 会根据你的动作推进剧情！随时输入【离开场景】可退出。")
		return true, nil

	case "去健身房", "健身房", "铁馆", "更衣室":
		s := p.sessions.StartSolo(userID, userNickname, SceneGym)
		sceneInfo := AllScenes[SceneGym]
		card := FormatSceneCard(sceneInfo, s.HeartRate, s.Tension, s.ExtraStatus)
		p.sendText(receiver, card)
		return true, nil

	case "去小酒馆", "小酒馆", "酒吧", "清吧":
		s := p.sessions.StartSolo(userID, userNickname, SceneBar)
		sceneInfo := AllScenes[SceneBar]
		card := FormatSceneCard(sceneInfo, s.HeartRate, s.Tension, s.ExtraStatus)
		p.sendText(receiver, card)
		return true, nil

	case "回宿舍", "宿舍", "男寝", "男生宿舍":
		s := p.sessions.StartSolo(userID, userNickname, SceneDorm)
		sceneInfo := AllScenes[SceneDorm]
		card := FormatSceneCard(sceneInfo, s.HeartRate, s.Tension, s.ExtraStatus)
		p.sendText(receiver, card)
		return true, nil

	case "离开场景", "离开", "退出", "结束互动", "退出场景", "退出心跳", "退出沉浸馆", "退出俱乐部", "结束场景", "退出游戏", "结束":
		if p.sessions.EndSolo(userID) {
			p.sendText(receiver, "🚪【已离开场景】\n对方朝你挥了挥手：“回神了？下回想找刺激或者想喝两杯，随时再来找我。”\n（发送【心跳沉浸馆】可随时再次进入）")
			return true, nil
		}
		if text == "离开场景" || text == "退出场景" || text == "退出心跳" || text == "退出沉浸馆" || text == "退出俱乐部" || text == "结束场景" {
			p.sendText(receiver, "你当前不在任何心跳场景中哦。（私聊发送【心跳沉浸馆】可开启 1V1 专属剧场）")
			return true, nil
		}
		return false, nil
	}

	// 检查是否在活跃会话中
	session := p.sessions.GetSolo(userID)
	if session == nil {
		// 未在场景中，交由其他插件（如 AI 闲聊）处理
		return false, nil
	}

	// 正在场景中，进行剧情推进
	reply, err := ProcessSoloAction(p.caller, session, text)
	if err != nil {
		slog.Error("[heartbeat_club] 剧情处理失败", "err", err)
	}
	p.sendText(receiver, reply)
	return true, nil
}

func (p *HeartbeatClubPlugin) getUserAndTargetInfo(e *plugin.Event, msg *message.Message) (string, string, string) {
	userID := ""
	userNickname := "群友"
	targetNickname := ""

	if msg.Member != nil {
		userID = msg.Member.Username
		if msg.Member.DisplayName != "" {
			userNickname = msg.Member.DisplayName
		} else if msg.Member.Nickname != "" {
			userNickname = msg.Member.Nickname
		}
	} else if msg.Sender != nil {
		userID = msg.Sender.Username
		if msg.Sender.Remark != "" {
			userNickname = msg.Sender.Remark
		} else if msg.Sender.Nickname != "" {
			userNickname = msg.Sender.Nickname
		}
	}

	// 提取 @ 的目标群友
	rawContent := msg.GetContent()
	if strings.Contains(rawContent, "@") {
		parts := strings.Split(rawContent, "@")
		if len(parts) > 1 {
			targetPart := parts[1]
			endIdx := strings.IndexAny(targetPart, " \t\r\n\u2005\u00a0:：,，")
			if endIdx > 0 {
				targetNickname = strings.TrimSpace(targetPart[:endIdx])
			} else {
				targetNickname = strings.TrimSpace(targetPart)
			}
		}
	}

	return userID, userNickname, targetNickname
}

func (p *HeartbeatClubPlugin) resolveReceiver(e *plugin.Event, msg *message.Message) *contact.Contact {
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

func (p *HeartbeatClubPlugin) sendText(to *contact.Contact, text string) {
	if p.message == nil || to == nil || strings.TrimSpace(text) == "" {
		return
	}
	msg := &message.Message{
		Receiver: to,
		Type:     message.TypeText,
		Data:     &message.Message_Text{Text: &message.TextData{Content: text}},
	}
	if _, err := p.message.Send(msg); err != nil {
		slog.Error("[heartbeat_club] 发送消息失败", "err", err)
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
	p := &HeartbeatClubPlugin{
		sessions: NewSessionManager(),
	}
	slog.Info("[heartbeat_club] 心跳沉浸馆插件启动中...")
	plugin.Start(p)
}
