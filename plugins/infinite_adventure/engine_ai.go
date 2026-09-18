package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"

	"github.com/sbgayhub/golem/sdk/plugin"
)

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

func callAIChat(caller plugin.CallerAbility, system string, msgs []openAIMsg) (string, error) {
	if caller == nil {
		return "", fmt.Errorf("caller ability is nil")
	}
	disableThinking := false
	payload := aiChatPayload{
		System:         system,
		Messages:       msgs,
		TimeoutSeconds: 40,
		Thinking:       &disableThinking,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	_, resBytes, err := caller.CallPlugin("ai.chat", map[string]string{
		"payload": string(b),
	})
	if err != nil {
		return "", err
	}

	resStr := strings.TrimSpace(string(resBytes))
	if resStr == "" {
		return "", fmt.Errorf("empty reply from ai.chat")
	}
	return resStr, nil
}

func extractJSONBlock(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "```json"); idx != -1 {
		raw = raw[idx+7:]
		if end := strings.Index(raw, "```"); end != -1 {
			raw = raw[:end]
		}
	} else if idx := strings.Index(raw, "```"); idx != -1 {
		raw = raw[idx+3:]
		if end := strings.Index(raw, "```"); end != -1 {
			raw = raw[:end]
		}
	}

	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start != -1 && end != -1 && end > start {
		return strings.TrimSpace(raw[start : end+1])
	}
	return strings.TrimSpace(raw)
}

func cleanJSON(s string) string {
	// 修复对象/数组末尾多余逗号 {"a": 1,} -> {"a": 1}
	reComma := regexp.MustCompile(`,\s*([\}\]])`)
	return reComma.ReplaceAllString(s, "$1")
}

func findStringMatch(s, pattern string) string {
	re := regexp.MustCompile(pattern)
	if m := re.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func extractInitOutputByRegex(raw string) *AdventureInitOutput {
	title := findStringMatch(raw, `"(?:title|故事篇名)"\s*:\s*"([^"]+)"`)
	scene := findStringMatch(raw, `"(?:opening_scene|场景)"\s*:\s*"((?:[^"\\]|\\.)*)"`)
	if title == "" || scene == "" {
		return nil
	}

	crisis := findStringMatch(raw, `"(?:crisis|情境|背景)"\s*:\s*"((?:[^"\\]|\\.)*)"`)
	words := findStringMatch(raw, `"(?:opening_words|对白)"\s*:\s*"((?:[^"\\]|\\.)*)"`)
	name := findStringMatch(raw, `"(?:name|名字)"\s*:\s*"([^"]+)"`)
	identity := findStringMatch(raw, `"(?:identity|身份)"\s*:\s*"([^"]+)"`)
	appearance := findStringMatch(raw, `"(?:appearance|外貌)"\s*:\s*"((?:[^"\\]|\\.)*)"`)
	personality := findStringMatch(raw, `"(?:personality|性格)"\s*:\s*"((?:[^"\\]|\\.)*)"`)

	var options []string
	optRe := regexp.MustCompile(`"(?:options|选项)"\s*:\s*\[([\s\S]*?)\]`)
	if match := optRe.FindStringSubmatch(raw); len(match) > 1 {
		optBlock := match[1]
		lines := strings.Split(optBlock, "\n")
		for _, l := range lines {
			l = strings.TrimSpace(l)
			l = strings.TrimPrefix(l, "\"")
			l = strings.TrimSuffix(l, ",")
			l = strings.TrimSuffix(l, "\"")
			l = strings.TrimSpace(l)
			if len(l) > 2 {
				options = append(options, l)
			}
		}
	}
	if len(options) == 0 {
		options = []string{
			"注视着对方，轻声打破沉默",
			"调整姿态，顺应眼前的氛围",
			"移开视线，端起手边的物品掩饰心绪",
		}
	}

	return &AdventureInitOutput{
		Title:        title,
		Crisis:       crisis,
		OpeningScene: scene,
		OpeningWords: words,
		Options:      options,
		Partner: PartnerProfile{
			Name:        name,
			Identity:    identity,
			Appearance:  appearance,
			Personality: personality,
			Age:         28,
		},
		DynamicAttrs: []DynamicAttr{
			{Name: "心动氛围", Value: "45%"},
			{Name: "默契指数", Value: "35%"},
		},
		Inventory: []string{"贴身随身物", "温热的茶水"},
	}
}

func sanitizeStory(s string) string {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "```") || (strings.HasPrefix(s, "{") && strings.Contains(s, "\"story\"")) {
		if out := extractTurnOutputByRegex(s); out != nil && out.Story != "" {
			return out.Story
		}
	}
	return s
}

func extractTurnOutputByRegex(raw string) *AdventureTurnOutput {
	// 提取 story
	story := findStringMatch(raw, `"(?:story|剧情|旁白)"\s*:\s*"((?:[^"\\]|\\.)*)"`)
	if story == "" {
		re := regexp.MustCompile(`"(?:story|剧情|旁白)"\s*:\s*"([\s\S]*?)"\s*,\s*"(?:partner_words|health_delta|bond_delta|heart_rate|attr_updates|add_items)`)
		if m := re.FindStringSubmatch(raw); len(m) > 1 {
			story = strings.TrimSpace(m[1])
		}
	}
	if story == "" {
		return nil
	}

	// 提取 partner_words
	partnerWords := findStringMatch(raw, `"(?:partner_words|搭档对白|对白)"\s*:\s*"((?:[^"\\]|\\.)*)"`)
	if partnerWords == "" {
		re := regexp.MustCompile(`"(?:partner_words|搭档对白|对白)"\s*:\s*"([\s\S]*?)"\s*,\s*"(?:health_delta|bond_delta|heart_rate|attr_updates|add_items)`)
		if m := re.FindStringSubmatch(raw); len(m) > 1 {
			partnerWords = strings.TrimSpace(m[1])
		}
	}

	healthDelta := 0
	if m := regexp.MustCompile(`"health_delta"\s*:\s*(-?\d+)`).FindStringSubmatch(raw); len(m) > 1 {
		healthDelta, _ = strconv.Atoi(m[1])
	}

	bondDelta := 5
	if m := regexp.MustCompile(`"bond_delta"\s*:\s*(-?\d+)`).FindStringSubmatch(raw); len(m) > 1 {
		bondDelta, _ = strconv.Atoi(m[1])
	}

	heartRate := 88
	if m := regexp.MustCompile(`"heart_rate"\s*:\s*(\d+)`).FindStringSubmatch(raw); len(m) > 1 {
		heartRate, _ = strconv.Atoi(m[1])
	}

	isEnding := false
	if m := regexp.MustCompile(`"is_ending"\s*:\s*(true|false)`).FindStringSubmatch(raw); len(m) > 1 {
		isEnding = (m[1] == "true")
	}

	endingTitle := findStringMatch(raw, `"(?:ending_title|终局标题)"\s*:\s*"([^"]+)"`)

	extractList := func(pattern string) []string {
		var items []string
		re := regexp.MustCompile(pattern)
		if match := re.FindStringSubmatch(raw); len(match) > 1 {
			block := strings.ReplaceAll(match[1], "\n", ",")
			for _, part := range strings.Split(block, ",") {
				part = strings.TrimSpace(part)
				part = strings.Trim(part, "\"")
				part = strings.Trim(part, "'")
				part = strings.TrimSpace(part)
				if part != "" {
					items = append(items, part)
				}
			}
		}
		return items
	}

	options := extractList(`"(?:options|选项)"\s*:\s*\[([\s\S]*?)\]`)
	addItems := extractList(`"(?:add_items|获得物品)"\s*:\s*\[([\s\S]*?)\]`)
	removeItems := extractList(`"(?:remove_items|失去物品|消耗物品)"\s*:\s*\[([\s\S]*?)\]`)

	return &AdventureTurnOutput{
		Story:        story,
		PartnerWords: partnerWords,
		HealthDelta:  healthDelta,
		BondDelta:    bondDelta,
		HeartRate:    heartRate,
		AddItems:     addItems,
		RemoveItems:  removeItems,
		Options:      options,
		IsEnding:     isEnding,
		EndingTitle:  endingTitle,
	}
}

// GenerateOpening 生成冒险开局与人设
func GenerateOpening(caller plugin.CallerAbility, themePrompt, userNickname string) (*AdventureInitOutput, error) {
	systemPrompt := `你是一位极富生活洞察力、镜头语言美感与真实人性温度的顶级现代故事叙事引擎。
你的任务是根据当事人与用户设定的脑洞（若未指定则完全自主发挥），展开一段极具真实临场感、呼吸感与情感张力的双人沉浸互动情境。

【创作原则 —— 拒绝任何固定示例与套路，全权交由你自主自由创作】：
1. 严禁使用任何固化的场景模板与陈词滥调（严禁千篇一律地写暴雨避雨、停电点蜡烛等被过度使用的套路）！生活万象皆可入戏，每次开局都必须是新鲜、独特、具有原创灵气与呼吸感的全新情境；
2. 深度结合当事人与输入的主题脑洞进行人性化推演；若玩家未限定主题，请完全凭借你的生活观察与美学直觉自主构思，题材完全自由开放；
3. 真实生活质感与电影感：
   - 捕捉环境的光影流动、空气温度、细微声响与材质触感；
   - 细腻刻画两人之间的空间物理距离、眼神交汇、不经意的肢体动作与微妙的情感拉扯；
4. 严禁出戏词汇：严禁在台词或描写中提及“剧本”、“系统”、“任务”、“副本”、“玩家”、“NPC”等机械字眼；
5. 引号规范：JSON 内部对白必须使用全角中文引号「...」或 “...”，严禁在字符串内使用英文半角双引号 "；
6. 必须输出严格合法的 JSON 对象。

JSON 格式规范：
{
  "title": "文学故事篇名",
  "crisis": "当下情境设定（生动交代两人目前身处的具体空间、正在发生的事情、周围氛围与关系张力，40-80字）",
  "partner": {
    "name": "搭档名字",
    "age": 28,
    "identity": "身份或职业",
    "appearance": "外貌与神态特征（体貌身形、衣着、眼神神采、嗓音特点）",
    "personality": "性格特质与口吻"
  },
  "dynamic_attrs": [
    {"name": "自适应属性1（根据情境自然拟定）", "value": "初始值"},
    {"name": "自适应属性2", "value": "初始值"}
  ],
  "inventory": ["贴合当前情境的具体物品1", "物品2", "物品3"],
  "opening_scene": "开局场景描写（光线、温度、声响与两人的相对位置，80-140字）",
  "opening_words": "第一句破冰对白与神态微动作（微动作写在全角中文括号内，台词使用中文引号）",
  "options": ["推荐应对方向1", "推荐应对方向2", "推荐应对方向3"]
}`

	userPrompt := fmt.Sprintf("当事人：%s\n玩家设定的主题脑洞：%s\n请立即为他们生成专属的互动剧情开局：", userNickname, themePrompt)
	if strings.TrimSpace(themePrompt) == "" {
		userPrompt = fmt.Sprintf("当事人：%s\n玩家未指定主题，请发挥你的最高创作水准，完全自主构想一个极具新鲜感、生活实感与情感细腻度的全新情境（不要受任何既有范式限制）。请立即展开：", userNickname)
	}

	resp, err := callAIChat(caller, systemPrompt, []openAIMsg{{Role: "user", Content: userPrompt}})
	if err != nil {
		slog.Warn("[infinite_adventure] AI生成开局失败，触发本地高品质兜底", "err", err)
		return getFallbackOpening(themePrompt, userNickname), nil
	}

	jsonStr := extractJSONBlock(resp)
	var output AdventureInitOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		cleaned := cleanJSON(jsonStr)
		if err2 := json.Unmarshal([]byte(cleaned), &output); err2 != nil {
			slog.Warn("[infinite_adventure] 解析开局JSON失败，尝试正则字段提取", "err", err, "err2", err2, "raw", resp)
			if regexOut := extractInitOutputByRegex(resp); regexOut != nil {
				return regexOut, nil
			}
			return getFallbackOpening(themePrompt, userNickname), nil
		}
	}

	return &output, nil
}

// AdvanceTurn 单回合剧情推进
func AdvanceTurn(caller plugin.CallerAbility, state *SoloAdventureState, userAction string) (*AdventureTurnOutput, error) {
	systemPrompt := fmt.Sprintf(`你现在是双男主文字剧情的主持人（DM）兼搭档角色【%s】的扮演者。
【故事背景】：%s
【搭档档案】：%s，%d岁，%s，外貌：%s，性格：%s
【当前状态】：默契/心动 %d%%，心率 %d bpm
【当前物品】：%s
【当前属性】：%s

【对戏与演进准则】：
1. 剧情推演：真实推进玩家行动带来的环境变化与生活/剧情进展，富有生活质感与细腻镜头感，严禁出现“剧本”、“系统”、“任务”等破坏代入感的字眼；
2. 搭档反馈：生动演绎【%s】的台词与微表情（必须用中文括号包裹神态动作，如：（拉开易拉罐递给你，眼神带着笑意）“慢点喝，没人跟你抢”）；
3. 情感推拉：贴合当前题材基调（若是日常浪漫则突出温馨宠溺、心动暧昧与眼神肢体推拉；若是破案探险则突出默契信任）；
4. 数值推演：合理调整羁绊/心动度增量（通常 +4 ~ +10%%），如发现新物品或消耗旧物品请列出；
5. 回合节奏：当故事进入高潮或达成温馨终局时，可判定 is_ending=true 并给出结局称号；
6. 必须输出严格合法的 JSON 对象。

JSON 格式规范：
{
  "story": "环境与事件推进描写（60-120字）",
  "partner_words": "搭档对话及括号微动作（30-70字）",
  "health_delta": 0,
  "bond_delta": 6,
  "heart_rate": 88,
  "attr_updates": [{"name": "属性名", "value": "更新值"}],
  "add_items": [],
  "remove_items": [],
  "options": ["下一步推荐行动1", "下一步推荐行动2", "下一步推荐行动3"],
  "is_ending": false,
  "ending_title": ""
}`,
		state.Partner.Name,
		state.Title,
		state.Partner.Name, state.Partner.Age, state.Partner.Identity, state.Partner.Appearance, state.Partner.Personality,
		state.Bond, state.HeartRate,
		strings.Join(state.Inventory, "、"),
		formatAttrs(state.DynamicAttrs),
		state.Partner.Name,
	)

	msgs := make([]openAIMsg, 0)
	historySlice := state.History
	if len(historySlice) > 19 {
		historySlice = historySlice[len(historySlice)-19:]
	}
	for _, h := range historySlice {
		msgs = append(msgs, openAIMsg{Role: h.Role, Content: h.Content})
	}
	msgs = append(msgs, openAIMsg{Role: "user", Content: fmt.Sprintf("玩家行动：%s", userAction)})

	resp, err := callAIChat(caller, systemPrompt, msgs)
	if err != nil {
		slog.Warn("[infinite_adventure] AI推进回合失败，触发本地兜底", "err", err)
		return getFallbackTurn(state, userAction), nil
	}

	jsonStr := extractJSONBlock(resp)
	var output AdventureTurnOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		cleaned := cleanJSON(jsonStr)
		if err2 := json.Unmarshal([]byte(cleaned), &output); err2 != nil {
			slog.Warn("[infinite_adventure] 解析回合JSON失败，尝试正则字段提取", "err", err, "err2", err2, "raw", resp)
			if regexOut := extractTurnOutputByRegex(resp); regexOut != nil && regexOut.Story != "" {
				regexOut.Story = sanitizeStory(regexOut.Story)
				return regexOut, nil
			}
			return getFallbackTurn(state, userAction), nil
		}
	}

	output.Story = sanitizeStory(output.Story)
	return &output, nil
}

// AdvanceGroupTurn 群聊回合剧情推进
func AdvanceGroupTurn(caller plugin.CallerAbility, state *GroupAdventureState, actorNick, actorAction string, supplements []string) (*AdventureTurnOutput, error) {
	partnerName := "搭档"
	if state.NPCLeader != nil && state.NPCLeader.Name != "" {
		partnerName = state.NPCLeader.Name
	}

	var systemPrompt string
	if state.Mode == GroupModePair {
		otherNick := state.PairNick2
		if actorNick == state.PairNick2 {
			otherNick = state.PairNick1
		}

		systemPrompt = fmt.Sprintf(`你现在是双人互动故事的【纯旁白与场景叙事DM】。
【故事背景】：%s
【当事人】：【%s】与【%s】
【本次发言/行动者】：【%s】
【本次行动内容】：%s
【现场同伴】：【%s】
【当前默契/心动度】：%d%%

【对戏铁律】：
1. 绝对严禁替代【%s】或【%s】说话或做动作！两人都是群聊里的真实人类，对白与动作由他们本人打字输入；
2. 你的唯一职责是作为【电影镜头般的客观叙事旁白】，生动聚焦描摹【%s】刚才的发言或举动所引发的具体环境反馈、物理声响、空气中两人的氛围流动以及空间距离张力（80-140字）；
3. 双方均可自由打字推动故事，options 留空数组 []，严禁生成任何预设选项；
4. partner_words 请留空字符串 ""；
5. 【心动/默契度增减评分准则】（bond_delta 支持正负值）：
   - 直球心意表白/深情相拥/决定性定情举动：+20 ~ +35（若两人已深情互表心意或确定关系，可直接补满至 100 并触发终局）；
   - 细致关怀/眼神交汇/肢体细节（如递热饮、披外套、指尖相触、拉近距离）：+10 ~ +18；
   - 普通日常/平淡试探：+3 ~ +8；
   - 敷衍冷落/刻意保持距离/生硬回避：-5 ~ -12；
   - 恶语相向/言语伤害/冷嘲热讽/拒绝触碰/拂袖离去：-15 ~ -30；
6. 【结局判定】：
   - 【圆满终局】：当两人互动深入、心动/默契度接近或达到 100%%（或两人的言行动作自然走向告别、确定关系、温情相拥、落幕离去等收尾时刻）时，判定为完结，将 is_ending 设为 true，并在 ending_title 中给出诗意电影感终局标题（如：《漫长夏日的落幕》、《风雪止息时》等），story 输出温情升华的终局旁白；
   - 【心碎/散场终局】：当心动/默契度降至 10%% 以下，或两人产生严重冲突、冷漠离场、情感彻底破裂时，判定为遗憾完结，将 is_ending 设为 true，并在 ending_title 填写如《渐行渐远》、《擦肩而过》、《形同陌路》等伤感标题，story 输出一段清冷沉寂、各自走向不同方向的散场落幕描写；
   - 未到收尾节点时保持 is_ending=false；
7. 严禁出现“剧本”、“系统”、“任务”、“回合”、“玩家”等出戏字眼；
8. 必须输出严格合法的 JSON 对象。

JSON 格式规范：
{
  "story": "客观环境与现场氛围变化描写（若为终局则为温情升华的电影感收尾描写，80-140字）",
  "partner_words": "",
  "health_delta": 0,
  "bond_delta": 5,
  "heart_rate": 88,
  "attr_updates": [],
  "add_items": [],
  "remove_items": [],
  "options": [],
  "is_ending": false,
  "ending_title": ""
}`,
			state.Title,
			state.PairNick1, state.PairNick2,
			actorNick,
			actorAction,
			otherNick,
			state.TeamBond,
			state.PairNick1, state.PairNick2,
			actorNick,
		)
	} else {
		systemPrompt = fmt.Sprintf(`你现在是多人小队剧情推演的叙事DM。
【故事背景】：%s
【模式】：%s
【队伍成员】：%s
【团队默契】：%d%%
【当前物品】：%s
【动态属性】：%s

【对戏准则】：
1. 聚焦群友【%s】刚才采取的行动，生动推进现实情境与小队的互动变化，严禁出现“剧本”、“系统”等出戏词汇；
2. 重点描写小队成员之间的战术配合、危机化解与信任增进；
3. 输出严格合法的 JSON 对象。

JSON 格式规范：
{
  "story": "事件进展与环境描写（60-120字）",
  "partner_words": "领队/NPC的回应与动作（30-70字）",
  "health_delta": 0,
  "bond_delta": 5,
  "heart_rate": 90,
  "attr_updates": [],
  "add_items": [],
  "remove_items": [],
  "options": ["下一步推荐战术1", "下一步推荐战术2", "下一步推荐战术3"],
  "is_ending": false,
  "ending_title": ""
}`,
			state.Title,
			state.Mode,
			formatGroupMembers(state),
			state.TeamBond,
			strings.Join(state.Inventory, "、"),
			formatAttrs(state.DynamicAttrs),
			actorNick,
		)
	}

	msgs := make([]openAIMsg, 0)
	historySlice := state.History
	if len(historySlice) > 19 {
		historySlice = historySlice[len(historySlice)-19:]
	}
	for _, h := range historySlice {
		msgs = append(msgs, openAIMsg{Role: h.Role, Content: h.Content})
	}
	userContent := fmt.Sprintf("【%s】的行动：%s", actorNick, actorAction)
	if len(supplements) > 0 {
		userContent += fmt.Sprintf("；同伴补充互动：%s", strings.Join(supplements, "；"))
	}
	msgs = append(msgs, openAIMsg{Role: "user", Content: userContent})

	resp, err := callAIChat(caller, systemPrompt, msgs)
	if err != nil {
		slog.Warn("[infinite_adventure] 群聊AI推进失败，触发本地兜底", "err", err)
		return getFallbackGroupTurn(state, actorNick, actorAction, partnerName), nil
	}

	jsonStr := extractJSONBlock(resp)
	var output AdventureTurnOutput
	if err := json.Unmarshal([]byte(jsonStr), &output); err != nil {
		cleaned := cleanJSON(jsonStr)
		if err2 := json.Unmarshal([]byte(cleaned), &output); err2 != nil {
			slog.Warn("[infinite_adventure] 群聊解析回合JSON失败，尝试正则字段提取", "err", err, "err2", err2, "raw", resp)
			if regexOut := extractTurnOutputByRegex(resp); regexOut != nil && regexOut.Story != "" {
				regexOut.Story = sanitizeStory(regexOut.Story)
				return regexOut, nil
			}

			trimmed := strings.TrimSpace(resp)
			isJSONLike := strings.Contains(trimmed, "```") || strings.Contains(trimmed, "\"story\"") || strings.Contains(trimmed, "“story”") || strings.HasPrefix(trimmed, "{")
			if len(trimmed) > 20 && !isJSONLike {
				opt1 := fmt.Sprintf("看向%s，轻声打破沉默", actorNick)
				opt2 := "挪得更近一些，顺手把身边的物品递过去"
				opt3 := "靠在一旁闭上眼睛，任由呼吸慢慢平复"
				if state.Mode == GroupModePair {
					opt1 = fmt.Sprintf("注视着%s，回应他的眼神或动作", actorNick)
					opt2 = "轻声说出自己心中的想法"
					opt3 = "配合他的节奏，静静享受这一刻"
				}
				return &AdventureTurnOutput{
					Story:       trimmed,
					BondDelta:   rand.IntN(4) + 4,
					HeartRate:   88,
					Options:     []string{opt1, opt2, opt3},
					IsEnding:    false,
					EndingTitle: "",
				}, nil
			}
			return getFallbackGroupTurn(state, actorNick, actorAction, partnerName), nil
		}
	}

	output.Story = sanitizeStory(output.Story)
	return &output, nil
}

func formatAttrs(attrs []DynamicAttr) string {
	if len(attrs) == 0 {
		return "无"
	}
	parts := make([]string, 0, len(attrs))
	for _, a := range attrs {
		parts = append(parts, fmt.Sprintf("%s: %s", a.Name, a.Value))
	}
	return strings.Join(parts, " | ")
}

func formatGroupMembers(state *GroupAdventureState) string {
	if state.Mode == GroupModePair {
		return fmt.Sprintf("双男主搭档：【%s】与【%s】", state.PairNick1, state.PairNick2)
	}
	parts := make([]string, 0, len(state.Members))
	for _, m := range state.Members {
		parts = append(parts, fmt.Sprintf("%s(%s·HP:%d)", m.Nickname, m.Role, m.Health))
	}
	return strings.Join(parts, "、")
}

// 本地离线兜底
func getFallbackOpening(themePrompt, userNickname string) *AdventureInitOutput {
	title := "《风吹过的夏天》"
	crisis := "落日黄昏，两人坐在天台台阶上吹着晚风，晚霞染红半边天，微风拂过衣角，带来初夏特有的清新气息与远处的低沉车流。"
	scene := "傍晚的橘红余晖把两人的影子拉得很长。天台的风吹散了白日的燥热，远处城市的喧嚣被落日镀上一层暖金色的光晕，周围安静得能听见彼此平稳的呼吸。"
	words := "（转头看着你，眉眼在夕阳的柔光下显得格外舒展，嘴角带着放松的浅笑）“今天站在这里吹吹风挺好的。尝尝这罐饮料，特意给你留的。”"
	opt := []string{
		"接过来握在手心，偏头冲他笑笑",
		"靠在身后的栏杆上，指着天边的晚霞跟他闲聊",
		"安静看着远方的日落，感受身侧传来的温度与微风",
	}

	if strings.TrimSpace(themePrompt) != "" {
		title = fmt.Sprintf("《%s》", themePrompt)
		crisis = fmt.Sprintf("围绕【%s】展开的情境刚刚拉开帷幕，微光流转，空气中弥漫着微妙而专注的氛围，两人的视线在当下悄然交汇。", themePrompt)
		scene = "四周的光影与声响仿佛在这一刻悄然隐退，彼此的距离在这一方空间里变得格外清晰。空气中流动着某种未言明的引力，故事的序幕正徐徐铺展。"
		words = "（微微侧过头注视着你，眼底带着一丝若有若无的光彩，声音放得很轻）“好巧……没想到能在这里遇见你。”"
		opt = []string{
			"迎上他的视线，自然地接下话茬",
			"轻抿一口杯中的饮品，微笑着打破沉默",
			"低头调整了一下呼吸，轻声回应他的话",
		}
	}

	return &AdventureInitOutput{
		Title:  title,
		Crisis: crisis,
		Partner: PartnerProfile{
			Name:        "严策",
			Age:         28,
			Identity:    "建筑设计师 / 摄影师",
			Appearance:  "185cm，肩宽挺拔，穿浅灰色连帽卫衣，神态放松，笑起来眉眼柔和，清润低音",
			Personality: "沉稳细致、嘴硬心软、行动派且极其体贴",
		},
		DynamicAttrs: []DynamicAttr{
			{Name: "生活惬意度", Value: "45%"},
			{Name: "默契指数", Value: "35%"},
		},
		Inventory:    []string{"温热湿纸巾", "常温黑咖啡", "薄荷糖"},
		OpeningScene: scene,
		OpeningWords: words,
		Options:      opt,
	}
}

func getFallbackTurn(state *SoloAdventureState, userAction string) *AdventureTurnOutput {
	return &AdventureTurnOutput{
		Story:        fmt.Sprintf("你的举动打破了僵局。面对【%s】，四周的阴影似乎有所收敛，两人的呼吸在严寒中交织成白雾。", userAction),
		PartnerWords: fmt.Sprintf("（%s上前一步挡在你身前，微哑的低音带着不容抗拒的稳重）“做得好，按你说的办。后背完全交给我，别怕。”", state.Partner.Name),
		HealthDelta:  -rand.IntN(4),
		BondDelta:    rand.IntN(6) + 4,
		HeartRate:    state.HeartRate + rand.IntN(10) - 2,
		AttrUpdates:  json.RawMessage(`[{"name": "体温", "value": "36.2℃"}]`),
		AddItems:     nil,
		RemoveItems:  nil,
		Options: []string{
			"向他靠得更近一些，观察前方掩体",
			"拿出战术折刀递给他配合探路",
			"轻声在他耳边说几句打气的话",
		},
		IsEnding:    false,
		EndingTitle: "",
	}
}

func getFallbackGroupTurn(state *GroupAdventureState, actorNick, actorAction, partnerName string) *AdventureTurnOutput {
	if state.Mode == GroupModePair {
		return &AdventureTurnOutput{
			Story:        fmt.Sprintf("【%s】的举动打破了安静。微弱的光线勾勒出两人的轮廓，空气里的温热渐渐漫开，目光在昏暗中安静交织，心跳也悄然加速。", actorNick),
			PartnerWords: "",
			HealthDelta:  0,
			BondDelta:    func() int {
				if strings.Contains(actorAction, "走") || strings.Contains(actorAction, "离") || strings.Contains(actorAction, "分") || strings.Contains(actorAction, "算") || strings.Contains(actorAction, "冷") || strings.Contains(actorAction, "停") {
					return -(rand.IntN(6) + 8)
				}
				return rand.IntN(5) + 4
			}(),
			HeartRate:    88,
			AttrUpdates:  nil,
			AddItems:     nil,
			RemoveItems:  nil,
			Options: []string{
				fmt.Sprintf("看向%s，轻声打破沉默", actorNick),
				"挪得更近一些，顺手把身边的外套递过去",
				"靠在一旁闭上眼睛，任由呼吸慢慢平复",
			},
			IsEnding:    false,
			EndingTitle: "",
		}
	}
	return &AdventureTurnOutput{
		Story:        fmt.Sprintf("【%s】果断出手：%s！小队各成员瞬间找到了支撑点，战局险象环生却配合默契！", actorNick, actorAction),
		PartnerWords: fmt.Sprintf("（%s大吼一声，一把拉拽住险些滑落的队友）“全员跟上！阵型别散！”", partnerName),
		HealthDelta:  -rand.IntN(3),
		BondDelta:    rand.IntN(5) + 3,
		HeartRate:    110,
		AttrUpdates:  nil,
		AddItems:     nil,
		RemoveItems:  nil,
		Options: []string{
			"掩护后排成员迅速转移",
			"集火压制前方突发危机",
			"清点全队剩余弹药与体力",
		},
		IsEnding:    false,
		EndingTitle: "",
	}
}
