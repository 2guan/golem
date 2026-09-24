package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractJSONBlock(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{
			input:    `{"title": "test"}`,
			expected: `{"title": "test"}`,
		},
		{
			input:    "Here is your json:\n```json\n{\"title\": \"test\"}\n```\nEnjoy!",
			expected: `{"title": "test"}`,
		},
		{
			input:    `Leading text {"nested": {"a": 1}} trailing text`,
			expected: `{"nested": {"a": 1}}`,
		},
	}

	for _, c := range cases {
		out := extractJSONBlock(c.input)
		if out != c.expected {
			t.Errorf("extractJSONBlock(%q) = %q; expected %q", c.input, out, c.expected)
		}
	}

	// 测试用户日志中的真实开局 JSON（带末尾逗号）
	userRaw := "```json\n" + `{
  "title": "《雪线之上》",
  "crisis": "两人在海拔四千三百米的雪山营地露营，入夜后气温骤降至零下十五度，狂风裹挟着雪粒拍打帐篷。睡袋长度不足以抵御严寒，他们不得不挤进同一个双人睡袋，背靠背缩成一团。",
  "partner": {
    "name": "横贯",
    "age": 31,
    "identity": "高山向导/地质工程师",
    "appearance": "189cm，宽肩窄腰，皮肤是常年户外活动留下的健康小麦色，眼窝深邃，鼻梁高挺，下颌线硬朗。此刻只穿着黑色抓绒内衣，袖口挽到小臂，露出结实的肌肉线条。",
    "personality": "沉稳内敛，极少废话但每句都在点子上。看似粗犷实则心细如发，照顾人时动作自然得像呼吸。偶尔会在对方紧张时用低沉嗓音说句冷幽默。",
  },
  "dynamic_attrs": [
    {"name": "寒冷指数", "value": "85%"},
    {"name": "睡袋空间", "value": "紧张"},
    {"name": "山风强度", "value": "9级"},
    {"name": "心跳距离", "value": "8cm"}
  ],
  "inventory": ["军用保温壶（内装姜茶）", "头灯", "急救毯", "半块压缩饼干"],
  "opening_scene": "帐篷外是呼啸的暴雪，风声尖锐得像金属摩擦。睡袋里弥漫着羊毛混纺的暖意，但脚尖依然冰凉。横贯背对着你，宽厚的后背几乎占满睡袋一半空间，他刚灌好热水袋递过来，呼出的热气在头灯光晕里凝成白雾。你们之间隔着一层抓绒衣料，能清晰感受到他脊背的温度。",
  "opening_words": "（把裹着布的热水袋塞到你冻僵的手里，声音低沉）“手给我，放腋下暖着。脚别乱动，会漏风。”（顿了顿）“当年在昆仑山测绘，零下二十度睡过车底。这算什么。”",
  "options": [
    "（把热水袋分一半给他，小声说）“一人一半，不然明天你手会僵。”",
    "（往他背后靠了靠，感受那份温度）“横贯，你体温好高。”",
    "（盯着帐篷顶部晃动的光影）“……你说，雪什么时候停？”"
  ]
}` + "\n```"

	extracted := extractJSONBlock(userRaw)
	cleaned := cleanJSON(extracted)
	var out AdventureInitOutput
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		t.Fatalf("failed to unmarshal user JSON after cleanJSON: %v", err)
	}
	if out.Title != "《雪线之上》" || out.Partner.Name != "横贯" {
		t.Fatalf("unexpected unmarshaled data: %+v", out)
	}

	// 测试 14:41:03 日志中由于 options 内部包含未转义双引号导致的解析失败
	userRawWithInnerQuotes := `{
  "title": "《砂锅白粥与陌生来电》",
  "crisis": "深夜十一点的街角大排档，塑料棚顶的串灯昏黄摇晃。你与邻桌的陌生男人——横贯，在老板娘端来滚烫砂锅粥的瞬间，因同时伸手去拿香菜罐，指尖在瓷罐冰凉的表面短暂相触。",
  "partner": {
    "name": "横贯",
    "age": 29,
    "identity": "自由摄影师，刚结束外拍返程",
    "appearance": "穿着洗旧的深蓝工装衬衫，袖口卷到小臂，露出线条分明的前臂和一块表盘微花的机械表。眉骨高，鼻梁挺，下颌线干净，此刻正微微蹙眉看着指尖相触的方位，眼神里有种长途跋涉后的倦怠与一丝意外的光。",
    "personality": "习惯用观察代替言语，行动直接，声音低沉平稳，偶尔冷幽默"
  },
  "dynamic_attrs": [
    {"name": "氛围温度", "value": "28%（深夜微凉，粥气蒸腾）"},
    {"name": "陌生张力", "value": "45%（指尖触碰后的短暂凝滞）"},
    {"name": "生活实感", "value": "92%（油渍斑驳的塑料桌布，远处收音机的粤语老歌）"}
  ],
  "inventory": ["半杯冰镇维他奶（杯壁挂满水珠）", "一台银色徕卡胶片机（皮质背带磨损）", "一串沾着泥土的旧钥匙"],
  "opening_scene": "路灯把影子拉得很长，你的影子和他的在潮湿的水泥地上几乎要重叠。砂锅在桌上咕嘟作响，白气混着米香猛地窜升，模糊了你们之间那罐孤零零的香菜。老板娘用方言含糊地催了句“趁热食”，他抬起眼，瞳孔里映着串灯碎碎的光，你的手指还停在香菜罐冰凉的釉面上。",
  "opening_words": "（指尖若无其事地收回，拿起自己的白瓷勺，在粥面轻轻搅了搅）“这粥熬得够火候，米油都出来了。”（抬眼看向你，语气平常得像在谈论天气）“你常来这家？”",
  "options": ["用筷子尖拨了拨粥里的瑶柱，自然接话："第一次来。朋友说这儿的砂锅粥能治失眠。"", "把香菜罐往他那边轻轻推过去，指了指他手边的小碟："你要加吗？我吃不惯这个。"", "不直接回答，反而看向他搁在隔壁空椅上的相机："拍夜景？这光圈开得够大。""]
}`

	cleanedWithQuotes := cleanJSON(userRawWithInnerQuotes)
	var out2 AdventureInitOutput
	if err := json.Unmarshal([]byte(cleanedWithQuotes), &out2); err != nil {
		// 若 cleanJSON 未能完全解决，正则提取器应成功提取
		regexOut := extractInitOutputByRegex(userRawWithInnerQuotes)
		if regexOut == nil || regexOut.Title != "《砂锅白粥与陌生来电》" {
			t.Fatalf("regex fallback extraction failed: %v", err)
		}
	} else {
		if out2.Title != "《砂锅白粥与陌生来电》" || len(out2.Options) != 3 {
			t.Fatalf("unexpected unmarshaled data with inner quotes: %+v", out2)
		}
	}
}

func TestResolveActionText(t *testing.T) {
	opts := []string{"检查车辆", "寻找掩体", "生火取暖"}

	if resolveActionText("1", opts) != "检查车辆" {
		t.Errorf("expected 检查车辆, got %s", resolveActionText("1", opts))
	}
	if resolveActionText("2", opts) != "寻找掩体" {
		t.Errorf("expected 寻找掩体, got %s", resolveActionText("2", opts))
	}
	if resolveActionText("B", opts) != "寻找掩体" {
		t.Errorf("expected 寻找掩体, got %s", resolveActionText("B", opts))
	}
	if resolveActionText("自由开枪", opts) != "自由开枪" {
		t.Errorf("expected 自由开枪, got %s", resolveActionText("自由开枪", opts))
	}
}

func TestIsSimpleChoice(t *testing.T) {
	if !isSimpleChoice("1", 3) {
		t.Error("1 should be simple choice")
	}
	if !isSimpleChoice("a", 3) {
		t.Error("a should be simple choice")
	}
	if isSimpleChoice("4", 3) {
		t.Error("4 exceeds max options")
	}
	if isSimpleChoice("我们去生火吧", 3) {
		t.Error("complex text should not be simple choice")
	}
}

func TestCleanMention(t *testing.T) {
	cases := []struct {
		input       string
		expectedTxt string
		expectedHad bool
	}{
		{
			input:       "双人冒险@横贯\u2005 在一顿饭局中一见钟情",
			expectedTxt: "双人冒险 在一顿饭局中一见钟情",
			expectedHad: true,
		},
		{
			input:       "@机器人\u2005 开启冒险 街角便利店",
			expectedTxt: "开启冒险 街角便利店",
			expectedHad: true,
		},
		{
			input:       "双人冒险 @横贯 凌晨海边看日出",
			expectedTxt: "双人冒险 凌晨海边看日出",
			expectedHad: true,
		},
		{
			input:       "双人冒险@横贯",
			expectedTxt: "双人冒险",
			expectedHad: true,
		},
		{
			input:       "1",
			expectedTxt: "1",
			expectedHad: false,
		},
	}

	for _, c := range cases {
		txt, had := cleanMention(c.input)
		if txt != c.expectedTxt || had != c.expectedHad {
			t.Errorf("cleanMention(%q) = (%q, %v); expected (%q, %v)", c.input, txt, had, c.expectedTxt, c.expectedHad)
		}
	}
}

func TestStorageManager(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test_adventure_storage.json")
	defer os.Remove(tmpFile)

	sm := NewStorageManager(tmpFile)

	// Test Solo
	solo := &SoloAdventureState{
		SessionID:    "user_123",
		UserNickname: "测试玩家",
		Title:        "《测试剧本》",
		Health:       100,
		Bond:         20,
		Inventory:    []string{"手电筒"},
	}
	sm.SetSolo(solo)

	gotSolo := sm.GetSolo("user_123")
	if gotSolo == nil || gotSolo.Title != "《测试剧本》" {
		t.Fatalf("failed to retrieve solo state")
	}

	// Test Group
	group := &GroupAdventureState{
		ChatroomID: "group_456@chatroom",
		Mode:       GroupModeSquad,
		Title:      "《群聊小队》",
		TeamBond:   30,
	}
	sm.SetGroup(group)

	gotGroup := sm.GetGroup("group_456@chatroom")
	if gotGroup == nil || gotGroup.Title != "《群聊小队》" {
		t.Fatalf("failed to retrieve group state")
	}

	sm.DeleteSolo("user_123")
	if sm.GetSolo("user_123") != nil {
		t.Error("expected solo to be deleted")
	}
}

func TestSquadLobbyLifecycle(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test_squad_lifecycle.json")
	defer os.Remove(tmpFile)

	sm := NewStorageManager(tmpFile)
	cID := "test_chat@chatroom"

	// 1. Create lobby
	msg := CreateSquadLobby(sm, cID, "user_1", "队长小明", "无限流副本")
	if !strings.Contains(msg, "队长小明") {
		t.Errorf("unexpected lobby msg: %s", msg)
	}

	// 2. Join lobby
	joinMsg, ok := JoinSquadLobby(sm, cID, "user_2", "队员小李", 5)
	if !ok || !strings.Contains(joinMsg, "队员小李") {
		t.Errorf("failed to join squad: %s", joinMsg)
	}

	// 3. Duplicate join
	dupMsg, _ := JoinSquadLobby(sm, cID, "user_2", "队员小李", 5)
	if !strings.Contains(dupMsg, "已经在队伍里") {
		t.Errorf("duplicate join should be rejected: %s", dupMsg)
	}

	state := sm.GetGroup(cID)
	if len(state.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(state.Members))
	}
}

func TestPairAdventureFreeTurns(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test_pair_turns.json")
	defer os.Remove(tmpFile)

	sm := NewStorageManager(tmpFile)
	cID := "57101231210@chatroom"

	state := &GroupAdventureState{
		ChatroomID: cID,
		Mode:       GroupModePair,
		PairUser1:  "wxid_player_a",
		PairNick1:  "小李",
		PairUser2:  "Hengguan",
		PairNick2:  "横贯",
		TurnCount:  1,
		TeamBond:   30,
		Options:    []string{"选项A", "选项B", "选项C"},
		Status:     "in_progress",
	}
	sm.SetGroup(state)

	// 1. 第三方群友发言，应被提示
	resp, err := ProcessGroupAction(nil, sm, state, "other_user", "路人甲", "我也要来")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(resp, "当前故事正由 @小李 与 @横贯 推进中") {
		t.Errorf("expected third party reminder, got: %s", resp)
	}

	// 2. 小李发言，直接推动剧情
	resp, err = ProcessGroupAction(nil, sm, state, "wxid_player_a", "小李", "我把外套脱下来递给他")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(resp, "🎬【小李 的行动】") {
		t.Errorf("expected banner 🎬【小李 的行动】, got: %s", resp)
	}
	if strings.Contains(resp, "第 2 回合") || strings.Contains(resp, "随身物品") {
		t.Errorf("expected no round number or inventory, got: %s", resp)
	}
	if strings.Contains(resp, "1. ") || strings.Contains(resp, "选项A") {
		t.Errorf("expected no options rendered for pair mode, got: %s", resp)
	}
	if strings.Contains(resp, "轮到") {
		t.Errorf("expected no turn alternation concept, got: %s", resp)
	}

	// 3. 横贯发言，直接推动剧情
	resp, err = ProcessGroupAction(nil, sm, state, "Hengguan", "横贯", "接过外套，道了声谢")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(resp, "🎬【横贯 的行动】") {
		t.Errorf("expected banner 🎬【横贯 的行动】, got: %s", resp)
	}

	// 4. 横贯连续再次发言，仍然直接推动剧情（不受轮次限制）
	resp, err = ProcessGroupAction(nil, sm, state, "Hengguan", "横贯", "拉开椅子坐到他身边")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(resp, "🎬【横贯 的行动】") {
		t.Errorf("expected banner 🎬【横贯 的行动】 on consecutive reply, got: %s", resp)
	}
}

func TestPairAdventureHeartbreakEnding(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test_pair_heartbreak.json")
	defer os.Remove(tmpFile)

	sm := NewStorageManager(tmpFile)
	cID := "57101231210@chatroom"

	state := &GroupAdventureState{
		ChatroomID: cID,
		Mode:       GroupModePair,
		PairUser1:  "wxid_player_a",
		PairNick1:  "小李",
		PairUser2:  "Hengguan",
		PairNick2:  "横贯",
		TeamBond:   15,
		TurnCount:  3,
		Status:     "in_progress",
	}
	sm.SetGroup(state)

	// 本地兜底或AI返回时，假设产生负向心动度导致 TeamBond <= 10
	// 模拟触发散场终局
	state.TeamBond = 8
	resp, err := ProcessGroupAction(nil, sm, state, "wxid_player_a", "小李", "算了吧，话不投机，我走了")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// 验证降至10以下时触发了散场终局
	if !strings.Contains(resp, "🥀") || !strings.Contains(resp, "💔") {
		t.Errorf("expected heartbreak icons 🥀 and 💔, got: %s", resp)
	}
	if !strings.Contains(resp, "故事在沉默与疏离中落幕") {
		t.Errorf("expected fallout ending text, got: %s", resp)
	}
	// 验证已自动从存储中删除
	if sm.GetGroup(cID) != nil {
		t.Error("group should be deleted after ending")
	}
}

func TestExtractTurnOutputByRegex(t *testing.T) {
	// 用户报错日志中的真实大模型产出（含 partner_words 内未转义双引号、attr_updates 为字符串数组）
	raw := "```json\n" + `{
  "story": "小李无声地侧身靠近，将那卷受潮的硝化棉导火索和半壶冷凝水轻轻塞进横贯手中，动作稳得像在交接手术器械。两人的肩膀几乎相抵，在这狭窄的通道里形成一个短暂的支撑三角。滴水声忽然密集起来，头顶传来一声低沉的岩层呻吟。",
  "partner_words": "横贯迅速接过物资塞进医疗包，同时用空出的手稳住小李的背囊带："水省着喝，导火索……也许能用来做标记。你专心前面，背后交给我。"",
  "health_delta": 0,
  "bond_delta": 7,
  "heart_rate": 95,
  "attr_updates": ["岩壁承压指数: 61% -> 59%"],
  "add_items": [],
  "remove_items": ["受潮的硝化棉导火索", "半壶浑浊的冷凝水"],
  "options": ["继续敲击岩壁侦察薄弱点", "用碎石试探性清理通道", "标记已探明的安全路径"],
  "is_ending": false,
  "ending_title": ""
}` + "\n```"

	out := extractTurnOutputByRegex(raw)
	if out == nil {
		t.Fatalf("extractTurnOutputByRegex returned nil for raw json")
	}

	if !strings.Contains(out.Story, "小李无声地侧身靠近") {
		t.Errorf("story extracted incorrectly: %s", out.Story)
	}
	if !strings.Contains(out.PartnerWords, "横贯迅速接过物资") {
		t.Errorf("partnerWords extracted incorrectly: %s", out.PartnerWords)
	}
	if out.BondDelta != 7 {
		t.Errorf("expected bond delta 7, got %d", out.BondDelta)
	}
	if len(out.Options) != 3 {
		t.Errorf("expected 3 options, got %d", len(out.Options))
	}
	if len(out.RemoveItems) != 2 {
		t.Errorf("expected 2 remove items, got %d", len(out.RemoveItems))
	}

	// 测试 sanitizeStory 防止代码块泄漏
	sanitized := sanitizeStory(raw)
	if strings.Contains(sanitized, "```") || strings.Contains(sanitized, "\"story\"") {
		t.Errorf("sanitized story still contains raw code block: %s", sanitized)
	}
	if !strings.Contains(sanitized, "小李无声地侧身靠近") {
		t.Errorf("sanitized story does not contain story content: %s", sanitized)
	}
}

func TestIsParticipantOrAdmin(t *testing.T) {
	p := &InfiniteAdventurePlugin{}

	// 1. 双人模式测试
	pairState := &GroupAdventureState{
		Mode:      GroupModePair,
		CreatorID: "user_creator",
		PairUser1: "user_a",
		PairNick1: "玩家A",
		PairUser2: "user_b",
		PairNick2: "玩家B",
	}

	if !p.isParticipantOrAdmin("room1@chatroom", "user_a", "玩家A", pairState) {
		t.Errorf("pair user1 should be allowed to end adventure")
	}
	if !p.isParticipantOrAdmin("room1@chatroom", "user_b", "玩家B", pairState) {
		t.Errorf("pair user2 should be allowed to end adventure")
	}
	if !p.isParticipantOrAdmin("room1@chatroom", "user_creator", "创建者", pairState) {
		t.Errorf("creator should be allowed to end adventure")
	}
	if p.isParticipantOrAdmin("room1@chatroom", "user_stranger", "路人甲", pairState) {
		t.Errorf("stranger should NOT be allowed to end adventure")
	}

	// 2. 小队模式测试
	squadState := &GroupAdventureState{
		Mode:        GroupModeSquad,
		CreatorID:   "captain_id",
		CreatorNick: "队长",
		Members: map[string]*SquadMember{
			"member_1": {Username: "member_1", Nickname: "队员1"},
			"member_2": {Username: "member_2", Nickname: "队员2"},
		},
		MemberOrder: []string{"captain_id", "member_1", "member_2"},
	}

	if !p.isParticipantOrAdmin("room2@chatroom", "captain_id", "队长", squadState) {
		t.Errorf("squad captain should be allowed to end adventure")
	}
	if !p.isParticipantOrAdmin("room2@chatroom", "member_1", "队员1", squadState) {
		t.Errorf("squad member_1 should be allowed to end adventure")
	}
	if !p.isParticipantOrAdmin("room2@chatroom", "member_2", "队员2", squadState) {
		t.Errorf("squad member_2 should be allowed to end adventure")
	}
	if p.isParticipantOrAdmin("room2@chatroom", "other_user", "吃瓜群众", squadState) {
		t.Errorf("unrelated squad member should NOT be allowed to end adventure")
	}
}



