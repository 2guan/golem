package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sbgayhub/golem/sdk/plugin"
)

// StartSoloAdventure 开启私聊 1V1 沉浸冒险
func StartSoloAdventure(caller plugin.CallerAbility, storage *StorageManager, sessionID, nickname, themePrompt string) (string, error) {
	initData, err := GenerateOpening(caller, themePrompt, nickname)
	if err != nil {
		return "", err
	}

	state := &SoloAdventureState{
		SessionID:    sessionID,
		UserNickname: nickname,
		Title:        initData.Title,
		ThemePrompt:  themePrompt,
		Crisis:       initData.Crisis,
		Partner:      initData.Partner,
		Health:       100,
		Bond:         20,
		HeartRate:    78,
		DynamicAttrs: initData.DynamicAttrs,
		Inventory:    initData.Inventory,
		CurrentScene: initData.OpeningScene,
		TurnCount:    1,
		Options:      initData.Options,
		History: []HistoryMsg{
			{Role: "assistant", Content: fmt.Sprintf("%s\n%s", initData.OpeningScene, initData.OpeningWords)},
		},
		UpdatedAt: time.Now(),
		IsActive:  true,
	}

	storage.SetSolo(state)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✨ %s\n", state.Title))
	sb.WriteString(fmt.Sprintf("🎯【当前情境】：%s\n\n", state.Crisis))

	sb.WriteString(fmt.Sprintf("👤【搭档档案】：%s（%d岁 · %s）\n", state.Partner.Name, state.Partner.Age, state.Partner.Identity))
	sb.WriteString(fmt.Sprintf("形象气场：%s\n性格口吻：%s\n\n", state.Partner.Appearance, state.Partner.Personality))

	sb.WriteString(fmt.Sprintf("🎬【场景】：\n%s\n\n", initData.OpeningScene))
	sb.WriteString(fmt.Sprintf("💬 %s：\n%s\n\n", state.Partner.Name, initData.OpeningWords))

	sb.WriteString(formatSoloStatusMini(state))
	sb.WriteString("\n\n")

	sb.WriteString("💡【行动指引】（回复序号，或直接打字输入任意自由动作）：\n")
	for i, opt := range state.Options {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, opt))
	}
	sb.WriteString("\n（随时发送【查看状态】查看背包，或发送【退出】随时存盘离开）")

	return sb.String(), nil
}

// ProcessSoloAction 处理私聊 1V1 回合推进
func ProcessSoloAction(caller plugin.CallerAbility, storage *StorageManager, state *SoloAdventureState, rawAction string) (string, error) {
	actionText := resolveActionText(rawAction, state.Options)

	turnOut, err := AdvanceTurn(caller, state, actionText)
	if err != nil {
		return "", err
	}

	// 更新数值
	state.TurnCount++
	state.Health += turnOut.HealthDelta
	if state.Health > 100 {
		state.Health = 100
	}
	if state.Health < 0 {
		state.Health = 0
	}

	state.Bond += turnOut.BondDelta
	if state.Bond > 100 {
		state.Bond = 100
	}

	if turnOut.HeartRate > 0 {
		state.HeartRate = turnOut.HeartRate
	}

	// 更新动态属性
	for _, up := range turnOut.GetDynamicAttrUpdates() {
		found := false
		for i := range state.DynamicAttrs {
			if state.DynamicAttrs[i].Name == up.Name {
				state.DynamicAttrs[i].Value = up.Value
				found = true
				break
			}
		}
		if !found {
			state.DynamicAttrs = append(state.DynamicAttrs, up)
		}
	}

	// 背包更新
	for _, item := range turnOut.AddItems {
		item = strings.TrimSpace(item)
		if item != "" && !slices.Contains(state.Inventory, item) {
			state.Inventory = append(state.Inventory, item)
		}
	}
	for _, item := range turnOut.RemoveItems {
		item = strings.TrimSpace(item)
		state.Inventory = slices.DeleteFunc(state.Inventory, func(s string) bool {
			return s == item
		})
	}

	if len(turnOut.Options) > 0 {
		state.Options = turnOut.Options
	}

	// 记录上下文
	state.History = append(state.History,
		HistoryMsg{Role: "user", Content: fmt.Sprintf("玩家行动：%s", actionText)},
		HistoryMsg{Role: "assistant", Content: fmt.Sprintf("%s\n%s", turnOut.Story, turnOut.PartnerWords)},
	)
	if len(state.History) > 20 {
		state.History = state.History[len(state.History)-20:]
	}

	storage.SetSolo(state)

	var sb strings.Builder

	// 检查是否结局或阵亡
	if state.Health <= 0 {
		storage.DeleteSolo(state.SessionID)
		return fmt.Sprintf("💀【冒险折戟】\n%s\n\n你因体力与伤势耗尽倒在%s怀中，他拼尽全力护住你，但风暴将你们吞没……\n\n最终羁绊指数：%d%%\n发送【开启冒险】可开启全新轮回！", turnOut.Story, state.Partner.Name, state.Bond), nil
	}

	if turnOut.IsEnding {
		storage.DeleteSolo(state.SessionID)
		endingTitle := turnOut.EndingTitle
		if endingTitle == "" {
			endingTitle = "【终局·曙光与誓约】"
		}
		return fmt.Sprintf("🏆 %s\n\n%s\n\n💬 %s：\n%s\n\n✨ 历经 %d 个回合，你们走过了这段旅程，彼此的心跳与默契在此刻定格。\n❤️ 最终契合度：%d%%\n\n🎉 本篇完结！发送【开启冒险】或【随机故事】可体验全新故事！",
			endingTitle, turnOut.Story, state.Partner.Name, turnOut.PartnerWords, state.TurnCount, state.Bond), nil
	}

	sb.WriteString(fmt.Sprintf("🎬【第 %d 回合】\n%s\n\n", state.TurnCount, turnOut.Story))
	sb.WriteString(fmt.Sprintf("💬 %s：\n%s\n\n", state.Partner.Name, turnOut.PartnerWords))

	if len(turnOut.AddItems) > 0 {
		sb.WriteString(fmt.Sprintf("🎒 获得物品：【%s】\n", strings.Join(turnOut.AddItems, "、")))
	}
	if len(turnOut.RemoveItems) > 0 {
		sb.WriteString(fmt.Sprintf("⚠️ 消耗/遗失：【%s】\n", strings.Join(turnOut.RemoveItems, "、")))
	}

	sb.WriteString(formatSoloStatusMini(state))
	sb.WriteString("\n\n")

	sb.WriteString("💡【下一步行动】（直接回复序号或打字自由输入）：\n")
	for i, opt := range state.Options {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, opt))
	}

	return sb.String(), nil
}

// FormatSoloStatus 查看完整状态与背包
func FormatSoloStatus(state *SoloAdventureState) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋【当前冒险状态】%s\n", state.Title))
	sb.WriteString(fmt.Sprintf("👤 搭档：%s（%s · %d岁）\n", state.Partner.Name, state.Partner.Identity, state.Partner.Age))
	sb.WriteString(fmt.Sprintf("❤️ 生命值：%d / 100\n", state.Health))
	sb.WriteString(fmt.Sprintf("🔥 羁绊默契：%d%%\n", state.Bond))
	sb.WriteString(fmt.Sprintf("💓 实时心率：%d bpm\n", state.HeartRate))

	if len(state.DynamicAttrs) > 0 {
		sb.WriteString("📊 特殊属性：")
		for _, a := range state.DynamicAttrs {
			sb.WriteString(fmt.Sprintf("[%s: %s] ", a.Name, a.Value))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("🎒 随身背包：%s\n", strings.Join(state.Inventory, "、")))
	sb.WriteString(fmt.Sprintf("⏳ 历经回合：%d\n", state.TurnCount))
	sb.WriteString("\n直接输入你的行动或序号继续冒险，或发送【退出】离开。")
	return sb.String()
}

func formatSoloStatusMini(state *SoloAdventureState) string {
	parts := []string{
		fmt.Sprintf("❤️生命 %d", state.Health),
		fmt.Sprintf("🔥羁绊 %d%%", state.Bond),
		fmt.Sprintf("💓心率 %dbpm", state.HeartRate),
	}
	for _, a := range state.DynamicAttrs {
		parts = append(parts, fmt.Sprintf("%s %s", a.Name, a.Value))
	}
	return strings.Join(parts, " | ")
}

func resolveActionText(raw string, options []string) string {
	trimmed := strings.TrimSpace(raw)
	// 如果是数字 1, 2, 3
	if idx, err := strconv.Atoi(trimmed); err == nil && idx >= 1 && idx <= len(options) {
		return options[idx-1]
	}
	// 如果是 A, B, C / a, b, c
	upper := strings.ToUpper(trimmed)
	if len(upper) == 1 && upper[0] >= 'A' && upper[0] <= 'C' {
		idx := int(upper[0] - 'A')
		if idx < len(options) {
			return options[idx]
		}
	}
	return trimmed
}
