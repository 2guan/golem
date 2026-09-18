package main

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"time"
)

type GameState int

const (
	StateNone GameState = iota
	StateRecruiting
	StatePlaying
	StateVoting
)

type Player struct {
	Index        int    // 1-based 出场编号
	Username     string // wxid
	Nickname     string // 群显示名
	IsUndercover bool   // 是否卧底
	Word         string // 分配的词
	Alive        bool   // 是否存活
}

type Game struct {
	ChatroomID     string
	State          GameState
	HostID         string
	HostName       string
	Players        []*Player
	CivilianWord   string
	UndercoverWord string
	Round          int
	Votes          map[string]string // voterUsername -> targetUsername
	CreatedAt      time.Time
}

type GameManager struct {
	mu    sync.RWMutex
	games map[string]*Game // chatroomID -> Game
}

func NewGameManager() *GameManager {
	return &GameManager{
		games: make(map[string]*Game),
	}
}

func (m *GameManager) GetGame(chatroomID string) *Game {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.games[chatroomID]
}

func (m *GameManager) StartRecruit(chatroomID, hostID, hostName string) (*Game, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if g, ok := m.games[chatroomID]; ok && g.State != StateNone {
		return nil, "", fmt.Errorf("当前已有进行中的谁是卧底游戏，发【结束谁是卧底】可重置")
	}

	game := &Game{
		ChatroomID: chatroomID,
		State:      StateRecruiting,
		HostID:     hostID,
		HostName:   hostName,
		Players:    make([]*Player, 0),
		Votes:      make(map[string]string),
		CreatedAt:  time.Now(),
		Round:      1,
	}

	// 发起人自动报名为 1 号
	game.Players = append(game.Players, &Player{
		Index:    1,
		Username: hostID,
		Nickname: hostName,
		Alive:    true,
	})

	m.games[chatroomID] = game

	msg := fmt.Sprintf("🕵️【谁是卧底】报名开始啦！\n\n"+
		"发起人：%s\n"+
		"当前已报名（1人）：\n1. %s\n\n"+
		"👉 其他想玩的小伙伴请在群内回复【+1】或【报名】加入！\n"+
		"人数达到 3-8 人后，发起人发送【开始游戏】即可发词！", hostName, hostName)

	return game, msg, nil
}

func (m *GameManager) Join(chatroomID, userID, nickname string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	game, ok := m.games[chatroomID]
	if !ok || game.State != StateRecruiting {
		return "", fmt.Errorf("当前未开启报名，发【发起谁是卧底】即可开局")
	}

	for _, p := range game.Players {
		if p.Username == userID {
			return "", fmt.Errorf("你已经报名过啦，请勿重复+1")
		}
	}

	if len(game.Players) >= 8 {
		return "", fmt.Errorf("本局报名人数已达上限（8人），请发起人发【开始游戏】")
	}

	player := &Player{
		Index:    len(game.Players) + 1,
		Username: userID,
		Nickname: nickname,
		Alive:    true,
	}
	game.Players = append(game.Players, player)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✅ @%s 报名成功！当前已报名（%d人）：\n", nickname, len(game.Players)))
	for _, p := range game.Players {
		sb.WriteString(fmt.Sprintf("%d. %s\n", p.Index, p.Nickname))
	}
	if len(game.Players) >= 3 {
		sb.WriteString("\n👉 满 3 人即可由发起人发送【开始游戏】发词；也可继续等待更多人【+1】！")
	}

	return sb.String(), nil
}

func (m *GameManager) StartGame(chatroomID, hostID string) (*Game, []*Player, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	game, ok := m.games[chatroomID]
	if !ok || game.State != StateRecruiting {
		return nil, nil, "", fmt.Errorf("当前没有待开始的游戏")
	}

	if len(game.Players) < 3 {
		return nil, nil, "", fmt.Errorf("至少需要 3 人才能开始游戏，当前仅 %d 人报名", len(game.Players))
	}

	// 抽取词语
	cWord, uWord := RandomWordPair()
	game.CivilianWord = cWord
	game.UndercoverWord = uWord

	// 随机决定卧底数量与人选（<=6人1个卧底，>=7人2个卧底）
	undercoverCount := 1
	if len(game.Players) >= 7 {
		undercoverCount = 2
	}

	// 打乱指定卧底
	perm := rand.Perm(len(game.Players))
	for i := 0; i < undercoverCount; i++ {
		game.Players[perm[i]].IsUndercover = true
		game.Players[perm[i]].Word = uWord
	}
	for i := undercoverCount; i < len(game.Players); i++ {
		game.Players[perm[i]].IsUndercover = false
		game.Players[perm[i]].Word = cWord
	}

	// 重排发言顺序编号
	rand.Shuffle(len(game.Players), func(i, j int) {
		game.Players[i], game.Players[j] = game.Players[j], game.Players[i]
	})
	for i, p := range game.Players {
		p.Index = i + 1
	}

	game.State = StatePlaying
	game.Round = 1

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🎮【谁是卧底 · 正式开始】\n\n"+
		"👥 参赛玩家：%d人（平民 %d人，卧底 %d人）\n\n"+
		"📢【出场与发言顺序】：\n", len(game.Players), len(game.Players)-undercoverCount, undercoverCount))

	for _, p := range game.Players {
		sb.WriteString(fmt.Sprintf("%d号：%s\n", p.Index, p.Nickname))
	}

	sb.WriteString("\n⚠️ 肉丸正在给每位玩家【私聊发送专属词语】，请大家查收私聊窗口！\n" +
		"（如未收到私聊，请确认已添加肉丸好友）\n\n" +
		"🗣️【游戏规则】：\n" +
		"请大家按顺序依次在群里发一句话描述自己的词语，不要直接说出词汇本身！\n" +
		"全员描述完毕后，发【进入投票】开始抓卧底！")

	return game, game.Players, sb.String(), nil
}

func (m *GameManager) EnterVote(chatroomID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	game, ok := m.games[chatroomID]
	if !ok || game.State != StatePlaying {
		return "", fmt.Errorf("当前不是发言阶段，无法进入投票")
	}

	game.State = StateVoting
	game.Votes = make(map[string]string)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🗳️【第 %d 轮 · 投票放逐开始】\n\n可投票的目标玩家：\n", game.Round))
	for _, p := range game.Players {
		if p.Alive {
			sb.WriteString(fmt.Sprintf("%d号：%s\n", p.Index, p.Nickname))
		}
	}
	sb.WriteString("\n👉 请存活玩家直接回复【投X号】（如：投1号）进行公投！\n" +
		"当大家投票完毕或发起人发送【公布票数】时结算！")

	return sb.String(), nil
}

func (m *GameManager) Vote(chatroomID, voterID, content string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	game, ok := m.games[chatroomID]
	if !ok || game.State != StateVoting {
		return "", false, fmt.Errorf("当前不是投票阶段")
	}

	// 确认投票者是否存活
	var voter *Player
	for _, p := range game.Players {
		if p.Username == voterID && p.Alive {
			voter = p
			break
		}
	}
	if voter == nil {
		return "", false, fmt.Errorf("你已出局或不是本局玩家，无法投票")
	}

	// 解析被投目标：如 "投1号"、"投 2"、"1"
	content = strings.TrimPrefix(content, "投票")
	content = strings.TrimPrefix(content, "投")
	content = strings.TrimSuffix(content, "号")
	targetIndex, err := strconv.Atoi(strings.TrimSpace(content))
	if err != nil {
		return "", false, fmt.Errorf("投票格式不正确，请输入【投X号】（例如：投1号）")
	}

	var target *Player
	for _, p := range game.Players {
		if p.Index == targetIndex && p.Alive {
			target = p
			break
		}
	}
	if target == nil {
		return "", false, fmt.Errorf("未找到有效存活的 %d 号玩家", targetIndex)
	}

	game.Votes[voterID] = target.Username

	// 检查是否所有存活玩家都已投票
	aliveCount := 0
	for _, p := range game.Players {
		if p.Alive {
			aliveCount++
		}
	}

	allVoted := len(game.Votes) >= aliveCount
	reply := fmt.Sprintf("✅ %s 成功投给了 %d号【%s】（当前已投 %d/%d）", voter.Nickname, target.Index, target.Nickname, len(game.Votes), aliveCount)
	return reply, allVoted, nil
}

// TallyResult 投票结算结果
type TallyResult struct {
	Detail     string
	GameOver   bool
	WinnerDesc string
}

func (m *GameManager) TallyVotes(chatroomID string) (*TallyResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	game, ok := m.games[chatroomID]
	if !ok || game.State != StateVoting {
		return nil, fmt.Errorf("当前未在投票阶段")
	}

	if len(game.Votes) == 0 {
		return nil, fmt.Errorf("目前还没有人投票，请存活玩家发送【投X号】！")
	}

	// 统计票数
	voteCounts := make(map[string]int)
	for _, targetID := range game.Votes {
		voteCounts[targetID]++
	}

	// 找出最高票
	maxVotes := 0
	for _, cnt := range voteCounts {
		if cnt > maxVotes {
			maxVotes = cnt
		}
	}

	var topTargets []string
	for targetID, cnt := range voteCounts {
		if cnt == maxVotes {
			topTargets = append(topTargets, targetID)
		}
	}

	var sb strings.Builder
	sb.WriteString("📊【票数统计公布】：\n")
	for _, p := range game.Players {
		if p.Alive {
			cnt := voteCounts[p.Username]
			sb.WriteString(fmt.Sprintf("%d号 %s：%d 票\n", p.Index, p.Nickname, cnt))
		}
	}

	// 平票处理
	if len(topTargets) > 1 {
		sb.WriteString("\n⚖️ 出现平票！本轮无人出局，继续进入下一轮发言！\n")
		game.State = StatePlaying
		game.Round++
		game.Votes = make(map[string]string)
		return &TallyResult{
			Detail:   sb.String(),
			GameOver: false,
		}, nil
	}

	// 淘汰最高票者
	eliminatedID := topTargets[0]
	var eliminated *Player
	for _, p := range game.Players {
		if p.Username == eliminatedID {
			p.Alive = false
			eliminated = p
			break
		}
	}

	roleName := "【平民】"
	if eliminated.IsUndercover {
		roleName = "【卧底】⚠️"
	}

	sb.WriteString(fmt.Sprintf("\n⚰️ %d号 %s 以最高票出局！Ta的真实身份是：%s\n", eliminated.Index, eliminated.Nickname, roleName))

	// 判断胜负
	aliveCivilian := 0
	aliveUndercover := 0
	for _, p := range game.Players {
		if p.Alive {
			if p.IsUndercover {
				aliveUndercover++
			} else {
				aliveCivilian++
			}
		}
	}

	if aliveUndercover == 0 {
		// 平民获胜
		sb.WriteString(fmt.Sprintf("\n🎉🎉【游戏结束 · 平民大获全胜！】🎉🎉\n"+
			"真相揭晓：\n"+
			"平民词：【%s】\n"+
			"卧底词：【%s】\n", game.CivilianWord, game.UndercoverWord))
		delete(m.games, chatroomID)
		return &TallyResult{
			Detail:     sb.String(),
			GameOver:   true,
			WinnerDesc: "平民胜利",
		}, nil
	}

	if aliveUndercover >= aliveCivilian {
		// 卧底获胜
		sb.WriteString(fmt.Sprintf("\n😈😈【游戏结束 · 卧底绝地翻盘获胜！】😈😈\n"+
			"真相揭晓：\n"+
			"平民词：【%s】\n"+
			"卧底词：【%s】\n", game.CivilianWord, game.UndercoverWord))
		delete(m.games, chatroomID)
		return &TallyResult{
			Detail:     sb.String(),
			GameOver:   true,
			WinnerDesc: "卧底胜利",
		}, nil
	}

	// 游戏继续
	game.Round++
	game.State = StatePlaying
	game.Votes = make(map[string]string)

	sb.WriteString(fmt.Sprintf("\n👉 场上还剩 %d 名平民，%d 名卧底！\n进入第 %d 轮发言描述！所有人按顺序继续发言！", aliveCivilian, aliveUndercover, game.Round))

	return &TallyResult{
		Detail:   sb.String(),
		GameOver: false,
	}, nil
}

func (m *GameManager) EndGame(chatroomID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.games[chatroomID]; ok {
		delete(m.games, chatroomID)
		return true
	}
	return false
}
