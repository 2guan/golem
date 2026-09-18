package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/sbgayhub/golem/sdk/plugin"
)

type Engine struct {
	mu      sync.Mutex
	history map[string][]string // sessionID -> recent dish names
	menu    []Dish
}

func NewEngine() *Engine {
	return &Engine{
		history: make(map[string][]string),
		menu:    defaultMenu,
	}
}

// GetCurrentMealTime 根据时间判断当前最适宜的餐段
func GetCurrentMealTime(t time.Time) MealTime {
	hour := t.Hour()
	switch {
	case hour >= 6 && hour < 10:
		return MealBreakfast
	case hour >= 10 && hour < 14:
		return MealLunch
	case hour >= 14 && hour < 17:
		return MealTea
	case hour >= 17 && hour < 21:
		return MealDinner
	default:
		return MealSupper
	}
}

func MealTimeTitle(m MealTime) string {
	switch m {
	case MealBreakfast:
		return "清晨苏醒能量站（早餐）"
	case MealLunch:
		return "打工人午饭回血时刻（午餐）"
	case MealTea:
		return "三点几嘞做咩啊摸鱼下午茶"
	case MealDinner:
		return "犒劳生活的仪式感晚饭（晚餐）"
	case MealSupper:
		return "灵魂深夜食堂（罪恶夜宵）"
	default:
		return "美味饭点"
	}
}

// FilterDishes 根据餐段与偏好标签过滤可用菜品
func (e *Engine) FilterDishes(mealTime MealTime, includeTags, excludeTags []string, excludeNames []string) []Dish {
	var candidates []Dish

	for _, d := range e.menu {
		// 检查最近是否刚推荐过
		if containsString(excludeNames, d.Name) {
			continue
		}

		// 检查餐段匹配
		timeMatch := false
		for _, mt := range d.MealTimes {
			if mt == mealTime {
				timeMatch = true
				break
			}
		}
		if !timeMatch {
			continue
		}

		// 检查排除标签（如：不想吃辣、不吃面）
		hasExcludedTag := false
		for _, ex := range excludeTags {
			if strings.Contains(d.Name, ex) || strings.Contains(d.Category, ex) || containsTag(d.Tags, ex) {
				hasExcludedTag = true
				break
			}
		}
		if hasExcludedTag {
			continue
		}

		// 检查包含标签（如：减脂、清淡）
		if len(includeTags) > 0 {
			hasIncludeTag := false
			for _, in := range includeTags {
				if strings.Contains(d.Name, in) || strings.Contains(d.Category, in) || containsTag(d.Tags, in) {
					hasIncludeTag = true
					break
				}
			}
			if !hasIncludeTag {
				continue
			}
		}

		candidates = append(candidates, d)
	}

	return candidates
}

// PickDish 挑选一道菜品并记入历史
func (e *Engine) PickDish(sessionID string, mealTime MealTime, includeTags, excludeTags []string) (*Dish, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	recent := e.history[sessionID]
	candidates := e.FilterDishes(mealTime, includeTags, excludeTags, recent)

	// 如果候选库被排除干净了，清空该会话历史重置
	if len(candidates) == 0 {
		candidates = e.FilterDishes(mealTime, includeTags, excludeTags, nil)
		recent = nil
	}

	if len(candidates) == 0 {
		return nil, false
	}

	idx := rand.IntN(len(candidates))
	chosen := candidates[idx]

	recent = append(recent, chosen.Name)
	if len(recent) > 5 {
		recent = recent[len(recent)-5:]
	}
	e.history[sessionID] = recent

	return &chosen, true
}

func (e *Engine) FormatDishCard(dish *Dish, mealTime MealTime, isReroll bool) string {
	rerollHeader := ""
	if isReroll {
		rerollHeader = "🔄 【肉丸老饕挑剔换菜】：\n得嘞，刚才那道入不了法眼是吧？肉丸给您重新端上一盘：\n\n"
	}

	tagsStr := strings.Join(dish.Tags, " · ")
	return fmt.Sprintf("%s🍲【肉丸老饕食堂 · 今日专属餐盘】\n\n"+
		"⏰ 饭点时空：%s\n"+
		"🍽️ 今日招牌：【%s】\n"+
		"🏷️ 风格属性：%s\n"+
		"🔥 罪恶评级：%s\n\n"+
		"📖【菜品档案】：\n%s\n\n"+
		"🥢【肉丸老饕私房秘籍】：\n%s\n\n"+
		"💡 还不合胃口？发【换一个】随时再换，或发【不想吃面/想吃清淡/吃什么 减脂】精准点菜！",
		rerollHeader, MealTimeTitle(mealTime), dish.Name, tagsStr, dish.Calories, dish.Description, dish.MeatballTip)
}

// RecommendAI 当有复杂自定义需求时调用 ai.chat 定制
func RecommendAI(caller plugin.CallerAbility, userDemand string, mealTime MealTime) (string, error) {
	if caller == nil {
		return "", fmt.Errorf("caller not available")
	}

	systemPrompt := fmt.Sprintf(`你是肉丸（35岁北京人，前电竞职业选手，95kg微胖但注重生活品质的美食饕餮，嘴碎幽默但懂吃懂生活。平时自称“肉丸”或“我”，只有在对方明确是年轻学生或小孩时才可以自称“叔叔”，其余情况绝不自称叔叔）。
现在是【%s】时间段。
玩家向你提出专门的美食需求：“%s”。

请以肉丸本人的第一人称口吻（自称肉丸），为他量身推荐1-2种最地道贴合的美食搭配！
格式要求：
🍲【肉丸老饕专属定制菜单】
🍽️ 推荐美味：菜品名称
🏷️ 风味标签：标签1 · 标签2
📖 为何推荐：幽默风趣地说明为何这道菜完美契合他当下的需求（50-80字）
🥢 私房吃法：肉丸独家讲究吃法或避坑指南（30-60字）

字数控制在200字以内，语气地道生动像熟人老大哥，不要官腔。`, MealTimeTitle(mealTime), userDemand)

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
			{Role: "user", Content: userDemand},
		},
		TimeoutSeconds: 20,
		Thinking:       &disableThinking,
	}

	b, _ := json.Marshal(payload)
	_, resBytes, err := caller.CallPlugin("ai.chat", map[string]string{
		"payload": string(b),
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(resBytes)), nil
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func containsTag(tags []string, target string) bool {
	for _, t := range tags {
		if strings.Contains(t, target) || strings.Contains(target, t) {
			return true
		}
	}
	return false
}
