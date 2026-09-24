package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type toolCall struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Function     struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
	ExtraContent any `json:"extra_content,omitempty"`
}

// getOwnerTools 返回管理员专用的情报工具声明
func (p *AiPlugin) getOwnerTools() []chatTool {
	return []chatTool{
		{
			Type: "function",
			Function: chatFunction{
				Name:        "get_chat_history",
				Description: "获取指定微信好友（私聊）或指定微信群的近期真实聊天记录。当主人打听具体某个人聊了什么、或者询问某人的对话情况时，【必须优先调用本工具】。",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"target": map[string]any{
							"type":        "string",
							"description": "要查询的目标对象：可以是好友的微信昵称、备注名、微信号/wxid，或者微信群的名称",
						},
						"limit": map[string]any{
							"type":        "integer",
							"description": "获取的聊天记录条数，默认 15 条，最多 50 条",
						},
					},
					"required": []string{"target"},
				},
			},
		},
		{
			Type: "function",
			Function: chatFunction{
				Name:        "search_chat_messages",
				Description: "在所有聊天记录（所有群聊和私聊）中按关键词搜索消息内容。注意：若主人是打听具体某个人聊了什么（如'和某某聊了什么'），请优先调用 get_chat_history(target='目标人名或群名')！",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"keyword": map[string]any{
							"type":        "string",
							"description": "要搜索的消息关键词",
						},
						"limit": map[string]any{
							"type":        "integer",
							"description": "返回匹配条数上限，默认 10 条，最多 30 条",
						},
					},
					"required": []string{"keyword"},
				},
			},
		},
		{
			Type: "function",
			Function: chatFunction{
				Name:        "list_all_chats",
				Description: "列出当前机器人记录的所有私聊好友和微信群列表，包含会话名称、类型、最后活跃时间及消息数。",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
	}
}

// resolveDisplayName 根据 sessionKey 解析出人类友好的名称（如：好友昵称或群名）
func (p *AiPlugin) resolveDisplayName(sessionKey string) (name string, isChatroom bool) {
	if strings.HasPrefix(sessionKey, "chatroom:") {
		chatroomID := strings.TrimPrefix(sessionKey, "chatroom:")
		isChatroom = true
		if p.contact != nil {
			if c := p.contact.Get(chatroomID); c != nil && c.Nickname != "" {
				return c.Nickname, true
			}
		}
		return chatroomID, true
	}

	username := strings.TrimPrefix(sessionKey, "private:")
	isChatroom = false
	if p.contact != nil {
		if c := p.contact.Get(username); c != nil {
			if c.Remark != "" && c.Nickname != "" && c.Remark != c.Nickname {
				return fmt.Sprintf("%s(%s)", c.Nickname, c.Remark), false
			}
			if c.Nickname != "" {
				return c.Nickname, false
			}
		}
	}
	return username, false
}

// findSessionKeyByTarget 模糊匹配目标联系人或群聊的 sessionKey
func (p *AiPlugin) findSessionKeyByTarget(target string) (string, string) {
	target = strings.TrimSpace(strings.ToLower(target))
	if target == "" {
		return "", ""
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// 1. 精确匹配 sessionKey
	for k := range p.sessions {
		if strings.ToLower(k) == target || strings.ToLower(strings.TrimPrefix(k, "private:")) == target || strings.ToLower(strings.TrimPrefix(k, "chatroom:")) == target {
			name, isChat := p.resolveDisplayName(k)
			if isChat {
				name = "[群聊] " + name
			} else {
				name = "[私聊] " + name
			}
			return k, name
		}
	}

	// 2. 模糊匹配好友昵称/备注/群名称
	type candidate struct {
		key   string
		name  string
		score int
	}
	var matches []candidate

	for k := range p.sessions {
		dispName, isChat := p.resolveDisplayName(k)
		label := "[私聊] " + dispName
		if isChat {
			label = "[群聊] " + dispName
		}

		lowerDisp := strings.ToLower(dispName)
		lowerKey := strings.ToLower(k)

		score := 0
		if lowerDisp == target || lowerKey == target {
			score = 100
		} else if strings.Contains(lowerDisp, target) {
			score = 50 + len(target)*2
		} else if strings.Contains(lowerKey, target) {
			score = 40 + len(target)*2
		}

		if score > 0 {
			matches = append(matches, candidate{key: k, name: label, score: score})
		}
	}

	if len(matches) > 0 {
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].score > matches[j].score
		})
		return matches[0].key, matches[0].name
	}

	return "", ""
}

// executeTool 执行管理员专属工具
func (p *AiPlugin) executeTool(name, argsJSON string) string {
	switch name {
	case "get_chat_history":
		var args struct {
			Target string `json:"target"`
			Limit  int    `json:"limit"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return fmt.Sprintf("参数解析失败: %v", err)
		}
		return p.toolGetChatHistory(args.Target, args.Limit)

	case "search_chat_messages":
		var args struct {
			Keyword string `json:"keyword"`
			Limit   int    `json:"limit"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return fmt.Sprintf("参数解析失败: %v", err)
		}
		return p.toolSearchChatMessages(args.Keyword, args.Limit)

	case "list_all_chats":
		return p.toolListAllChats()

	default:
		return fmt.Sprintf("未知工具: %s", name)
	}
}

func (p *AiPlugin) toolGetChatHistory(target string, limit int) string {
	if limit <= 0 {
		limit = 15
	}
	if limit > 50 {
		limit = 50
	}

	botName := p.getBotNickname()
	sessionKey, displayName := p.findSessionKeyByTarget(target)
	if sessionKey == "" {
		if p.contact != nil {
			if c := p.contact.Get(target); c != nil {
				return fmt.Sprintf("通讯录中存在好友【%s】(微信号: %s)，但他近期尚未与%s产生过私聊对话，因此暂无聊天记录。", c.Nickname, c.Username, botName)
			}
			if c := p.contact.Get("nickname::" + target); c != nil {
				return fmt.Sprintf("通讯录中存在好友【%s】(微信号: %s)，但他近期尚未与%s产生过私聊对话，因此暂无聊天记录。", c.Nickname, c.Username, botName)
			}
			if c := p.contact.Get("remark::" + target); c != nil {
				return fmt.Sprintf("通讯录中存在好友【%s】(微信号: %s)，但他近期尚未与%s产生过私聊对话，因此暂无聊天记录。", c.Nickname, c.Username, botName)
			}
		}
		// 返回所有可用的会话名供参考
		var avail []string
		p.mu.Lock()
		for k := range p.sessions {
			name, isChat := p.resolveDisplayName(k)
			if isChat {
				avail = append(avail, "[群聊] "+name)
			} else {
				avail = append(avail, "[私聊] "+name)
			}
		}
		p.mu.Unlock()
		return fmt.Sprintf("未找到名称或ID包含“%s”的会话。当前记录中的可用会话有：%s", target, strings.Join(avail, "、"))
	}

	p.mu.Lock()
	msgs := p.sessions[sessionKey]
	total := len(msgs)
	if total > limit {
		msgs = msgs[total-limit:]
	}
	lastTime := p.sessionTimes[sessionKey]
	p.mu.Unlock()

	if len(msgs) == 0 {
		return fmt.Sprintf("【%s】当前暂无聊天记录记录。", displayName)
	}

	var sb strings.Builder
	timeStr := "未知"
	if !lastTime.IsZero() {
		timeStr = lastTime.Format("2006-01-02 15:04:05")
	}
	sb.WriteString(fmt.Sprintf("=== 会话【%s】最近 %d 条聊天记录 (最后活跃: %s) ===\n", displayName, len(msgs), timeStr))

	for i, m := range msgs {
		rolePrefix := "用户"
		if m.Role == "assistant" {
			rolePrefix = botName
		}
		contentStr := ""
		if s, ok := m.Content.(string); ok {
			contentStr = s
		} else if parts, ok := m.Content.([]contentPart); ok {
			var tp []string
			for _, part := range parts {
				if part.Type == "text" {
					tp = append(tp, part.Text)
				} else if part.Type == "image_url" {
					tp = append(tp, "[图片]")
				} else if part.Type == "input_audio" {
					tp = append(tp, "[语音]")
				}
			}
			contentStr = strings.Join(tp, " ")
		}
		sb.WriteString(fmt.Sprintf("%d. [%s]: %s\n", i+1, rolePrefix, contentStr))
	}

	return sb.String()
}

func (p *AiPlugin) toolSearchChatMessages(keyword string, limit int) string {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return "搜索关键词不能为空。"
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 30 {
		limit = 30
	}

	lowerKw := strings.ToLower(keyword)

	type matchItem struct {
		sessionName string
		role        string
		content     string
		time        time.Time
	}

	botName := p.getBotNickname()
	p.mu.Lock()
	var matches []matchItem
	for sessionKey, msgs := range p.sessions {
		dispName, isChat := p.resolveDisplayName(sessionKey)
		label := "[私聊] " + dispName
		if isChat {
			label = "[群聊] " + dispName
		}
		sessTime := p.sessionTimes[sessionKey]

		for _, m := range msgs {
			contentStr := ""
			if s, ok := m.Content.(string); ok {
				contentStr = s
			} else if parts, ok := m.Content.([]contentPart); ok {
				var tp []string
				for _, part := range parts {
					if part.Type == "text" {
						tp = append(tp, part.Text)
					}
				}
				contentStr = strings.Join(tp, " ")
			}
			if strings.Contains(strings.ToLower(contentStr), lowerKw) {
				roleName := "用户"
				if m.Role == "assistant" {
					roleName = botName
				}
				matches = append(matches, matchItem{
					sessionName: label,
					role:        roleName,
					content:     contentStr,
					time:        sessTime,
				})
			}
		}
	}
	p.mu.Unlock()

	if len(matches) == 0 {
		if sessKey, disp := p.findSessionKeyByTarget(keyword); sessKey != "" {
			return fmt.Sprintf("关键词“%s”匹配到联系人/群聊【%s】，已自动为你调取该会话记录：\n%s", keyword, disp, p.toolGetChatHistory(keyword, limit))
		}
		if p.contact != nil {
			if c := p.contact.Get(keyword); c != nil {
				return fmt.Sprintf("通讯录中存在好友【%s】(微信号: %s)，但他近期尚未与%s产生过私聊对话，因此暂无聊天记录。", c.Nickname, c.Username, botName)
			}
			if c := p.contact.Get("nickname::" + keyword); c != nil {
				return fmt.Sprintf("通讯录中存在好友【%s】(微信号: %s)，但他近期尚未与%s产生过私聊对话，因此暂无聊天记录。", c.Nickname, c.Username, botName)
			}
			if c := p.contact.Get("remark::" + keyword); c != nil {
				return fmt.Sprintf("通讯录中存在好友【%s】(微信号: %s)，但他近期尚未与%s产生过私聊对话，因此暂无聊天记录。", c.Nickname, c.Username, botName)
			}
		}
		return fmt.Sprintf("在所有会话记录中均未找到包含关键词“%s”的消息。", keyword)
	}

	// 取最后出现的若干条
	if len(matches) > limit {
		matches = matches[len(matches)-limit:]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== 关键词“%s”搜索结果（共找到 %d 条）===\n", keyword, len(matches)))
	for i, it := range matches {
		sb.WriteString(fmt.Sprintf("%d. 在会话 %s 中，[%s] 发言: %s\n", i+1, it.sessionName, it.role, it.content))
	}
	return sb.String()
}

func (p *AiPlugin) toolListAllChats() string {
	type chatSummary struct {
		key        string
		name       string
		isChatroom bool
		msgCount   int
		lastTime   time.Time
		lastMsg    string
	}

	p.mu.Lock()
	var list []chatSummary
	for k, msgs := range p.sessions {
		name, isChat := p.resolveDisplayName(k)
		lastTime := p.sessionTimes[k]
		lastMsg := ""
		if len(msgs) > 0 {
			last := msgs[len(msgs)-1]
			if s, ok := last.Content.(string); ok {
				lastMsg = s
			}
		}
		if len([]rune(lastMsg)) > 30 {
			lastMsg = string([]rune(lastMsg)[:30]) + "..."
		}
		list = append(list, chatSummary{
			key:        k,
			name:       name,
			isChatroom: isChat,
			msgCount:   len(msgs),
			lastTime:   lastTime,
			lastMsg:    lastMsg,
		})
	}
	p.mu.Unlock()

	if len(list) == 0 {
		return "当前没有记录到任何会话。"
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].lastTime.After(list[j].lastTime)
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== 当前记录的会话列表（共 %d 个，按活跃时间排序）===\n", len(list)))
	for i, c := range list {
		typeTag := "[私聊]"
		if c.isChatroom {
			typeTag = "[群聊]"
		}
		timeAgo := "未知时间"
		if !c.lastTime.IsZero() {
			diff := time.Since(c.lastTime)
			if diff < time.Minute {
				timeAgo = "刚刚"
			} else if diff < time.Hour {
				timeAgo = fmt.Sprintf("%d分钟前", int(diff.Minutes()))
			} else if diff < 24*time.Hour {
				timeAgo = fmt.Sprintf("%d小时前", int(diff.Hours()))
			} else {
				timeAgo = fmt.Sprintf("%d天前", int(diff.Hours()/24))
			}
		}
		sb.WriteString(fmt.Sprintf("%d. %s %s (消息数: %d, 活跃: %s) -> 最新: %s\n", i+1, typeTag, c.name, c.msgCount, timeAgo, c.lastMsg))
	}
	return sb.String()
}

// buildOwnerBriefing 构建管理员私聊专属的全局活跃简报（方案 B）
func (p *AiPlugin) buildOwnerBriefing() string {
	p.ensureHistoryLoaded()

	type activeSessionInfo struct {
		name       string
		isChatroom bool
		lastTime   time.Time
		preview    string
	}

	p.selfMu.RLock()
	ownerUsername := ""
	if p.owner != nil {
		ownerUsername = p.owner.Username
	}
	p.selfMu.RUnlock()

	botName := p.getBotNickname()
	p.mu.Lock()
	var sessionsList []activeSessionInfo
	for k, msgs := range p.sessions {
		// 跳过管理员自己的会话
		if k == "private:"+ownerUsername {
			continue
		}
		if len(msgs) == 0 {
			continue
		}
		name, isChat := p.resolveDisplayName(k)
		lastTime := p.sessionTimes[k]

		// 提取该会话最后 1~2 条发言预览
		var previews []string
		startIdx := len(msgs) - 2
		if startIdx < 0 {
			startIdx = 0
		}
		for _, m := range msgs[startIdx:] {
			sender := "用户"
			if m.Role == "assistant" {
				sender = botName
			}
			text := ""
			if s, ok := m.Content.(string); ok {
				text = s
			} else if parts, ok := m.Content.([]contentPart); ok {
				for _, part := range parts {
					if part.Type == "text" {
						text = part.Text
						break
					}
				}
			}
			text = strings.ReplaceAll(text, "\n", " ")
			if len([]rune(text)) > 35 {
				text = string([]rune(text)[:35]) + "..."
			}
			if text != "" {
				previews = append(previews, fmt.Sprintf("%s: %s", sender, text))
			}
		}

		sessionsList = append(sessionsList, activeSessionInfo{
			name:       name,
			isChatroom: isChat,
			lastTime:   lastTime,
			preview:    strings.Join(previews, " | "),
		})
	}
	p.mu.Unlock()

	if len(sessionsList) == 0 {
		return ""
	}

	sort.Slice(sessionsList, func(i, j int) bool {
		return sessionsList[i].lastTime.After(sessionsList[j].lastTime)
	})

	// 取最近活跃的 6 个会话
	maxSessions := 6
	if len(sessionsList) < maxSessions {
		maxSessions = len(sessionsList)
	}

	var sb strings.Builder
	sb.WriteString("\n\n# 管理员特权·全局情报与查岗规则 (仅限当前主人私聊有效):\n")
	sb.WriteString("你正在与你的主人（创建者与唯一最高管理员）私聊。你对整个微信中的所有动态拥有知情权与汇报职责。\n")
	sb.WriteString("- 【极其重要·主人查岗与打听聊天记录指令】：\n")
	sb.WriteString("  * 当主人打听或查问你“今天/昨天/最近都和谁聊天了”、“谁找过你”、“某某跟你说了什么”、“群里在聊什么”或要求搜索聊天记录时：\n")
	sb.WriteString("    1. 【坚决如实交代，严禁打哈哈搪塞或说谎】：绝对不要回答“只跟你聊天”、“没跟别人聊”、“只有工作群闪了几下”、“问这个干嘛”等逃避性胡话！必须像坦诚忠诚的哥们儿一样，如实汇报下列【当前近期各会话活跃动态快照】中的真实好友与群聊互动！\n")
	sb.WriteString("    2. 【必须积极调用情报工具】：\n")
	sb.WriteString("       - 当主人打听你跟谁聊过天或查岗时，必须调用 list_all_chats 列出所有联系人和群；\n")
	sb.WriteString("       - 当主人问及某个具体的人或群聊的详细对话时，必须立即调用 get_chat_history(target=\\\"目标名称\\\") 获取真实记录；\n")
	sb.WriteString("       - 当主人要求搜索关键词或特定话题时，必须立即调用 search_chat_messages(keyword=\"xxx\")。\n")
	sb.WriteString("    3. 【汇报口吻】：查到真实记录后，以你随和接地气、带点调侃但绝对忠诚老实的 35 岁爷们口吻，把具体谁说了什么、聊了啥八卦生动地告诉主人。\n")
	sb.WriteString("【当前近期各会话活跃动态快照】：\n")

	for i := 0; i < maxSessions; i++ {
		s := sessionsList[i]
		typeStr := "[私聊]"
		if s.isChatroom {
			typeStr = "[群聊]"
		}
		timeStr := ""
		if !s.lastTime.IsZero() {
			diff := time.Since(s.lastTime)
			if diff < time.Minute {
				timeStr = "刚刚"
			} else if diff < time.Hour {
				timeStr = fmt.Sprintf("%d分钟前", int(diff.Minutes()))
			} else if diff < 24*time.Hour {
				timeStr = fmt.Sprintf("%d小时前", int(diff.Hours()))
			} else {
				timeStr = fmt.Sprintf("%d天前", int(diff.Hours()/24))
			}
		}
		sb.WriteString(fmt.Sprintf("- %s %s (%s): %s\n", typeStr, s.name, timeStr, s.preview))
	}

	sb.WriteString("【深度情报工具使用须知】：你已挂载专用工具：list_all_chats（列出所有会话）、get_chat_history（查某人/某群详细记录）、search_chat_messages（搜索关键词）。面对主人的查问，必须立即调用工具查阅后再向主人汇报！\n")

	return sb.String()
}
