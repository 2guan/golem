package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/sbgayhub/golem/sdk/plugin"
)

var defaultSquadRoles = []string{
	"前锋突击（近战破障）",
	"战地医疗（伤势救护）",
	"敏捷尖兵（潜行侦查）",
	"战术学者（机关解密）",
	"重装支援（火力掩护）",
}

// StartPairAdventure 开启群聊双人搭档剧情
func StartPairAdventure(caller plugin.CallerAbility, storage *StorageManager, chatroomID, u1, nick1, u2, nick2, themePrompt string) (string, error) {
	initData, err := GenerateOpening(caller, themePrompt, fmt.Sprintf("%s与%s", nick1, nick2))
	if err != nil {
		return "", err
	}

	state := &GroupAdventureState{
		ChatroomID:   chatroomID,
		Mode:         GroupModePair,
		CreatorID:    u1,
		CreatorNick:  nick1,
		Title:        initData.Title,
		ThemePrompt:  themePrompt,
		Crisis:       initData.Crisis,
		PairUser1:    u1,
		PairNick1:          nick1,
		PairUser2:          u2,
		PairNick2:          nick2,
		ActiveUser:         u2,
		ActiveNick:         nick2,
		PendingSupplements: nil,
		TeamBond:           30,
		DynamicAttrs:       initData.DynamicAttrs,
		Inventory:          initData.Inventory,
		CurrentScene:       initData.OpeningScene,
		TurnCount:          1,
		Options:            initData.Options,
		History: []HistoryMsg{
			{Role: "assistant", Content: fmt.Sprintf("%s\n%s", initData.OpeningScene, initData.OpeningWords)},
		},
		Status:    "in_progress",
		UpdatedAt: time.Now(),
	}

	storage.SetGroup(state)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✨ %s\n", state.Title))
	sb.WriteString(fmt.Sprintf("👥 当事人：@%s × @%s\n", nick1, nick2))
	sb.WriteString(fmt.Sprintf("🎯 情境：%s\n\n", state.Crisis))

	sb.WriteString(fmt.Sprintf("🎬【场景】：\n%s\n\n", initData.OpeningScene))

	sb.WriteString(fmt.Sprintf("🔥 默契/心动度：%d%%", state.TeamBond))

	return sb.String(), nil
}

// CreateSquadLobby 创建【兄弟小队·多人求生】上车大厅
func CreateSquadLobby(storage *StorageManager, chatroomID, creatorID, creatorNick, themePrompt string) string {
	state := &GroupAdventureState{
		ChatroomID:   chatroomID,
		Mode:         GroupModeSquad,
		CreatorID:    creatorID,
		CreatorNick:  creatorNick,
		ThemePrompt:  themePrompt,
		Members:      make(map[string]*SquadMember),
		MemberOrder:  []string{creatorID},
		Status:       "lobby",
		LobbyExpires: time.Now().Add(3 * time.Minute),
		UpdatedAt:    time.Now(),
	}

	state.Members[creatorID] = &SquadMember{
		Username: creatorID,
		Nickname: creatorNick,
		Role:     defaultSquadRoles[0],
		Health:   100,
	}

	storage.SetGroup(state)

	promptDesc := themePrompt
	if promptDesc == "" {
		promptDesc = "大模型随机盲盒题材"
	}

	return fmt.Sprintf("🚙【冒险小队集结令】\n"+
		"发起人：@%s\n"+
		"设定主题：%s\n\n"+
		"当前队伍（1/5 人）：\n"+
		"1. %s [队长 · %s]\n\n"+
		"👉 其他想加入的好兄弟请发送【上车】或【加入】！\n"+
		"👉 人齐后队长发送【发车】立即开局！（3分钟内有效）",
		creatorNick, promptDesc, creatorNick, defaultSquadRoles[0])
}

// JoinSquadLobby 队员上车
func JoinSquadLobby(storage *StorageManager, chatroomID, userID, nickname string, maxMembers int) (string, bool) {
	state := storage.GetGroup(chatroomID)
	if state == nil || state.Status != "lobby" {
		return "", false
	}

	if time.Now().After(state.LobbyExpires) {
		storage.DeleteGroup(chatroomID)
		return "⚠️ 上车等待已超时，小队大厅已自动解散。可重新发送【创建小队】发起！", true
	}

	if _, exists := state.Members[userID]; exists {
		return fmt.Sprintf("@%s 你已经在队伍里啦！等待队长发车中~", nickname), true
	}

	if len(state.Members) >= maxMembers {
		return fmt.Sprintf("⚠️ 队伍已满员（%d/%d），无法继续上车啦！", len(state.Members), maxMembers), true
	}

	roleIdx := len(state.Members)
	role := "后援战备"
	if roleIdx < len(defaultSquadRoles) {
		role = defaultSquadRoles[roleIdx]
	}

	state.Members[userID] = &SquadMember{
		Username: userID,
		Nickname: nickname,
		Role:     role,
		Health:   100,
	}
	state.MemberOrder = append(state.MemberOrder, userID)
	storage.SetGroup(state)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🎉 @%s 成功上车！战术身份：【%s】\n", nickname, role))
	sb.WriteString(fmt.Sprintf("当前队伍（%d/%d 人）：\n", len(state.Members), maxMembers))
	for i, uid := range state.MemberOrder {
		m := state.Members[uid]
		sb.WriteString(fmt.Sprintf("%d. %s - %s\n", i+1, m.Nickname, m.Role))
	}
	sb.WriteString("\n队长发送【发车】即可开局！")
	return sb.String(), true
}

// LaunchSquad 发车开启小队冒险
func LaunchSquad(caller plugin.CallerAbility, storage *StorageManager, chatroomID, senderID string) (string, error) {
	state := storage.GetGroup(chatroomID)
	if state == nil || state.Status != "lobby" {
		return "当前没有正在集结的小队，可发送【创建小队 [主题]】发起！", nil
	}

	if senderID != state.CreatorID {
		return fmt.Sprintf("只有队长 @%s 可以发车哦！", state.CreatorNick), nil
	}

	if len(state.Members) < 2 {
		return "小队至少需要 2 位队员才能出发，快喊群里好兄弟发送【上车】加入吧！", nil
	}

	names := make([]string, 0, len(state.Members))
	for _, uid := range state.MemberOrder {
		names = append(names, state.Members[uid].Nickname)
	}

	theme := state.ThemePrompt
	if theme == "" {
		theme = "由 " + strings.Join(names, "、") + " 组成的兄弟探险队，深入极端危险险境"
	}

	initData, err := GenerateOpening(caller, theme, strings.Join(names, "与"))
	if err != nil {
		return "", err
	}

	state.Title = initData.Title
	state.Crisis = initData.Crisis
	state.NPCLeader = &initData.Partner
	state.TeamBond = 30
	state.DynamicAttrs = initData.DynamicAttrs
	state.Inventory = initData.Inventory
	state.CurrentScene = initData.OpeningScene
	state.TurnCount = 1
	state.Options = initData.Options
	state.History = []HistoryMsg{
		{Role: "assistant", Content: fmt.Sprintf("%s\n%s", initData.OpeningScene, initData.OpeningWords)},
	}
	state.Status = "in_progress"
	state.UpdatedAt = time.Now()

	storage.SetGroup(state)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🚀【小队发车】%s\n", state.Title))
	sb.WriteString(fmt.Sprintf("⚠️【全队危机】：%s\n\n", state.Crisis))

	sb.WriteString("🛡️【小队阵容】：\n")
	for i, uid := range state.MemberOrder {
		m := state.Members[uid]
		sb.WriteString(fmt.Sprintf("%d. @%s · %s\n", i+1, m.Nickname, m.Role))
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("🎬【遭遇战况】：\n%s\n\n", initData.OpeningScene))
	sb.WriteString(fmt.Sprintf("💬 战地呼叫：\n%s\n\n", initData.OpeningWords))

	sb.WriteString(fmt.Sprintf("🔥 团队默契度：%d%% | 🎒 共享物资：%s\n\n", state.TeamBond, strings.Join(state.Inventory, "、")))

	sb.WriteString("💡【全队战术抉择】（任何队员均可直接回复序号或@机器人输入自由行动）：\n")
	for i, opt := range state.Options {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, opt))
	}

	return sb.String(), nil
}

// ProcessGroupAction 处理群聊回合推进
func ProcessGroupAction(caller plugin.CallerAbility, storage *StorageManager, state *GroupAdventureState, senderID, senderNick, rawAction string) (string, error) {
	// 双人模式权限与轮次检查
	if state.Mode == GroupModePair {
		isUser1 := (senderID != "" && senderID == state.PairUser1) ||
			(senderNick != "" && (senderNick == state.PairNick1 || senderNick == state.PairUser1))
		isUser2 := (senderID != "" && senderID == state.PairUser2) ||
			(senderNick != "" && (senderNick == state.PairNick2 || senderNick == state.PairUser2))

		// 如果当时未完全绑定真实ID，当前发言者匹配昵称时自动绑定
		if isUser1 {
			if state.PairUser1 == "" && senderID != "" {
				state.PairUser1 = senderID
			}
			if state.PairNick1 == "" && senderNick != "" {
				state.PairNick1 = senderNick
			}
			if senderNick == "" && state.PairNick1 != "" {
				senderNick = state.PairNick1
			}
		}
		if isUser2 {
			if state.PairUser2 == "" && senderID != "" {
				state.PairUser2 = senderID
			}
			if state.PairNick2 == "" && senderNick != "" {
				state.PairNick2 = senderNick
			}
			if senderNick == "" && state.PairNick2 != "" {
				senderNick = state.PairNick2
			}
		}

		if !isUser1 && !isUser2 {
			return fmt.Sprintf("当前故事正由 @%s 与 @%s 推进中。", state.PairNick1, state.PairNick2), nil
		}
	} else if state.Mode == GroupModeSquad {
		if _, ok := state.Members[senderID]; !ok {
			return "⚠️ 你不是当前小队成员，可等待本局结束后发送【创建小队】开启全新一局！", nil
		}
	}

	actionText := resolveActionText(rawAction, state.Options)

	turnOut, err := AdvanceGroupTurn(caller, state, senderNick, actionText, state.PendingSupplements)
	if err != nil {
		return "", err
	}

	state.TurnCount++
	state.TeamBond += turnOut.BondDelta
	if state.TeamBond > 100 {
		state.TeamBond = 100
	}
	if state.TeamBond < 0 {
		state.TeamBond = 0
	}

	// 双人模式下，心动度降到 10% 或以下直接触发散场终局
	if state.Mode == GroupModePair && state.TeamBond <= 10 {
		turnOut.IsEnding = true
		if turnOut.EndingTitle == "" {
			turnOut.EndingTitle = "《渐行渐远》"
		}
	}

	// 更新成员伤害（若有）
	if turnOut.HealthDelta < 0 && state.Mode == GroupModeSquad {
		if m, ok := state.Members[senderID]; ok {
			m.Health += turnOut.HealthDelta
			if m.Health < 0 {
				m.Health = 0
			}
		}
	}

	// 物品增减
	for _, it := range turnOut.AddItems {
		it = strings.TrimSpace(it)
		if it != "" && !slices.Contains(state.Inventory, it) {
			state.Inventory = append(state.Inventory, it)
		}
	}
	for _, it := range turnOut.RemoveItems {
		it = strings.TrimSpace(it)
		state.Inventory = slices.DeleteFunc(state.Inventory, func(s string) bool { return s == it })
	}

	if len(turnOut.Options) > 0 {
		state.Options = turnOut.Options
	}

	histContent := fmt.Sprintf("【%s】行动：%s", senderNick, actionText)
	if len(state.PendingSupplements) > 0 {
		histContent += fmt.Sprintf("（同伴补充：%s）", strings.Join(state.PendingSupplements, "，"))
	}

	state.History = append(state.History,
		HistoryMsg{Role: "user", Content: histContent},
		HistoryMsg{Role: "assistant", Content: strings.TrimSpace(fmt.Sprintf("%s\n%s", turnOut.Story, turnOut.PartnerWords))},
	)
	if len(state.History) > 20 {
		state.History = state.History[len(state.History)-20:]
	}

	// 计算下一位行动当事人并清空已消耗的补充
	nextNick := state.PairNick2
	nextUser := state.PairUser2
	if senderNick == state.PairNick2 || senderID == state.PairUser2 {
		nextNick = state.PairNick1
		nextUser = state.PairUser1
	}
	state.ActiveNick = nextNick
	state.ActiveUser = nextUser
	state.PendingSupplements = nil

	storage.SetGroup(state)

	var sb strings.Builder

	if turnOut.IsEnding {
		storage.DeleteGroup(state.ChatroomID)
		title := turnOut.EndingTitle
		if title == "" {
			title = "【终局】"
		}
		if state.Mode == GroupModePair {
			wordsSection := ""
			if strings.TrimSpace(turnOut.PartnerWords) != "" {
				wordsSection = fmt.Sprintf("\n\n💬 终局对白：\n%s", turnOut.PartnerWords)
			}
			icon := "🏆"
			heartIcon := "❤️"
			outcomeText := "迎来了属于彼此的结局"
			if state.TeamBond <= 10 {
				icon = "🥀"
				heartIcon = "💔"
				outcomeText = "两人的距离最终未能跨越，故事在沉默与疏离中落幕"
			}
			return fmt.Sprintf("%s %s\n\n%s%s\n\n✨ @%s 与 @%s %s。\n%s 默契/心动终值：%d%%\n\n🎉 故事完结！发送【双人冒险】可开启新故事！",
				icon, title, turnOut.Story, wordsSection, state.PairNick1, state.PairNick2, outcomeText, heartIcon, state.TeamBond), nil
		}
		return fmt.Sprintf("🏆 %s\n\n%s\n\n💬 终局时刻：\n%s\n\n✨ 全体队员突破难关！小队历经 %d 回合并肩携手，最终全员凯旋！\n🔥 团队默契终值：%d%%\n\n🎉 通关大吉！发送【创建小队】可再开新征程！",
			title, turnOut.Story, turnOut.PartnerWords, state.TurnCount, state.TeamBond), nil
	}

	if state.Mode == GroupModePair {
		sb.WriteString(fmt.Sprintf("🎬【%s 的行动】\n%s\n\n", senderNick, turnOut.Story))
		sb.WriteString(fmt.Sprintf("🔥 默契/心动度：%d%%", state.TeamBond))
	} else {
		sb.WriteString(fmt.Sprintf("🎬【第 %d 回合 · %s 的行动】\n%s\n\n", state.TurnCount, senderNick, turnOut.Story))
		if turnOut.PartnerWords != "" {
			sb.WriteString(fmt.Sprintf("💬 战况配合：\n%s\n\n", turnOut.PartnerWords))
		}
		if len(turnOut.AddItems) > 0 {
			sb.WriteString(fmt.Sprintf("🎒 全队收获：【%s】\n", strings.Join(turnOut.AddItems, "、")))
		}
		if len(turnOut.RemoveItems) > 0 {
			sb.WriteString(fmt.Sprintf("⚠️ 消耗物资：【%s】\n", strings.Join(turnOut.RemoveItems, "、")))
		}
		sb.WriteString(fmt.Sprintf("🔥 团队默契：%d%% | 🎒 共享物资：%s\n\n", state.TeamBond, strings.Join(state.Inventory, "、")))
		sb.WriteString("💡【下一步战术】（回复序号或直接输入行动）：\n")
		for i, opt := range state.Options {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, opt))
		}
	}

	return sb.String(), nil
}

// FormatGroupStatus 查看群聊冒险状态
func FormatGroupStatus(state *GroupAdventureState) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋【群聊冒险状态】%s\n", state.Title))
	sb.WriteString(fmt.Sprintf("⚠️ 当前危机：%s\n", state.Crisis))
	if state.Mode == GroupModePair {
		sb.WriteString(fmt.Sprintf("👥 当事人：@%s × @%s\n", state.PairNick1, state.PairNick2))
		sb.WriteString(fmt.Sprintf("🔥 默契/心动度：%d%%\n", state.TeamBond))
		sb.WriteString("\n双方均可随时打字输入你想做的事或对白推进故事，发送【解散队伍】退出。")
	} else {
		sb.WriteString(fmt.Sprintf("🛡️ 小队人数：%d 人 | 🔥 团队默契度：%d%%\n", len(state.Members), state.TeamBond))
		for i, uid := range state.MemberOrder {
			m := state.Members[uid]
			sb.WriteString(fmt.Sprintf("  %d. %s (%s · HP:%d)\n", i+1, m.Nickname, m.Role, m.Health))
		}
		sb.WriteString(fmt.Sprintf("🎒 共享背包：%s\n", strings.Join(state.Inventory, "、")))
		sb.WriteString(fmt.Sprintf("⏳ 历经回合：%d\n", state.TurnCount))
		sb.WriteString("\n发送选项序号或输入行动继续，发送【解散队伍】退出。")
	}
	return sb.String()
}
