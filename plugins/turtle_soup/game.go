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

// StartCustomStory 以指定故事开局（用于原创私房好汤）
func (m *GameManager) StartCustomStory(sessionID string, story *Story) (*Story, string) {
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

	welcome := fmt.Sprintf("🍲【私房海龟汤 · %s】\n\n"+
		"📜【汤面】：\n%s\n\n"+
		"💡【玩法说明】：\n"+
		"这可是独家压箱底的私房好汤！大家可自由提问（如：是他杀吗？死者是人类吗？），我会回答【是】/【不是】/【与此无关】。\n"+
		"卡壳了可发【海龟汤提示】，猜不出发【看汤底】揭晓真相！",
		story.Title, story.Surface)

	return story, welcome
}

// generateAIStory 调用 ai.chat 动态熬制全新海龟汤
func (p *TurtleSoupPlugin) generateAIStory(timeoutSec int) (*Story, error) {
	if p.caller == nil {
		return nil, fmt.Errorf("caller not injected")
	}

	if timeoutSec <= 0 {
		timeoutSec = 120
	}

	systemPrompt := `你是一个高分悬疑推理作家。请创作一个原创、逻辑自洽、极其反转出人意料的经典海龟汤故事。
严格按以下 JSON 格式输出，不要带任何 markdown 代码块或多余文字：
{
  "title": "简短故事标题（4-8字）",
  "surface": "汤面（50-100字，描述一个看似极其荒诞诡异、不可思议的现场或事件）",
  "bottom": "汤底（120-220字，合乎情理但极具反转的真相细节）",
  "clues": [
    "第一级提示：关于环境或身份的线索",
    "第二级提示：关于关键行为或心理动机的线索",
    "第三级提示：关于破案最核心秘密的线索"
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

	enableThinking := true
	payload := aiChatPayload{
		System: systemPrompt,
		Messages: []openAIMsg{
			{Role: "user", Content: "请立刻创作一碗高质量的全新原创海龟汤。"},
		},
		TimeoutSeconds: timeoutSec,
		Thinking:       &enableThinking,
	}

	b, _ := json.Marshal(payload)
	_, resBytes, err := p.caller.CallPlugin("ai.chat", map[string]string{
		"payload": string(b),
	})
	if err != nil {
		return nil, err
	}

	text := strings.TrimSpace(string(resBytes))
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var s Story
	if err := json.Unmarshal([]byte(text), &s); err != nil {
		return nil, err
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

	// 优先尝试调用 ai.chat 能力做精准剧情判定（设置 15 秒超时，避免阻塞聊天）
	if p.caller != nil {
		reply, err := p.judgeByAI(session.Story, question, speakerName)
		if err == nil && strings.TrimSpace(reply) != "" {
			return reply
		}
		slog.Warn("[turtle_soup] 私房判定超时或失败，走规则降级", "err", err)
	}

	// 降级规则判定
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
		TimeoutSeconds: 30,
		Thinking:       &disableThinking,
	}

	b, _ := json.Marshal(payload)
	_, resBytes, err := p.caller.CallPlugin("ai.chat", map[string]string{
		"payload": string(b),
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(resBytes)), nil
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
