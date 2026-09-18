package main

import (
	"log/slog"
	"strings"
	"time"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

type Config struct {
	EnableAI bool `toml:"enable_ai" comment:"当有复杂自定义需求时是否启用 AI 专属定制"`
}

type WhatToEatPlugin struct {
	plugin.ConfigAbility[Config]
	message message.Ability
	contact contact.Ability
	caller  plugin.CallerAbility
	engine  *Engine
}

func (p *WhatToEatPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "what_to_eat",
		Author:      "Golem Team",
		Version:     "1.0.0",
		Description: "今天吃什么美食盲盒插件：智能感知时段推荐老饕美食，支持偏好过滤、换菜及AI私房定制",
		Priority:    -15,
	}
}

func (p *WhatToEatPlugin) GetSubscriptions() []string {
	return []string{
		message.TypeText.Topic,
	}
}

func (p *WhatToEatPlugin) OnEvent(e *plugin.Event) (bool, error) {
	msg := e.Payload.(*plugin.Event_Message).Message
	if msg == nil {
		return false, nil
	}

	rawText := strings.TrimSpace(msg.GetContent())
	if rawText == "" {
		if td := msg.GetText(); td != nil {
			rawText = strings.TrimSpace(td.Content)
		}
	}
	if rawText == "" {
		return false, nil
	}

	sessionID := e.GetSender()
	if sessionID == "" && msg.Sender != nil {
		sessionID = msg.Sender.GetUsername()
	}

	receiver := p.resolveReceiver(e, msg)
	if receiver == nil {
		return false, nil
	}

	cleanText, wasMentioned := cleanMention(rawText)

	// 1. 帮助菜单
	if cleanText == "吃什么帮助" || cleanText == "美食帮助" {
		p.sendText(receiver, "🍲【肉丸老饕食堂 · 使用指南】\n\n"+
			"1. 发送【今天吃什么】/【吃什么】：自动根据当前饭点（早餐/午餐/下午茶/晚餐/夜宵）推荐招牌美食！\n"+
			"2. 发送【换一个】/【换道菜】：不合口味随时再换，自动规避近期重复菜品；\n"+
			"3. 偏好定制：发送【吃什么 减脂】/【吃什么 清淡】/【吃什么 爆辣】/【来点喝的】；\n"+
			"4. 避坑排除：发送【不想吃面】/【不吃辣】/【不想喝奶茶】精准避雷；\n"+
			"5. 私房定制：发送【吃什么 胃难受】/【吃什么 预算20】肉丸当场量身定制专属食谱！")
		return true, nil
	}

	// 2. 换菜指令
	if cleanText == "换一个" || cleanText == "换一盘" || cleanText == "换道菜" || cleanText == "再换一个" || cleanText == "不好吃换一个" {
		mealTime := GetCurrentMealTime(time.Now())
		dish, ok := p.engine.PickDish(sessionID, mealTime, nil, nil)
		if !ok {
			p.sendText(receiver, "⚠️ 哎哟喂，肉丸这会儿把这顿的拿手菜都给你翻了一遍了！来点【吃什么 减脂】或指定口味换个思路呗？")
			return true, nil
		}
		card := p.engine.FormatDishCard(dish, mealTime, true)
		p.sendText(receiver, card)
		return true, nil
	}

	// 3. 触发指令匹配
	isTrigger := false
	subQuery := ""

	triggers := []string{"今天吃什么", "中午吃什么", "晚上吃什么", "夜宵吃什么", "早饭吃什么", "早餐吃什么", "下午茶", "来点喝的", "喝什么", "吃什么"}
	for _, tr := range triggers {
		if strings.HasPrefix(cleanText, tr) {
			isTrigger = true
			subQuery = strings.TrimSpace(strings.TrimPrefix(cleanText, tr))
			break
		}
	}

	// 负向过滤直接触发（如：“不想吃面”、“不吃辣”）
	if !isTrigger {
		if strings.HasPrefix(cleanText, "不想吃") || strings.HasPrefix(cleanText, "不吃") {
			isTrigger = true
			subQuery = cleanText
		}
	}

	if !isTrigger {
		// 如果显式 @ 了机器人且带有吃或喝字样
		if wasMentioned && (strings.Contains(cleanText, "吃") || strings.Contains(cleanText, "喝")) {
			isTrigger = true
			subQuery = cleanText
		}
	}

	if !isTrigger {
		return false, nil
	}

	// 4. 解析餐段偏好
	now := time.Now()
	mealTime := GetCurrentMealTime(now)
	if strings.Contains(cleanText, "早") {
		mealTime = MealBreakfast
	} else if strings.Contains(cleanText, "中") || strings.Contains(cleanText, "午") {
		mealTime = MealLunch
	} else if strings.Contains(cleanText, "下午茶") || strings.Contains(cleanText, "喝") {
		mealTime = MealTea
	} else if strings.Contains(cleanText, "晚") {
		mealTime = MealDinner
	} else if strings.Contains(cleanText, "夜宵") || strings.Contains(cleanText, "宵夜") {
		mealTime = MealSupper
	}

	// 5. 判断是否属于特殊复杂诉求（走 AI 深度定制）
	complexKeywords := []string{"预算", "块钱", "胃", "病", "发烧", "生病", "招待", "聚餐", "宿舍", "减肥", "情侣", "约会", "老人", "小孩", "孕妇"}
	isComplex := false
	for _, kw := range complexKeywords {
		if strings.Contains(subQuery, kw) {
			isComplex = true
			break
		}
	}

	if isComplex && p.caller != nil {
		p.sendText(receiver, "👨‍🍳 肉丸正在翻老饕食谱，为您琢磨专属搭配...")
		go func() {
			reply, err := RecommendAI(p.caller, cleanText, mealTime)
			if err != nil {
				slog.Warn("[what_to_eat] AI 推荐失败，降级本地菜单", "err", err)
				dish, _ := p.engine.PickDish(sessionID, mealTime, nil, nil)
				if dish != nil {
					p.sendText(receiver, p.engine.FormatDishCard(dish, mealTime, false))
				}
				return
			}
			p.sendText(receiver, reply)
		}()
		return true, nil
	}

	// 6. 普通标签与过滤逻辑
	var includeTags []string
	var excludeTags []string

	if strings.Contains(subQuery, "不想吃辣") || strings.Contains(subQuery, "不吃辣") || strings.Contains(subQuery, "微辣") || strings.Contains(subQuery, "清淡") {
		excludeTags = append(excludeTags, "重口", "爆辣", "麻辣")
	}
	if strings.Contains(subQuery, "不想吃面") || strings.Contains(subQuery, "不吃面") {
		excludeTags = append(excludeTags, "面食")
	}
	if strings.Contains(subQuery, "不想吃肉") || strings.Contains(subQuery, "素") {
		excludeTags = append(excludeTags, "肉食")
	}

	if strings.Contains(subQuery, "减脂") || strings.Contains(subQuery, "轻食") || strings.Contains(subQuery, "低卡") {
		includeTags = append(includeTags, "减脂")
	}
	if strings.Contains(subQuery, "辣") && !strings.Contains(subQuery, "不") {
		includeTags = append(includeTags, "重口", "爆辣", "川菜", "湘菜")
	}
	if strings.Contains(subQuery, "面") && !strings.Contains(subQuery, "不") {
		includeTags = append(includeTags, "面食")
	}
	if strings.Contains(subQuery, "饭") && !strings.Contains(subQuery, "不") {
		includeTags = append(includeTags, "便当", "盖饭")
	}
	if strings.Contains(subQuery, "喝") || strings.Contains(subQuery, "茶") || strings.Contains(subQuery, "咖啡") {
		includeTags = append(includeTags, "饮品", "下午茶")
		mealTime = MealTea
	}

	dish, ok := p.engine.PickDish(sessionID, mealTime, includeTags, excludeTags)
	if !ok {
		// 宽松重试
		dish, ok = p.engine.PickDish(sessionID, mealTime, nil, nil)
	}

	if !ok {
		p.sendText(receiver, "⚠️ 哎呀，你这要求太刁钻，肉丸后厨库存见底啦！发【吃什么】让肉丸随缘给你来一道？")
		return true, nil
	}

	card := p.engine.FormatDishCard(dish, mealTime, false)
	p.sendText(receiver, card)
	return true, nil
}

func (p *WhatToEatPlugin) resolveReceiver(e *plugin.Event, msg *message.Message) *contact.Contact {
	receiver := p.contact.Get(e.GetSender())
	if receiver == nil {
		if msg.Sender != nil && msg.Sender.GetUsername() == e.GetSender() {
			receiver = msg.Sender
		} else if msg.Receiver != nil && msg.Receiver.GetUsername() == e.GetSender() {
			receiver = msg.Receiver
		} else {
			receiver = &contact.Contact{Username: e.GetSender()}
		}
	}
	return receiver
}

func (p *WhatToEatPlugin) sendText(to *contact.Contact, text string) {
	if p.message == nil || to == nil || strings.TrimSpace(text) == "" {
		return
	}
	msg := &message.Message{
		Receiver: to,
		Type:     message.TypeText,
		Data:     &message.Message_Text{Text: &message.TextData{Content: text}},
	}
	if _, err := p.message.Send(msg); err != nil {
		slog.Error("[what_to_eat] 发送消息失败", "err", err)
	}
}

func cleanMention(content string) (string, bool) {
	text := strings.TrimSpace(content)
	hadMention := false
	for strings.HasPrefix(text, "@") {
		hadMention = true
		idx := strings.IndexAny(text, " \t\r\n\u2005\u00a0:：,，")
		if idx > 0 {
			text = strings.TrimSpace(text[idx:])
			text = strings.TrimLeft(text, " :：,，\t\r\n\u2005\u00a0")
		} else {
			break
		}
	}
	if idx := strings.LastIndex(text, "@"); idx > 0 {
		hadMention = true
		text = strings.TrimSpace(text[:idx])
	}
	return strings.TrimSpace(text), hadMention
}

func main() {
	p := &WhatToEatPlugin{
		engine: NewEngine(),
	}
	slog.Info("[what_to_eat] 今天吃什么老饕插件启动中...")
	plugin.Start(p)
}
