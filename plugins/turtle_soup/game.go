package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync"
	"time"
)

type GameSession struct {
	mu             sync.Mutex
	ChatroomID     string
	Story          *Story
	StartTime      time.Time
	CluesGiven     int
	QuestionsCount int
	Solved         bool
}

type GameManager struct {
	mu       sync.RWMutex
	sessions map[string]*GameSession // key: chatroomID or senderID
	stories  []*Story
}

func NewGameManager() *GameManager {
	return &GameManager{
		sessions: make(map[string]*GameSession),
		stories:  defaultStories,
	}
}

// StartGame 为群聊或用户开启一局新的海龟汤
func (m *GameManager) StartGame(sessionID string) (*Story, string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx := rand.IntN(len(m.stories))
	story := m.stories[idx]

	m.sessions[sessionID] = &GameSession{
		ChatroomID:     sessionID,
		Story:          story,
		StartTime:      time.Now(),
		CluesGiven:     0,
		QuestionsCount: 0,
		Solved:         false,
	}

	welcome := fmt.Sprintf("🍲【海龟汤 · 第 %d 案：%s】\n\n"+
		"📜【汤面】：\n%s\n\n"+
		"💡【玩法说明】：\n"+
		"大家可以自由提问（如：是他杀吗？死者是人类吗？），我会回答【是】/【不是】/【与此无关】。\n"+
		"卡壳了可发【海龟汤提示】，猜不出发【看汤底】揭晓真相！",
		story.ID, story.Title, story.Surface)

	return story, welcome
}

// StartCustomStory 以指定故事开局（用于原创私房好汤，可指定主题）
func (m *GameManager) StartCustomStory(sessionID string, story *Story, topic string) (*Story, string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[sessionID] = &GameSession{
		ChatroomID:     sessionID,
		Story:          story,
		StartTime:      time.Now(),
		CluesGiven:     0,
		QuestionsCount: 0,
		Solved:         false,
	}

	header := fmt.Sprintf("🍲【海龟汤 · %s】", story.Title)
	if strings.TrimSpace(topic) != "" {
		header = fmt.Sprintf("🍲【海龟汤 · %s】（定制主题：%s）", story.Title, strings.TrimSpace(topic))
	}

	welcome := fmt.Sprintf("%s\n\n"+
		"📜【汤面】：\n%s\n\n"+
		"💡【玩法说明】：\n"+
		"这碗海龟汤新鲜出炉！大家可以自由提问（如：是他杀吗？死者是人类吗？），我会回答【是】/【不是】/【与此无关】。\n"+
		"卡壳了可发【海龟汤提示】，猜不出发【看汤底】揭晓真相！",
		header, story.Surface)

	return story, welcome
}

// extractJSONObject 从模型返回内容中截取第一个完整 JSON 块
func extractJSONObject(text string) string {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start != -1 && end != -1 && end > start {
		return text[start : end+1]
	}
	return text
}

// surfaceParadoxArchetypes 汤面表层矛盾模型（主角反常举动的表象张力，不包含任何具象行业或场景名词，彻底避免模型锚定）
var surfaceParadoxArchetypes = []string{
	"反常的破坏或挥霍：表面上在极其不理智地损毁珍贵事物或浪费资源，实则是为了切断某种更致命的隐形连锁反应",
	"冷酷违逆的见死不救：表面上看似见死不救、绝情离去或冷血反常，实则是极端规则或特定认知下的最高理性守护",
	"荒诞精密的强迫行为：表面上重复着毫无逻辑、令人费解的怪异举动或仪式，实则是对抗某种无声倒计时或客观现实约束的精密测量",
	"自投罗网的致命冒险：表面上主动放弃生路、步步走向危险的自杀式举动，实则是基于全局严密推演后的唯一生还险棋",
	"欲盖弥彰的轻重倒置：表面上在竭尽全力掩饰一件微不足道的生活小事，实则背后掩盖着关乎生死的重大真相",
	"平静日常下的无声惊雷：表面上是一场再平常不过的日常生活交谈或举动，实则当事人正在生死边缘与看不见的对手进行残酷博弈",
	"盲目误判的逆向操作：表面上胸有成竹地按惯性认知采取行动，却因一个不易察觉的客观现实发生天翻地覆的反转",
}

// underlyingTruthEngines 汤底底层真相逻辑引擎（推动真相闭环的现实驱动力）
var underlyingTruthEngines = []string{
	"专业壁垒与特殊规程：利用人类社会真实存在的特定行业、工艺、技术标准或操作协议中外人鲜为人知的刚性铁律",
	"硬核物理与生理边界：光影、声波、气压、温度、重力、反射、毒理或人体生理本能等客观现实规律发挥了决定性作用",
	"信息阻断与时间窗口：关键信息的单向延迟或极短的时间差，导致双方在各自最理性的判断下撞出了意料之外的惊人结局",
	"严苛监视下的求生暗语：在第三者或危险规则的直接审视下，依靠极其隐蔽的现实线索或默契传达绝密求救信号",
	"人性习惯与宿命讽刺：看似天衣无缝、机关算尽的计划，最终恰恰被常人最朴素的一个本能生活习惯打破，震撼且唏嘘",
}

// generateAIStory 调用 ai.chat 动态熬制全新海龟汤，支持指定主题/风格/提示词
func (p *TurtleSoupPlugin) generateAIStory(timeoutSec int, topic string) (*Story, error) {
	if p.caller == nil {
		return nil, fmt.Errorf("caller not injected")
	}

	if timeoutSec <= 0 {
		timeoutSec = 45
	}

	systemPrompt := `你是一位享誉业界的顶级悬疑推理作家、海龟汤设计大师。请创作一碗极具悬疑张力、逻辑严丝合缝、结局意料之外情理之中的高水平原创海龟汤。

【四大核心创作法则】
1. 真实动机，拒绝“强行合理”：
   - 汤面中人物的一切反常、诡异举动，在当事人的视角和现实处境下必须有【极度合理且别无选择的真实动机】。
   - 严禁神经病/发疯无厘头、严禁脱离现实常理的强行硬编。
2. 精妙叙诡，悬疑感拉满：
   - 汤面必须具备极强的画面感与矛盾冲突（40-90字），充分利用读者的思维定式（如对身份、地点、动作目的的惯性误解）。
   - 汤底（120-220字）一旦揭晓，所有疑点瞬间冰释前嫌，汤面的每个字都能精准对上，带给玩家“原来是这样！拍案叫绝”的推理快感。
3. 严格禁止陈词滥调与烂梗（违禁库）：
   - 严禁“吃人肉/不知情吃了人肉”；
   - 严禁“双胞胎替换身份/人格分裂/做梦/幻觉”；
   - 严禁“鬼魂幽灵/超自然超能力/灵异玄学”；
   - 严禁“侏儒坐过山车/盲人点灯跳海”等陈旧烂俗老汤；
   - 严禁“主角其实是宠物/狗/猫/玩具/动物”等老套路（必须以人类现实社会为舞台）。
4. 拒绝模板化与场景锚定：
   - 严禁机械套用“跳伞、潜水、雪山、电梯、停电、孤岛灯塔”等被过度使用的海龟汤刻板场景。
   - 鼓励完全自由取材于人类广泛的现实社会：如医疗急救、交通物流、现代工业、商业金融、科研考古、城乡民俗、日常生活等浩瀚领域，构思独一无二的巧妙盲区。

【输出格式】严格按以下 JSON 输出，不要输出任何 markdown 代码块标记或额外寒暄：
{
  "title": "精炼引人入胜的标题（4-8字）",
  "surface": "汤面（50-90字，呈现一个表面极度反常、充满悬疑感的不可思议现场或事件）",
  "bottom": "汤底（120-220字，逻辑严密、细节闭环、有强烈反转与心理冲击的完整真相）",
  "clues": [
    "第一级提示：关于环境、时间或特殊身份背景的隐晦线索",
    "第二级提示：关于反常行为背后真实目的的关键动机线索",
    "第三级提示：直接打破思维定式、直指核心真相的关键钥匙"
  ]
}`

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

	var userMsg string
	trimmedTopic := strings.TrimSpace(topic)
	if trimmedTopic != "" {
		userMsg = fmt.Sprintf("请围绕定制主题【%s】创作一碗全新原创海龟汤。汤面悬念十足，汤底必须紧扣该主题，逻辑严谨严防烂梗，充满反转魅力与情感冲击。", trimmedTopic)
	} else {
		paradox := surfaceParadoxArchetypes[rand.IntN(len(surfaceParadoxArchetypes))]
		truthEngine := underlyingTruthEngines[rand.IntN(len(underlyingTruthEngines))]
		userMsg = fmt.Sprintf("请创作一碗全新原创的高质量海龟汤。\n【构思灵感与张力框架】\n- 汤面矛盾结构：%s\n- 汤底现实逻辑：%s\n【要求】请完全自由发散思维，在广阔的人类现实社会中自由选取背景与身份，拒绝套路化与刻板印象，构思一桩让人意料之外却又严丝合缝的惊艳谜题。", paradox, truthEngine)
	}

	disableThinking := false
	payload := aiChatPayload{
		System: systemPrompt,
		Messages: []openAIMsg{
			{Role: "user", Content: userMsg},
		},
		TimeoutSeconds: timeoutSec,
		Thinking:       &disableThinking,
	}

	b, _ := json.Marshal(payload)
	_, resBytes, err := p.caller.CallPlugin("ai.chat", map[string]string{
		"payload": string(b),
	})
	if err != nil {
		return nil, err
	}

	raw := string(resBytes)
	cleanJSON := extractJSONObject(raw)

	var s Story
	if err := json.Unmarshal([]byte(cleanJSON), &s); err != nil {
		slog.Error("[turtle_soup] 解析 AI 故事 JSON 失败", "raw", raw, "clean", cleanJSON, "err", err)
		return nil, fmt.Errorf("解析海龟汤内容失败: %w", err)
	}
	if s.Title == "" || s.Surface == "" || s.Bottom == "" {
		return nil, fmt.Errorf("AI 生成的海龟汤字段不完整")
	}
	s.ID = 999
	return &s, nil
}

// GetActiveSession 获取当前活跃游戏
func (m *GameManager) GetActiveSession(sessionID string) *GameSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[sessionID]
}

// EndGame 结束并移除会话
func (m *GameManager) EndGame(sessionID string) (*Story, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, false
	}
	delete(m.sessions, sessionID)
	return session.Story, true
}

// GetClue 获取下一条线索
func (m *GameManager) GetClue(sessionID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return "当前没有进行中的海龟汤，发送【来碗海龟汤】即可开局！"
	}

	if session.CluesGiven >= len(session.Story.Clues) {
		return "⚠️ 所有关键提示都已经给完啦！再想不出发【看汤底】直接揭晓吧！"
	}

	clue := session.Story.Clues[session.CluesGiven]
	session.CluesGiven++

	return fmt.Sprintf("💡【海龟汤提示 %d/%d】：\n%s", session.CluesGiven, len(session.Story.Clues), clue)
}

// JudgeQuestion 裁判玩家提问
func (p *TurtleSoupPlugin) judgeQuestion(session *GameSession, question, speakerName string) string {
	session.mu.Lock()
	session.QuestionsCount++
	session.mu.Unlock()

	// 0. 优先检查故事自带的 KeyMatches 精准快速预设匹配
	if session.Story != nil && len(session.Story.KeyMatches) > 0 {
		q := strings.ToLower(question)
		for kw, ans := range session.Story.KeyMatches {
			if strings.Contains(q, strings.ToLower(kw)) {
				return ans
			}
		}
	}

	// 1. 优先尝试调用 ai.chat 能力做精准剧情判定（设置 25 秒超时，避免阻塞聊天）
	if p.caller != nil {
		reply, err := p.judgeByAI(session.Story, question, speakerName)
		if err == nil && strings.TrimSpace(reply) != "" {
			return reply
		}
		slog.Warn("[turtle_soup] 裁判判定超时或失败，走规则降级", "err", err)
	}

	// 2. 降级规则判定
	return fallbackJudge(session.Story, question)
}

func (p *TurtleSoupPlugin) judgeByAI(story *Story, question, speakerName string) (string, error) {
	botName := p.getBotName()
	systemPrompt := fmt.Sprintf(`你现在是海龟汤推理派对的主持人“%s”（幽默嘴碎但极有分寸感。自称“%s”或“我”，只有在对方明确是年轻学生小孩时才可以自称“叔叔”，其余情况绝不自称叔叔）。
当前题目《%s》：
【汤面】：%s
【汤底（绝对真相）】：%s

请根据汤底真相，严肃判定玩家的问题或猜测。
规则：
1. 回答开头必须是以下标签之一：
   - 【是】：如果完全符合汤底真相；
   - 【不是】：如果不符合汤底真相；
   - 【与此无关】：如果玩家提的问题对还原案情没有实质帮助；
   - 【关键线索！】：如果切中了案件的关键转折或破案要害；
   - 【破案了！】：如果玩家基本还原了汤底的核心真相。
2. 标签后面可附带一句 10-25 字的语气点评（鼓励、调侃或吐槽，切勿直接剧透关键细节）。
3. 如果判定是【破案了！】，请在后面公布【真相揭秘】：附带完整汤底并宣布游戏获胜。`, botName, botName, story.Title, story.Surface, story.Bottom)

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

	disableThinking := false
	payload := aiChatPayload{
		System: systemPrompt,
		Messages: []openAIMsg{
			{Role: "user", Content: fmt.Sprintf("玩家 %s 提问：“%s”", speakerName, question)},
		},
		TimeoutSeconds: 25,
		Thinking:       &disableThinking,
	}

	b, _ := json.Marshal(payload)
	_, resBytes, err := p.caller.CallPlugin("ai.chat", map[string]string{
		"payload": string(b),
	})
	if err != nil {
		return "", err
	}
	reply := strings.TrimSpace(string(resBytes))
	if strings.Contains(reply, "</think>") {
		idx := strings.LastIndex(reply, "</think>")
		reply = strings.TrimSpace(reply[idx+8:])
	}
	return reply, nil
}

func fallbackJudge(story *Story, question string) string {
	q := strings.ToLower(question)
	// 简单规则判定
	if strings.Contains(q, "自杀") || strings.Contains(q, "死") {
		return "【是】确有人员伤亡或极端选择，继续顺着这个思路挖！"
	}
	if strings.Contains(q, "毒") || strings.Contains(q, "中毒") {
		return "【不是】并没有下毒这回事，别往化学毒物上想了。"
	}
	if strings.Contains(q, "钱") || strings.Contains(q, "图财") {
		return "【与此无关】跟金钱财产没什么关系。"
	}
	if strings.Contains(q, "梦") || strings.Contains(q, "做梦") {
		return "【不是】全都是现实中发生的真实经历。"
	}
	return "【与此无关】感觉这个方向离核心稍微有点偏，换个角度试试？"
}
