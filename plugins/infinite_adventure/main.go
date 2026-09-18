package main

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/sbgayhub/golem/sdk/chatroom"
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

// Config 插件配置
type Config struct {
	MaxSoloTurns         int    `toml:"max_solo_turns" comment:"单次私聊冒险最大推荐回合数"`
	GroupSquadMaxMembers int    `toml:"group_squad_max_members" comment:"群聊小队最大人数"`
	GroupTimeoutMinutes  int    `toml:"group_timeout_minutes" comment:"群聊无操作超时分钟数"`
	SaveFile             string `toml:"save_file" comment:"存档文件路径"`
}

// InfiniteAdventurePlugin 无限冒险文字RPG插件
type InfiniteAdventurePlugin struct {
	plugin.ConfigAbility[Config]
	message  message.Ability
	contact  contact.Ability
	chatroom chatroom.Ability
	caller   plugin.CallerAbility
	storage  *StorageManager
}

func (p *InfiniteAdventurePlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "infinite_adventure",
		Author:      "Golem Team",
		Version:     "1.0.0",
		Description: "无限冒险谭：开放式双男主文字RPG，支持任意主题脑洞与随机灵感盲盒，私聊1V1深度沉浸与群聊双人互动/兄弟小队共斗",
		Priority:    15, // 优先拦截处于沉浸模式下的玩家输入
	}
}

func (p *InfiniteAdventurePlugin) GetSubscriptions() []string {
	return []string{
		message.TypeText.Topic,
	}
}

func (p *InfiniteAdventurePlugin) getStorage() *StorageManager {
	if p.storage == nil {
		savePath := p.Config.SaveFile
		if savePath == "" {
			savePath = "data/infinite_adventure.json"
		}
		p.storage = NewStorageManager(savePath)
	}
	return p.storage
}

func (p *InfiniteAdventurePlugin) getBotName() string {
	if p.contact != nil {
		if self := p.contact.GetSelf(); self != nil {
			if nick := strings.TrimSpace(self.GetNickname()); nick != "" {
				return nick
			}
		}
	}
	return "肉丸"
}

func (p *InfiniteAdventurePlugin) isBot(nameOrID string) bool {
	if nameOrID == "" {
		return false
	}
	if nameOrID == "肉丸" || nameOrID == "肉丸叔叔" {
		return true
	}
	if p.contact != nil {
		if self := p.contact.GetSelf(); self != nil {
			if nameOrID == self.GetUsername() || nameOrID == self.GetNickname() {
				return true
			}
		}
	}
	return false
}

func (p *InfiniteAdventurePlugin) resolveMember(chatroomID, target string) (string, string) {
	cleanTarget := strings.TrimSpace(strings.Trim(target, " \t\r\n\u2005\u00a0"))
	if p.chatroom != nil && chatroomID != "" {
		for _, m := range p.chatroom.ListMembers(chatroomID) {
			if m == nil {
				continue
			}
			nick := strings.TrimSpace(m.GetNickname())
			display := strings.TrimSpace(m.GetDisplayName())
			username := strings.TrimSpace(m.GetUsername())

			if nick == cleanTarget || display == cleanTarget || username == cleanTarget {
				finalName := display
				if finalName == "" {
					finalName = nick
				}
				if finalName == "" {
					finalName = cleanTarget
				}
				slog.Info("[infinite_adventure] 匹配到群成员", "target", target, "username", username, "display", finalName)
				return username, finalName
			}
		}
	}
	slog.Info("[infinite_adventure] 未从群成员列表匹配到特定用户，使用原名", "target", cleanTarget)
	return cleanTarget, cleanTarget
}

// isParticipantOrAdmin 检查发送者是否为冒险参与者或管理员（Bot主人/群主/群管理员）
func (p *InfiniteAdventurePlugin) isParticipantOrAdmin(chatroomID, userID, nickname string, state *GroupAdventureState) bool {
	if state == nil {
		return false
	}

	// 1. 检查 Bot 主人 / 管理员
	if p.contact != nil {
		if owner := p.contact.GetOwner(); owner != nil {
			if (userID != "" && (userID == owner.Username || userID == owner.Alias)) ||
				(nickname != "" && (nickname == owner.Nickname || nickname == owner.Remark)) {
				return true
			}
		}
	}

	// 2. 检查微信群群主或管理员
	if p.chatroom != nil && chatroomID != "" {
		if info, err := p.chatroom.GetInfo(chatroomID); err == nil && info != nil {
			if userID != "" && info.Owner != "" && userID == info.Owner {
				return true
			}
		}
		if userID != "" {
			if member := p.chatroom.GetMember(chatroomID, userID); member != nil {
				// Flag: bit 0(1) 为群主, bit 1(2) 为群管理员
				if member.Flag&3 != 0 || member.Flag > 0 {
					return true
				}
			}
		}
	}
	if p.contact != nil && chatroomID != "" && userID != "" {
		if c := p.contact.Get(chatroomID); c != nil && c.GetChatroom() != nil {
			if userID == c.GetChatroom().GetOwner() {
				return true
			}
		}
	}

	// 3. 检查创建者
	if (userID != "" && userID == state.CreatorID) ||
		(nickname != "" && nickname == state.CreatorNick) {
		return true
	}

	// 4. 检查双人模式参与者
	if state.Mode == GroupModePair {
		if (userID != "" && (userID == state.PairUser1 || userID == state.PairUser2)) ||
			(nickname != "" && (nickname == state.PairNick1 || nickname == state.PairNick2)) {
			return true
		}
	}

	// 5. 检查小队/多人模式成员列表
	if state.Members != nil {
		for uid, m := range state.Members {
			if (userID != "" && (userID == uid || (m != nil && userID == m.Username))) ||
				(nickname != "" && m != nil && nickname == m.Nickname) {
				return true
			}
		}
	}

	// 6. 检查 MemberOrder / ActiveUser
	if userID != "" {
		if slices.Contains(state.MemberOrder, userID) || userID == state.ActiveUser {
			return true
		}
	}
	if nickname != "" && (nickname == state.ActiveNick) {
		return true
	}

	return false
}

func (p *InfiniteAdventurePlugin) OnLoad() error {
	_ = p.getStorage()
	return nil
}

func (p *InfiniteAdventurePlugin) OnUnload() error {
	return nil
}

func (p *InfiniteAdventurePlugin) OnEnable() error {
	return nil
}

func (p *InfiniteAdventurePlugin) OnDisable() error {
	return nil
}

func (p *InfiniteAdventurePlugin) OnEvent(e *plugin.Event) (res bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("[infinite_adventure] 捕获运行时异常", "panic", r)
			res = false
			err = fmt.Errorf("panic: %v", r)
		}
	}()

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
	userID, userNickname, targets := p.getUserAndTargets(e, msg)
	cleanText, hadMention := cleanMention(rawText)

	slog.Info("[infinite_adventure] 收到消息", "sender", sessionID, "user", userID, "nick", userNickname, "text", rawText, "cleanText", cleanText, "targets", targets)

	receiver := p.resolveReceiver(e, msg)
	if receiver == nil {
		return false, nil
	}

	// 统一帮助指引
	if cleanText == "无限冒险" || cleanText == "无限剧本" || cleanText == "冒险帮助" || cleanText == "无限冒险帮助" {
		p.sendHelp(receiver, isChatroom)
		return true, nil
	}

	if isChatroom {
		return p.handleGroupEvent(receiver, sessionID, userID, userNickname, cleanText, rawText, targets, hadMention)
	}

	return p.handlePrivateEvent(receiver, sessionID, userNickname, cleanText)
}

// 私聊事件处理
func (p *InfiniteAdventurePlugin) handlePrivateEvent(receiver *contact.Contact, sessionID, nickname, text string) (bool, error) {
	state := p.getStorage().GetSolo(sessionID)

	// 退出当前故事
	if text == "退出" || text == "结束" || text == "结束冒险" || text == "离开冒险" || text == "退出冒险" {
		if state != nil {
			p.getStorage().DeleteSolo(sessionID)
			p.sendText(receiver, "💾 当前故事已存盘离开。发送【开启冒险 [自定义脑洞]】或【随机冒险】随时开启全新故事！")
			return true, nil
		}
		return false, nil
	}

	// 查看状态与背包
	if text == "查看状态" || text == "状态" || text == "背包" || text == "查看背包" {
		if state != nil {
			p.sendText(receiver, FormatSoloStatus(state))
			return true, nil
		}
		p.sendText(receiver, "你当前尚未开启故事哦！发送【开启冒险】或【随机冒险】即可开启专属沉浸互动！")
		return true, nil
	}

	// 随机盲盒开局
	if text == "随机剧本" || text == "随机冒险" || text == "掷骰开局" || text == "灵感盲盒" {
		p.sendText(receiver, "正在展开场景，请稍候...")
		resp, err := StartSoloAdventure(p.caller, p.getStorage(), sessionID, nickname, "")
		if err != nil {
			p.sendText(receiver, fmt.Sprintf("⚠️ 场景展开失败：%v，请稍后重试", err))
			return true, nil
		}
		p.sendText(receiver, resp)
		return true, nil
	}

	// 自定义主题开局
	if strings.HasPrefix(text, "开启冒险") || strings.HasPrefix(text, "开始冒险") || strings.HasPrefix(text, "创建冒险") {
		prompt := strings.TrimSpace(strings.TrimPrefix(text, "开启冒险"))
		prompt = strings.TrimSpace(strings.TrimPrefix(prompt, "开始冒险"))
		prompt = strings.TrimSpace(strings.TrimPrefix(prompt, "创建冒险"))

		p.sendText(receiver, "正在进入情境，请稍候...")
		resp, err := StartSoloAdventure(p.caller, p.getStorage(), sessionID, nickname, prompt)
		if err != nil {
			p.sendText(receiver, fmt.Sprintf("⚠️ 场景展开失败：%v，请稍后重试", err))
			return true, nil
		}
		p.sendText(receiver, resp)
		return true, nil
	}

	// 若处于活跃冒险中，直接打字均视为自由行动
	if state != nil && state.IsActive {
		resp, err := ProcessSoloAction(p.caller, p.getStorage(), state, text)
		if err != nil {
			slog.Warn("[infinite_adventure] 私聊回合推进失败", "err", err)
			p.sendText(receiver, "⚠️ 局势变化莫测，脑电波信号略微波动，请再试一次或重新输入你的行动！")
			return true, nil
		}
		p.sendText(receiver, resp)
		return true, nil
	}

	return false, nil
}

// 群聊事件处理
func (p *InfiniteAdventurePlugin) handleGroupEvent(receiver *contact.Contact, chatroomID, userID, nickname, text, rawText string, targets []string, hadMention bool) (bool, error) {
	groupState := p.getStorage().GetGroup(chatroomID)

	// 过滤 targets 中的机器人名字，避免将机器人当作被邀请群友
	validTargets := make([]string, 0, len(targets))
	for _, t := range targets {
		if !p.isBot(t) {
			validTargets = append(validTargets, t)
		}
	}

	// 解散 / 退出
	if text == "解散队伍" || text == "解散小队" || text == "结束冒险" || text == "退出冒险" || text == "结束" || text == "完结" {
		if groupState != nil {
			if p.isParticipantOrAdmin(chatroomID, userID, nickname, groupState) {
				p.getStorage().DeleteGroup(chatroomID)
				p.sendText(receiver, fmt.Sprintf("🚩 @%s 已结束当前故事。发送【双人冒险】可随时开启新故事！", nickname))
				return true, nil
			} else {
				if text == "退出冒险" || text == "结束冒险" || text == "解散队伍" || text == "解散小队" {
					p.sendText(receiver, fmt.Sprintf("⚠️ @%s 只有参与本次冒险的成员或群管理员/主人才能结束冒险哦！", nickname))
					return true, nil
				}
			}
		} else if text == "退出冒险" || text == "结束冒险" || text == "解散队伍" || text == "解散小队" {
			p.sendText(receiver, "本群当前没有正在进行的冒险剧场。发送【双人冒险 @群友】或【创建小队 [主题]】开启！")
			return true, nil
		}
		return false, nil
	}

	// 查看队伍状态
	if text == "查看小队" || text == "小队状态" || text == "冒险状态" || text == "小队背包" {
		if groupState != nil {
			p.sendText(receiver, FormatGroupStatus(groupState))
			return true, nil
		}
		p.sendText(receiver, "本群当前没有正在进行的冒险剧场，发送【创建小队 [主题]】或【双人冒险 @群友】开启！")
		return true, nil
	}

	// 上车
	if text == "上车" || text == "加入" || text == "加入队伍" || text == "上车！" {
		maxM := p.Config.GroupSquadMaxMembers
		if maxM <= 0 {
			maxM = 5
		}
		msg, handled := JoinSquadLobby(p.getStorage(), chatroomID, userID, nickname, maxM)
		if handled {
			p.sendText(receiver, msg)
			return true, nil
		}
		return false, nil
	}

	// 发车
	if text == "发车" || text == "出发" || text == "开始冒险" || text == "发车！" {
		if groupState != nil && groupState.Status == "lobby" {
			p.sendText(receiver, "🚀 全员戒备，小队正在踏入危险未知的任务区域，请稍候...")
			resp, err := LaunchSquad(p.caller, p.getStorage(), chatroomID, userID)
			if err != nil {
				p.sendText(receiver, fmt.Sprintf("⚠️ 发车失败：%v", err))
				return true, nil
			}
			p.sendText(receiver, resp)
			return true, nil
		}
		return false, nil
	}

	// 双人冒险模式
	if strings.HasPrefix(text, "双人冒险") || strings.HasPrefix(text, "开启双人冒险") {
		prompt := strings.TrimSpace(strings.TrimPrefix(text, "双人冒险"))
		prompt = strings.TrimSpace(strings.TrimPrefix(prompt, "开启双人冒险"))

		slog.Info("[infinite_adventure] 触发双人冒险", "validTargets", validTargets, "prompt", prompt)

		// 检查是否有 @ 目标（优先使用过滤后的有效玩家目标）
		if len(validTargets) >= 2 {
			u1, n1 := p.resolveMember(chatroomID, validTargets[0])
			u2, n2 := p.resolveMember(chatroomID, validTargets[1])
			p.sendText(receiver, "正在进入情境，请稍候...")
			resp, err := StartPairAdventure(p.caller, p.getStorage(), chatroomID, u1, n1, u2, n2, prompt)
			if err != nil {
				p.sendText(receiver, fmt.Sprintf("⚠️ 场景展开失败：%v", err))
				return true, nil
			}
			p.sendText(receiver, resp)
			return true, nil
		} else if len(validTargets) == 1 {
			u1, n1 := userID, nickname
			u2, n2 := p.resolveMember(chatroomID, validTargets[0])
			p.sendText(receiver, "正在进入情境，请稍候...")
			resp, err := StartPairAdventure(p.caller, p.getStorage(), chatroomID, u1, n1, u2, n2, prompt)
			if err != nil {
				p.sendText(receiver, fmt.Sprintf("⚠️ 场景展开失败：%v", err))
				return true, nil
			}
			p.sendText(receiver, resp)
			return true, nil
		} else {
			p.sendText(receiver, "💡 请在指令中 @你想一起开启故事的群友，例如：\n【双人冒险 @横贯】或【双人冒险 @横贯 凌晨海边看日出】")
			return true, nil
		}
	}

	// 创建小队大厅
	if strings.HasPrefix(text, "创建小队") || strings.HasPrefix(text, "组建小队") {
		prompt := strings.TrimSpace(strings.TrimPrefix(text, "创建小队"))
		prompt = strings.TrimSpace(strings.TrimPrefix(prompt, "组建小队"))

		resp := CreateSquadLobby(p.getStorage(), chatroomID, userID, nickname, prompt)
		p.sendText(receiver, resp)
		return true, nil
	}

	// 群聊进行中回合推演（双人模式下当事人可直接打字，或@机器人/输入选项）
	if groupState != nil && groupState.Status == "in_progress" {
		isAction := false
		actionContent := text

		isPairMember := false
		if groupState.Mode == GroupModePair {
			if (userID != "" && (userID == groupState.PairUser1 || userID == groupState.PairUser2)) ||
				(nickname != "" && (nickname == groupState.PairNick1 || nickname == groupState.PairNick2)) {
				isPairMember = true
			}
		}

		if hadMention {
			isAction = true
		} else if strings.HasPrefix(text, "行动") || strings.HasPrefix(text, "【行动】") {
			isAction = true
			actionContent = strings.TrimSpace(strings.TrimPrefix(text, "行动"))
			actionContent = strings.TrimSpace(strings.TrimPrefix(actionContent, "【行动】"))
		} else if isSimpleChoice(text, len(groupState.Options)) {
			isAction = true
		} else if isPairMember {
			// 双人模式当事人直接输入文字均可作为抉择或互动补充
			isAction = true
		}

		if isAction && strings.TrimSpace(actionContent) != "" {
			resp, err := ProcessGroupAction(p.caller, p.getStorage(), groupState, userID, nickname, actionContent)
			if err != nil {
				slog.Warn("[infinite_adventure] 群聊回合推进失败", "err", err)
				p.sendText(receiver, "⚠️ 战局变幻莫测，未能完全响应，请再试一次！")
				return true, nil
			}
			p.sendText(receiver, resp)
			return true, nil
		}
	}

	return false, nil
}

func isSimpleChoice(text string, maxOpt int) bool {
	t := strings.TrimSpace(text)
	t = strings.TrimSuffix(t, ".")
	t = strings.TrimSuffix(t, "、")
	t = strings.TrimSpace(t)
	if len(t) == 1 {
		if t[0] >= '1' && t[0] <= '9' {
			if maxOpt <= 0 {
				return true
			}
			return int(t[0]-'0') <= maxOpt
		}
		if (t[0] >= 'a' && t[0] <= 'c') || (t[0] >= 'A' && t[0] <= 'C') {
			return true
		}
	}
	return false
}

func (p *InfiniteAdventurePlugin) sendHelp(receiver *contact.Contact, isChatroom bool) {
	if isChatroom {
		p.sendText(receiver, "🌌【无限冒险谭 · 群聊玩法手册】\n\n"+
			"1. 📖【双人故事】：\n"+
			"   发送【双人冒险 @群友 [主题/留空随机]】\n"+
			"   两人进入专属情境（日常生活、合租、露营、悬疑破案、公路旅行等任意主题），轮流推进故事！\n\n"+
			"2. 🛡️【兄弟男团·小队冒险】：\n"+
			"   ① 发送【创建小队 [主题]】开启集结；\n"+
			"   ② 群友发送【上车】加入队伍（2~5人）；\n"+
			"   ③ 队长发送【发车】立即启航，并肩推进！\n\n"+
			"3. 🎮 行动方式：\n"+
			"   直接回复选项序号（1/2/3），或【@肉丸 输入自由行动】推进故事！\n"+
			"   随时发送【小队状态】或【解散队伍】。\n\n"+
			"💡 私聊肉丸发送【开启冒险 [一句话设定]】可开启 1V1 专属电影级私密故事！")
		return
	}

	p.sendText(receiver, "🌌【无限冒险谭 · 私聊 1V1 沉浸剧场】\n\n"+
		"在这里，大模型将根据你的想象力，为你量身展开专属双男主沉浸世界！\n\n"+
		"📖【开启方式】：\n"+
		"1. 发送【开启冒险 [一句话脑洞]】\n"+
		"   （例：开启冒险 暴风雨夜，我和合租室友在客厅喝冰啤酒看雨）\n"+
		"   （例：开启冒险 边境森林巡逻，我和老战友突遇浓雾迷路）\n\n"+
		"2. 发送【随机冒险】/【掷骰开局】：\n"+
		"   由大模型从海岛避雨、合租夜谈、公路自驾等场景中随机展开极具质感的故事！\n\n"+
		"🎮【游玩指引】：\n"+
		"• 进入后，你可以直接输入数字序号（1/2/3），也可以直接打字输入任意自由动作；\n"+
		"• 大模型会像电影一样描写肢体动作、呼吸心跳与搭档的微表情拉扯；\n"+
		"• 随时发送【查看状态】查看背包与属性；\n"+
		"• 随时发送【退出】可存盘离开。")
}

func (p *InfiniteAdventurePlugin) getUserAndTargets(e *plugin.Event, msg *message.Message) (string, string, []string) {
	userID := e.GetSender()
	userNickname := ""

	if msg.Member != nil {
		userID = msg.Member.Username
		userNickname = msg.Member.Nickname
	} else if msg.Sender != nil {
		if userNickname == "" {
			userNickname = msg.Sender.Nickname
		}
	}

	// 提取 @ 所有人
	var targets []string
	rawContent := msg.GetContent()
	if strings.Contains(rawContent, "@") {
		parts := strings.Split(rawContent, "@")
		for i := 1; i < len(parts); i++ {
			p := parts[i]
			end := strings.IndexAny(p, " \t\r\n\u2005\u00a0:：,，")
			nick := p
			if end > 0 {
				nick = p[:end]
			}
			nick = strings.TrimSpace(nick)
			if nick != "" && !slices.Contains(targets, nick) {
				targets = append(targets, nick)
			}
		}
	}

	return userID, userNickname, targets
}

func (p *InfiniteAdventurePlugin) resolveReceiver(e *plugin.Event, msg *message.Message) *contact.Contact {
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

func (p *InfiniteAdventurePlugin) sendText(to *contact.Contact, text string) {
	if p.message == nil || to == nil || strings.TrimSpace(text) == "" {
		return
	}
	slog.Info("[infinite_adventure] 发送消息", "to", to.Username, "text", text)
	msg := &message.Message{
		Receiver: to,
		Type:     message.TypeText,
		Content:  text,
		Data:     &message.Message_Text{Text: &message.TextData{Content: text}},
	}
	if _, err := p.message.Send(msg); err != nil {
		slog.Error("[infinite_adventure] 发送消息失败", "err", err)
	}
}

func cleanMention(content string) (string, bool) {
	hadMention := strings.Contains(content, "@")
	if !hadMention {
		return strings.TrimSpace(content), false
	}

	var sb strings.Builder
	runes := []rune(content)
	n := len(runes)
	i := 0

	for i < n {
		if runes[i] == '@' {
			// 寻找 @ 后的结束分隔符
			j := i + 1
			for j < n {
				r := runes[j]
				if r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == '\u2005' || r == '\u00a0' || r == ':' || r == '：' || r == ',' || r == '，' {
					j++ // 跳过分隔符
					break
				}
				if r == '@' {
					break
				}
				j++
			}
			sb.WriteRune(' ')
			i = j
		} else {
			sb.WriteRune(runes[i])
			i++
		}
	}

	// 规范化空格并合并
	fields := strings.Fields(sb.String())
	return strings.Join(fields, " "), true
}

func main() {
	p := &InfiniteAdventurePlugin{
		storage: NewStorageManager("data/infinite_adventure.json"),
	}
	slog.Info("[infinite_adventure] 无限冒险文字RPG插件启动中...")
	plugin.Start(p)
}
