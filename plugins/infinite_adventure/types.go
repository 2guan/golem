package main

import (
	"encoding/json"
	"strings"
	"time"
)

// HistoryMsg 对话上下文历史
type HistoryMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// PartnerProfile 冒险搭档人设
type PartnerProfile struct {
	Name        string `json:"name"`        // 姓名（如：陆沉、严策、雷蒙德、肉丸等）
	Age         int    `json:"age"`         // 年龄
	Identity    string `json:"identity"`    // 身份（如：前特战老兵、义体佣兵、落魄剑修、巡林员）
	Appearance  string `json:"appearance"`  // 外貌与身材（如：185cm，肩宽窄腰，手臂有旧疤，微哑低音）
	Personality string `json:"personality"` // 性格特质与口吻（沉稳护短、嘴硬心软、行动派）
}

// DynamicAttr 题材自适应专属属性
type DynamicAttr struct {
	Name  string `json:"name"`  // 如：体温、理智(SAN)、真元、辐射值
	Value string `json:"value"` // 如：36.2℃、80/100、15mSv
}

// SoloAdventureState 私聊 1V1 沉浸剧场状态
type SoloAdventureState struct {
	SessionID    string         `json:"session_id"`    // 玩家 wxid
	UserNickname string         `json:"user_nickname"` // 玩家昵称
	Title        string         `json:"title"`         // 剧本标题
	ThemePrompt  string         `json:"theme_prompt"`  // 原始脑洞或随机设定
	Crisis       string         `json:"crisis"`        // 核心危机
	Partner      PartnerProfile `json:"partner"`       // 男主搭档档案
	Health       int            `json:"health"`        // 玩家生命/体力值 (0~100)
	Bond         int            `json:"bond"`          // 两人羁绊度 (0~100%)
	HeartRate    int            `json:"heart_rate"`    // 心率 (bpm)
	DynamicAttrs []DynamicAttr  `json:"dynamic_attrs"` // 自适应动态属性
	Inventory    []string       `json:"inventory"`     // 随身背包物品
	CurrentScene string         `json:"current_scene"` // 当前地点/情境
	TurnCount    int            `json:"turn_count"`    // 当前回合数
	Options      []string       `json:"options"`       // 推荐的3个快捷行动
	History      []HistoryMsg   `json:"history"`       // 最近上下文历史
	UpdatedAt    time.Time      `json:"updated_at"`
	IsActive     bool           `json:"is_active"`
}

// 群聊模式常量
const (
	GroupModePair   = "pair"   // 双人锁死·全群吃瓜
	GroupModeSquad  = "squad"  // 兄弟男团·小队共斗
	GroupModePublic = "public" // 全群众筹行动
)

// SquadMember 小队成员
type SquadMember struct {
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`   // 战术身份（如：前锋先锋、战地医疗、尖兵斥候、战术智囊）
	Health   int    `json:"health"` // 生命值 (0~100)
}

// GroupAdventureState 群聊多人剧场状态
type GroupAdventureState struct {
	ChatroomID   string                  `json:"chatroom_id"`
	Mode         string                  `json:"mode"`          // pair / squad / public
	CreatorID    string                  `json:"creator_id"`    // 发起人 wxid
	CreatorNick  string                  `json:"creator_nick"`  // 发起人昵称
	Title        string                  `json:"title"`         // 剧本标题
	ThemePrompt  string                  `json:"theme_prompt"`  // 主题设定
	Crisis       string                  `json:"crisis"`        // 核心危机
	NPCLeader    *PartnerProfile         `json:"npc_leader"`    // NPC 搭档/领队
	Members      map[string]*SquadMember `json:"members"`       // 小队成员列表
	MemberOrder  []string                `json:"member_order"`  // 顺序列表
	PairUser1    string                  `json:"pair_user1"`    // 双人模式玩家1
	PairNick1    string                  `json:"pair_nick1"`
	PairUser2    string                  `json:"pair_user2"`    // 双人模式玩家2
	PairNick2    string                  `json:"pair_nick2"`
	TeamBond     int                     `json:"team_bond"`     // 团队默契度 (0~100%)
	DynamicAttrs []DynamicAttr           `json:"dynamic_attrs"` // 动态属性
	Inventory    []string                `json:"inventory"`     // 共享物资
	CurrentScene string                  `json:"current_scene"`
	TurnCount          int                     `json:"turn_count"`
	ActiveUser         string                  `json:"active_user"`          // 当前轮到抉择的玩家ID
	ActiveNick         string                  `json:"active_nick"`          // 当前轮到抉择的玩家昵称
	PendingSupplements []string                `json:"pending_supplements"`  // 当前回合非行动者补充的互动内容（对白/动作）
	Options            []string                `json:"options"`
	History            []HistoryMsg            `json:"history"`
	Status             string                  `json:"status"`        // "lobby" / "in_progress" / "finished"
	LobbyExpires       time.Time               `json:"lobby_expires"` // 上车等待超时
	UpdatedAt          time.Time               `json:"updated_at"`
}

// AdventureTurnOutput 大模型单回合结构化产出
type AdventureTurnOutput struct {
	Story        string          `json:"story"`         // 环境与危机推演（电影感描写）
	PartnerWords string          `json:"partner_words"` // 搭档的对话台词与肢体/神态微反应
	HealthDelta  int             `json:"health_delta"`  // 生命值变动 (-20 ~ +10)
	BondDelta    int             `json:"bond_delta"`    // 羁绊变动 (+3 ~ +15)
	HeartRate    int             `json:"heart_rate"`    // 实时心率 (bpm)
	AttrUpdates  json.RawMessage `json:"attr_updates"`  // 自适应属性更新
	AddItems     []string        `json:"add_items"`     // 获得物品
	RemoveItems  []string        `json:"remove_items"`  // 失去/消耗物品
	Options      []string        `json:"options"`       // 3个下一步快捷选项
	IsEnding     bool            `json:"is_ending"`     // 是否迎来结局
	EndingTitle  string          `json:"ending_title"`  // 结局称号
}

// GetDynamicAttrUpdates 解析属性更新（兼容对象数组与字符串数组）
func (a *AdventureTurnOutput) GetDynamicAttrUpdates() []DynamicAttr {
	if len(a.AttrUpdates) == 0 {
		return nil
	}
	var attrs []DynamicAttr
	if err := json.Unmarshal(a.AttrUpdates, &attrs); err == nil {
		return attrs
	}
	var strAttrs []string
	if err := json.Unmarshal(a.AttrUpdates, &strAttrs); err == nil {
		for _, s := range strAttrs {
			parts := strings.SplitN(s, ":", 2)
			if len(parts) == 2 {
				attrs = append(attrs, DynamicAttr{Name: strings.TrimSpace(parts[0]), Value: strings.TrimSpace(parts[1])})
			} else {
				attrs = append(attrs, DynamicAttr{Name: s, Value: ""})
			}
		}
		return attrs
	}
	return nil
}

// AdventureInitOutput 大模型开局生成结构化档案
type AdventureInitOutput struct {
	Title        string         `json:"title"`
	Crisis       string         `json:"crisis"`
	Partner      PartnerProfile `json:"partner"`
	DynamicAttrs []DynamicAttr  `json:"dynamic_attrs"`
	Inventory    []string       `json:"inventory"`
	OpeningScene string         `json:"opening_scene"`
	OpeningWords string         `json:"opening_words"`
	Options      []string       `json:"options"`
}
