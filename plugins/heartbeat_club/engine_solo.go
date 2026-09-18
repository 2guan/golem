package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"

	"github.com/sbgayhub/golem/sdk/plugin"
)

// ProcessSoloAction 处理私聊 1V1 沉浸互动
func ProcessSoloAction(caller plugin.CallerAbility, session *SoloSession, userAction string) (string, error) {
	sceneInfo, ok := AllScenes[session.Scene]
	if !ok {
		return "⚠️ 当前场景数据异常，已为你自动重置，请重新发送【去健身房】/【去小酒馆】/【回宿舍】。", nil
	}

	enrichedAction := enrichActionDesc(session.Scene, userAction)

	// 计算心率与暧昧度增量
	hrDelta := rand.IntN(6) + 3     // +3 ~ +8 bpm
	tensionDelta := rand.IntN(8) + 5 // +5 ~ +12 %

	// 调用 AI 渲染纯第一人称直接对话
	reply, err := callAISoloInteraction(caller, session, sceneInfo, enrichedAction)
	if err != nil {
		slog.Warn("[heartbeat_club] AI 剧情渲染失败，降级本地纯对话兜底", "err", err, "action", enrichedAction)
		reply = getFallbackSoloInteraction(session, sceneInfo, enrichedAction)
		slog.Info("[heartbeat_club] 采用本地兜底内容", "scene", sceneInfo.Title, "user", session.Nickname, "action", enrichedAction, "fallback_reply", reply)
	}

	// 清洗可能存在的旧格式标签
	reply = cleanLegacyTags(reply)

	// 更新存档状态
	session.HeartRate += hrDelta
	if session.HeartRate > 180 {
		session.HeartRate = 180
	}
	session.Tension += tensionDelta
	if session.Tension > 100 {
		session.Tension = 100
	}
	session.History = append(session.History,
		HistoryMsg{Role: "user", Content: userAction},
		HistoryMsg{Role: "assistant", Content: reply},
	)
	if len(session.History) > 6 {
		session.History = session.History[len(session.History)-6:]
	}

	// 末尾仅附带简短的身体指标指数
	statusCard := session.FormatStatusCard()
	fullResponse := fmt.Sprintf("%s\n\n%s", reply, statusCard)
	slog.Info("[heartbeat_club] 最终发送回复", "user", session.Nickname, "scene", sceneInfo.Title, "hr", session.HeartRate, "tension", session.Tension, "reply", fullResponse)
	return fullResponse, nil
}

// cleanLegacyTags 清理模型偶尔吐出的旧格式标签
func cleanLegacyTags(text string) string {
	res := text
	unwanted := []string{
		"🎬【微醺画面】：", "🎬【画面】：", "🎬【微醺画面】", "🎬【画面】",
		"💬 肉丸：", "💬 肉丸说：", "肉丸：", "肉丸说：",
		"💡【心跳指引】：", "💡【下一步指引】：", "💡 指引：", "💡【心跳指引】",
	}
	for _, tag := range unwanted {
		res = strings.ReplaceAll(res, tag, "")
	}
	// 去除外部可能多包的一层引号（如 “...” ）
	trimmed := strings.TrimSpace(res)
	if (strings.HasPrefix(trimmed, "“") && strings.HasSuffix(trimmed, "”")) ||
		(strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"")) {
		trimmed = strings.Trim(trimmed, "“”\"")
	}
	return strings.TrimSpace(trimmed)
}

// enrichActionDesc 将简写关键词映射为动作描述
func enrichActionDesc(scene SceneType, text string) string {
	clean := strings.TrimSpace(text)
	switch scene {
	case SceneGym:
		switch clean {
		case "贴身卧推", "卧推", "大重量卧推":
			return "准备冲刺大重量，请求肉丸站在正上方贴身辅助保护"
		case "借沐浴露", "沐浴露":
			return "更衣室淋浴间水汽蒸腾，带着一身水珠向肉丸借沐浴露"
		case "检查发力", "摸发力", "发力":
			return "让肉丸伸手贴在后背背阔肌和胸口上，感受发力紧绷与肌肉充血程度"
		case "递毛巾", "擦汗":
			return "递上温热毛巾顺势靠得很近，替肉丸擦拭喉结和脖颈上的热汗"
		}
	case SceneBar:
		switch clean {
		case "今夜特调", "调酒", "来杯酒":
			return "递给肉丸一个眼神，让他调一杯能让人彻底放下防备、带有微醺试探意味的独家特调"
		case "推杯换盏", "碰杯", "喝酒":
			return "端起酒杯与肉丸轻轻碰杯，眼神对视，看他喉结吞咽"
		case "装醉倚靠", "装醉", "靠肩膀":
			return "借着三分酒意，将头轻轻歪在肉丸宽厚结实的肩膀上"
		case "吧台耳语", "耳语", "说悄悄话":
			return "俯身撑在吧台边缘拉近距离，将温热呼吸吐在肉丸耳畔说了一句轻佻试探"
		}
	case SceneDorm:
		switch clean {
		case "挤一张床", "挤挤", "同床":
			return "借口被窝太冷，钻进了肉丸的单人床挤在一起"
		case "借穿球衣", "穿球衣", "球衣":
			return "顺手扯下肉丸换下来的宽松大号球衣套在自己身上"
		case "掰手腕", "较量":
			return "在上床下桌间抵住肘部十指相扣掰手腕较量"
		case "被窝夜谈", "夜谈", "聊聊天":
			return "黑暗中同盖一床被子，面对面低声聊起深夜的心跳秘密"
		}
	}
	return text
}

// callAISoloInteraction 调用大模型渲染纯第一人称直接对话
func callAISoloInteraction(caller plugin.CallerAbility, s *SoloSession, scene SceneInfo, action string) (string, error) {
	systemPrompt := fmt.Sprintf(`你现在就是微信对面的“肉丸”本人。这是一场即时微信面对面聊天对戏，严禁写第三人称小说旁白！
【你的人物设定】：
肉丸，35岁北京爷们，前职业电竞老将，179cm/95kg微壮饱满身材。表面糙汉嘴碎、爱开玩笑，骨子里懂生活懂体贴，喜欢男人，极具荷尔蒙性张力与熟男魅力。
自称规则：全程第一人称自称“我”或偶尔自称“哥”，绝不自称“叔叔”。

【当前场景】：%s（%s）
【当前身体状态】：❤️心率 %d bpm，🔥暧昧度 %d%%

【核心对戏规则 - 务必严格遵守】：
1. 纯第一人称直接对话：你必须像微信聊天对戏一样，直接打字跟我说话！
2. 严禁小说叙事旁白：绝对不要写任何“肉丸看着你”、“肉丸的大手搭在...”这种旁观者小说叙事！
3. 允许自然的短括号微动作：如果需要配合动作神态，可以直接写在简短的中文括号里（如“（一把扶住你肩膀拉过来）”、“（忍不住低笑了一声，低头看你）”），然后自然接着说话；
4. 语言风格：接地气的北京老哥口吻，声音微哑低沉，嘴碎逗趣又充满荷尔蒙拉扯，直球又带宠溺；
5. 绝不要任何标题或指引：不要带任何“【画面】”、“【对话】”、“肉丸：”等标签，不要在文末提示用户该怎么做；
6. 严格合规：严禁低俗露骨或敏感词，用成熟男人的语气、微动作与拉扯感制造心跳；
7. 字数控制在60-120字左右，像日常微信对戏一样干脆利落。

对方刚刚对你发送：“%s”，请直接回复你说的话：`,
		scene.Title, scene.Tagline, s.HeartRate, s.Tension, action)

	type openAIMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type aiChatPayload struct {
		System         string      `json:"system"`
		Messages       []openAIMsg `json:"messages"`
		TimeoutSeconds int         `json:"timeout_seconds,omitempty"`
		Thinking       *bool       `json:"thinking,omitempty"`
	}

	// 装配历史
	msgs := make([]openAIMsg, 0)
	for _, h := range s.History {
		msgs = append(msgs, openAIMsg{Role: h.Role, Content: h.Content})
	}
	msgs = append(msgs, openAIMsg{Role: "user", Content: action})

	disableThinking := false
	payload := aiChatPayload{
		System:         systemPrompt,
		Messages:       msgs,
		TimeoutSeconds: 35,
		Thinking:       &disableThinking,
	}

	b, _ := json.Marshal(payload)
	_, resBytes, err := caller.CallPlugin("ai.chat", map[string]string{
		"payload": string(b),
	})
	if err != nil {
		return "", err
	}

	rawReply := strings.TrimSpace(string(resBytes))
	if rawReply == "" {
		return "", fmt.Errorf("empty reply from ai.chat")
	}

	slog.Info("[heartbeat_club] 大模型返回剧情内容", "scene", scene.Title, "user", s.Nickname, "action", action, "llm_reply", rawReply)
	return rawReply, nil
}

// getFallbackSoloInteraction 本地离线高质量纯第一人称对话兜底
func getFallbackSoloInteraction(s *SoloSession, scene SceneInfo, action string) string {
	switch s.Scene {
	case SceneGym:
		variants := []string{
			"（一把稳稳托住你的肩膀拉过来）这就没力气了？刚才不是嘴还挺硬么。\n行了，往我胸口靠着歇会儿，手抓稳了，有我在这儿托着你摔不着。深呼吸缓缓，待会儿带你冲凉去。",
			"（深吸了一口气，顺手扣住你的后颈把你拉近）你小子胆子挺肥啊，在器械区就敢这么撩火？\n心跳都被你蹭快了。赶紧老实站好，待会儿进了更衣室我可不饶你。",
			"（双手贴在你紧绷发酸的肩胛骨上揉捏）转过身去，崩得跟块铁板似的。放松点，酸就喊出来，别硬忍着。\n平时看着挺板正，这后背线条练得还真不错。",
		}
		return variants[rand.IntN(len(variants))]

	case SceneBar:
		variants := []string{
			"（修长的大手若有若无地覆在你杯沿上，低头直视着你低低笑了一声）\n眼神这么直勾勾的，我看你不是不胜酒力，是心神乱了。第一杯还没喝完呢，就想跟我套近乎？",
			"（撑在吧台上倾过身子，微热的呼吸带着酒气拂过你耳畔）\n平时看着挺矜持，喝两口酒倒是挺主动。今晚这间酒吧就咱俩，你想聊多深，我都陪你。",
		}
		return variants[rand.IntN(len(variants))]

	case SceneDorm:
		variants := []string{
			"（顺势伸手将你往怀里一带，下巴抵着你的发顶低低笑了一声）\n嘴上说着不要，靠过来倒是挺快。冷就把手揣进我被窝里，哥的身板够暖和吧？",
			"（侧过身撑着头看你，伸手揉了一把你的后颈）\n大半夜的折腾什么呢？睡不着就老实靠着，再乱动，哥的心跳都要被你蹭快了。",
		}
		return variants[rand.IntN(len(variants))]
	}
	return "（伸手轻轻搭在你肩上，低头笑了笑）怎么了？有心事随时跟我说，在这儿没别人，听你的。"
}
