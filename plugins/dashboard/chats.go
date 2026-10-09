package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

var thoughtRegex = regexp.MustCompile(`(?s)<thought>.*?</thought>`)

// CleanThoughtTag 清理大模型生成的思维链标签
func CleanThoughtTag(s string) string {
	cleaned := thoughtRegex.ReplaceAllString(s, "")
	if idx := strings.Index(cleaned, "<thought>"); idx >= 0 {
		cleaned = cleaned[:idx]
	}
	return strings.TrimSpace(cleaned)
}

var (
	patTemplateRegex   = regexp.MustCompile(`(?s)<template><!\[CDATA\[(.*?)\]\]></template>`)
	patTemplateRegex2  = regexp.MustCompile(`(?s)<template>(.*?)</template>`)
	revokeMsgRegex     = regexp.MustCompile(`(?s)<replacemsg><!\[CDATA\[(.*?)\]\]></replacemsg>`)
	revokeMsgRegex2    = regexp.MustCompile(`(?s)<replacemsg>(.*?)</replacemsg>`)
	announcementRegex  = regexp.MustCompile(`(?s)<content><!\[CDATA\[(.*?)\]\]></content>`)
	announcementRegex2 = regexp.MustCompile(`(?s)<content>(.*?)</content>`)
	linkRegex          = regexp.MustCompile(`(?s)<link\s+name="([^"]+)"[^>]*>(.*?)</link>`)
	nicknameRegex      = regexp.MustCompile(`(?s)<nickname><!\[CDATA\[(.*?)\]\]></nickname>`)
	nicknameRegex2     = regexp.MustCompile(`(?s)<nickname>(.*?)</nickname>`)
	plainRegex         = regexp.MustCompile(`(?s)<plain><!\[CDATA\[(.*?)\]\]></plain>`)
	plainRegex2        = regexp.MustCompile(`(?s)<plain>(.*?)</plain>`)
	titleRegex         = regexp.MustCompile(`(?s)<title><!\[CDATA\[(.*?)\]\]></title>`)
	titleRegex2        = regexp.MustCompile(`(?s)<title>(.*?)</title>`)
	usernameRegex      = regexp.MustCompile(`(?s)<username><!\[CDATA\[(.*?)\]\]></username>`)
	usernameRegex2     = regexp.MustCompile(`(?s)<username>(.*?)</username>`)
	rePatQuotedVar     = regexp.MustCompile(`"\$\{([^}]+)\}"`)
	rePatBareVar       = regexp.MustCompile(`\$\{([^}]+)\}`)
	reAsciiQuote       = regexp.MustCompile(`"([^"]+)"`)
	reNewlines         = regexp.MustCompile(`[\r\n]+`)
)

// parseSystemNotice 解析拍一拍、撤回消息、进群通知、改名通知、群公告等系统事件
func parseSystemNotice(content, rawStr, chatroomID string, p *DashboardPlugin, botUsername string, memberNameMap map[string]string) string {
	text := strings.TrimSpace(content)
	raw := strings.TrimSpace(rawStr)

	// 1. 若 content 和 rawStr 均不含 <sysmsg，则可能是纯文本系统通知（如好友打招呼、移出群聊等）
	if !strings.Contains(text, "<sysmsg") && !strings.Contains(raw, "<sysmsg") {
		if text != "" && !strings.HasPrefix(text, "<msg>") {
			res := reAsciiQuote.ReplaceAllString(text, "“$1”")
			return res
		}
		return ""
	}

	xmlStr := text
	if !strings.Contains(xmlStr, "<sysmsg") {
		xmlStr = raw
	}
	if idx := strings.Index(xmlStr, "<sysmsg"); idx >= 0 {
		xmlStr = xmlStr[idx:]
	}

	resolveName := func(uid string) string {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			return ""
		}
		if botUsername != "" && uid == botUsername {
			return "你"
		}
		if n, ok := memberNameMap[uid]; ok && n != "" {
			return n
		}
		if p != nil && p.contact != nil {
			if c := p.contact.Get(uid); c != nil {
				if c.GetRemark() != "" {
					return c.GetRemark()
				}
				if c.GetNickname() != "" {
					return c.GetNickname()
				}
			}
		}
		return uid
	}

	// 2. 拍一拍消息
	if strings.Contains(xmlStr, `type="pat"`) {
		tmpl := ""
		if m := patTemplateRegex.FindStringSubmatch(xmlStr); len(m) > 1 {
			tmpl = m[1]
		} else if m2 := patTemplateRegex2.FindStringSubmatch(xmlStr); len(m2) > 1 {
			tmpl = m2[1]
		}
		if tmpl != "" {
			res := rePatQuotedVar.ReplaceAllStringFunc(tmpl, func(matched string) string {
				sub := rePatQuotedVar.FindStringSubmatch(matched)
				if len(sub) > 1 {
					return "“" + resolveName(sub[1]) + "”"
				}
				return matched
			})
			res = rePatBareVar.ReplaceAllStringFunc(res, func(matched string) string {
				sub := rePatBareVar.FindStringSubmatch(matched)
				if len(sub) > 1 {
					return resolveName(sub[1])
				}
				return matched
			})
			res = reNewlines.ReplaceAllString(res, " ")
			return strings.TrimSpace(res)
		}
		return ""
	}

	// 3. 撤回消息
	if strings.Contains(xmlStr, `type="revokemsg"`) {
		msg := ""
		if m := revokeMsgRegex.FindStringSubmatch(xmlStr); len(m) > 1 {
			msg = m[1]
		} else if m2 := revokeMsgRegex2.FindStringSubmatch(xmlStr); len(m2) > 1 {
			msg = m2[1]
		}
		if msg != "" {
			res := reAsciiQuote.ReplaceAllString(msg, "“$1”")
			return strings.TrimSpace(res)
		}
		return ""
	}

	// 4. 模板系统消息 (进群、修改群名、被移出群聊等)
	if strings.Contains(xmlStr, `type="sysmsgtemplate"`) {
		tmpl := ""
		if m := patTemplateRegex.FindStringSubmatch(xmlStr); len(m) > 1 {
			tmpl = m[1]
		} else if m2 := patTemplateRegex2.FindStringSubmatch(xmlStr); len(m2) > 1 {
			tmpl = m2[1]
		}
		if tmpl != "" {
			tmpl = reNewlines.ReplaceAllString(tmpl, " ")

			linkMatches := linkRegex.FindAllStringSubmatch(xmlStr, -1)
			varMap := make(map[string]string)
			for _, lm := range linkMatches {
				if len(lm) < 3 {
					continue
				}
				name := lm[1]
				body := lm[2]

				// 优先 nickname
				var nicks []string
				for _, nm := range nicknameRegex.FindAllStringSubmatch(body, -1) {
					if len(nm) > 1 && strings.TrimSpace(nm[1]) != "" {
						nicks = append(nicks, strings.TrimSpace(nm[1]))
					}
				}
				if len(nicks) == 0 {
					for _, nm := range nicknameRegex2.FindAllStringSubmatch(body, -1) {
						if len(nm) > 1 && strings.TrimSpace(nm[1]) != "" {
							nicks = append(nicks, strings.TrimSpace(nm[1]))
						}
					}
				}
				if len(nicks) > 0 {
					varMap[name] = strings.Join(nicks, "、")
					continue
				}

				// 其次 plain
				var plains []string
				for _, pm := range plainRegex.FindAllStringSubmatch(body, -1) {
					if len(pm) > 1 && strings.TrimSpace(pm[1]) != "" {
						plains = append(plains, strings.TrimSpace(pm[1]))
					}
				}
				if len(plains) == 0 {
					for _, pm := range plainRegex2.FindAllStringSubmatch(body, -1) {
						if len(pm) > 1 && strings.TrimSpace(pm[1]) != "" {
							plains = append(plains, strings.TrimSpace(pm[1]))
						}
					}
				}
				if len(plains) > 0 {
					varMap[name] = strings.Join(plains, "、")
					continue
				}

				// 其次 title
				if tm := titleRegex.FindStringSubmatch(body); len(tm) > 1 && strings.TrimSpace(tm[1]) != "" {
					varMap[name] = strings.TrimSpace(tm[1])
					continue
				} else if tm2 := titleRegex2.FindStringSubmatch(body); len(tm2) > 1 && strings.TrimSpace(tm2[1]) != "" {
					varMap[name] = strings.TrimSpace(tm2[1])
					continue
				}

				// 其次 username (通过 resolveName 换成群昵称/微信昵称)
				var unames []string
				for _, um := range usernameRegex.FindAllStringSubmatch(body, -1) {
					if len(um) > 1 && strings.TrimSpace(um[1]) != "" {
						unames = append(unames, resolveName(strings.TrimSpace(um[1])))
					}
				}
				if len(unames) == 0 {
					for _, um := range usernameRegex2.FindAllStringSubmatch(body, -1) {
						if len(um) > 1 && strings.TrimSpace(um[1]) != "" {
							unames = append(unames, resolveName(strings.TrimSpace(um[1])))
						}
					}
				}
				if len(unames) > 0 {
					varMap[name] = strings.Join(unames, "、")
					continue
				}
			}

			// 替换 "$var$" 或 “$var$” 为 “val”
			for k, v := range varMap {
				tmpl = strings.ReplaceAll(tmpl, `"$`+k+`$"`, "“"+v+"”")
				tmpl = strings.ReplaceAll(tmpl, `“$`+k+`$”`, "“"+v+"”")
				tmpl = strings.ReplaceAll(tmpl, `$`+k+`$`, v)
			}
			return strings.TrimSpace(tmpl)
		}
		return ""
	}

	// 5. 群公告
	if strings.Contains(xmlStr, `type="mmchatroombarannouncememt"`) {
		if m := announcementRegex.FindStringSubmatch(xmlStr); len(m) > 1 && strings.TrimSpace(m[1]) != "" {
			return "群公告：" + strings.TrimSpace(m[1])
		} else if m2 := announcementRegex2.FindStringSubmatch(xmlStr); len(m2) > 1 && strings.TrimSpace(m2[1]) != "" {
			return "群公告：" + strings.TrimSpace(m2[1])
		}
		return ""
	}

	return ""
}

func parseMsgTime(ts string) time.Time {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return time.Time{}
	}
	// 1. 若含有末尾 Z（sqlite 驱动由于 DATETIME 列类型自动附加的伪 UTC 标识），去除后按本地时间解析
	// 注：系统与 sqlite 库均存本地时间 (localtime)，驱动附加的 Z 会导致 8 小时时区偏移
	if strings.HasSuffix(ts, "Z") {
		clean := strings.TrimSuffix(ts, "Z")
		clean = strings.Replace(clean, "T", " ", 1)
		if t, err := time.ParseInLocation("2006-01-02 15:04:05.999999999", clean, time.Local); err == nil {
			return t
		}
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", clean, time.Local); err == nil {
			return t
		}
	}
	// 2. 带时区偏移的 RFC3339 / RFC3339Nano (如 2026-09-23T15:51:57+08:00)
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.In(time.Local)
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.In(time.Local)
	}
	// 3. 本地时间字符串 (2006-01-02 15:04:05)
	if t, err := time.ParseInLocation("2006-01-02 15:04:05.999999999", ts, time.Local); err == nil {
		return t
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", ts, time.Local); err == nil {
		return t
	}
	if t, err := time.ParseInLocation("2006-01-02", ts, time.Local); err == nil {
		return t
	}
	return time.Time{}
}


type AIHistoryMsg struct {
	Role     string `json:"role"`
	Content  any    `json:"content"` // 允许 string 或 []any 多模态数组
	MediaURL string `json:"media_url,omitempty"`
}

type AIHistorySession struct {
	UpdatedAt     string         `json:"updated_at"`
	ContextCutoff int            `json:"context_cutoff,omitempty"`
	Messages      []AIHistoryMsg `json:"messages"`
}

type AIHistoryFile struct {
	Version  int                         `json:"version"`
	Sessions map[string]AIHistorySession `json:"sessions"`
}

func extractContent(raw any) (text string, msgType string, mediaURL string, duration int) {
	if raw == nil {
		return "", "text", "", 0
	}
	if s, ok := raw.(string); ok {
		msgType = "text"
		if strings.Contains(s, "[语音]") || strings.Contains(s, "发来了一条语音消息") {
			msgType = "voice"
		} else if strings.Contains(s, "[图片]") || strings.Contains(s, "发送了一张图片") ||
			strings.Contains(s, "生活自拍照") || strings.Contains(s, "绘制的图片") {
			msgType = "image"
		}
		return s, msgType, "", 0
	}
	if list, ok := raw.([]any); ok {
		var textParts []string
		msgType = "text"
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				t, _ := m["type"].(string)
				switch t {
				case "text":
					if txt, ok := m["text"].(string); ok {
						textParts = append(textParts, txt)
					}
				case "image_url":
					msgType = "image"
					if imgObj, ok := m["image_url"].(map[string]any); ok {
						if u, ok := imgObj["url"].(string); ok && u != "" {
							mediaURL = u
						}
					}
				case "input_audio":
					msgType = "voice"
					if audioObj, ok := m["input_audio"].(map[string]any); ok {
						dataStr, _ := audioObj["data"].(string)
						fmtStr, _ := audioObj["format"].(string)
						if fmtStr == "" {
							fmtStr = "wav"
						}
						if dataStr != "" {
							if strings.HasPrefix(dataStr, "data:") {
								mediaURL = dataStr
							} else {
								mediaURL = fmt.Sprintf("data:audio/%s;base64,%s", fmtStr, dataStr)
							}
						}
					}
				}
			}
		}
		txt := strings.Join(textParts, "\n")
		if txt == "" {
			if msgType == "image" {
				txt = "[图片]"
			} else if msgType == "voice" {
				txt = "[语音]"
			}
		}
		return txt, msgType, mediaURL, duration
	}
	return fmt.Sprintf("%v", raw), "text", "", 0
}

func extractContentString(raw any) (string, string) {
	text, msgType, _, _ := extractContent(raw)
	return text, msgType
}

type SentRecord struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Type      string    `json:"type,omitempty"`
	Content   string    `json:"content"`
	MediaURL  string    `json:"media_url,omitempty"`
	Duration  int       `json:"duration,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type ChatEngine struct {
	mu               sync.Mutex
	aiHistoryPath    string
	statisticsDBPath string
	sentStorePath    string
	botUsername      string
	botNickname      string
	sentMessages     map[string][]SentRecord // sessionID -> records
}

func (ce *ChatEngine) SetBotInfo(username, nickname string) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	if username != "" {
		ce.botUsername = username
	}
	if nickname != "" {
		ce.botNickname = nickname
	}
}

func NewChatEngine(aiHistoryPath, statisticsDBPath, sentStorePath string) *ChatEngine {
	ce := &ChatEngine{
		aiHistoryPath:    aiHistoryPath,
		statisticsDBPath: statisticsDBPath,
		sentStorePath:    sentStorePath,
		sentMessages:     make(map[string][]SentRecord),
	}
	ce.loadSentStore()
	return ce
}

func (ce *ChatEngine) loadSentStore() {
	if ce.sentStorePath == "" {
		return
	}
	data, err := os.ReadFile(ce.sentStorePath)
	if err != nil {
		return
	}
	var stored map[string][]SentRecord
	if err := json.Unmarshal(data, &stored); err == nil && stored != nil {
		ce.sentMessages = stored
	}
}

// loadPluginSentRecords 动态读取所有插件通过 SDK 发送的出站消息记录 (data/plugin_sent.jsonl)
func (ce *ChatEngine) loadPluginSentRecords(sessionID string) []SentRecord {
	cleanID := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(sessionID, "chatroom:"), "private:"), "contact:")
	var records []SentRecord

	candPaths := []string{
		filepath.Join("data", "plugin_sent.jsonl"),
		"plugin_sent.jsonl",
		filepath.Join("..", "..", "data", "plugin_sent.jsonl"),
	}

	for _, pth := range candPaths {
		f, err := os.Open(pth)
		if err != nil {
			continue
		}
		defer f.Close()

		decoder := json.NewDecoder(f)
		for decoder.More() {
			var item struct {
				ID        int64  `json:"id"`
				SessionID string `json:"session_id"`
				Type      string `json:"type"`
				Content   string `json:"content"`
				Timestamp string `json:"timestamp"`
			}
			if err := decoder.Decode(&item); err == nil {
				cID := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(item.SessionID, "chatroom:"), "private:"), "contact:")
				if cID == cleanID {
					pt := parseMsgTime(item.Timestamp)
					if pt.IsZero() {
						pt = time.Now()
					}
					st := item.Type
					if st == "" {
						st = "text"
					}
					records = append(records, SentRecord{
						ID:        item.ID,
						SessionID: cleanID,
						Role:      "assistant",
						Type:      st,
						Content:   item.Content,
						Timestamp: pt,
					})
				}
			}
		}
		break
	}
	return records
}

func (ce *ChatEngine) saveSentStoreLocked() {
	if ce.sentStorePath == "" {
		return
	}
	data, err := json.MarshalIndent(ce.sentMessages, "", "  ")
	if err == nil {
		_ = os.WriteFile(ce.sentStorePath, data, 0644)
	}
}

// RecordSentMediaMessage 记录管理员代发的富媒体消息（文本、图片、语音）
func (ce *ChatEngine) RecordSentMediaMessage(targetID, msgType, content, mediaURL string, duration int, botUsername string) error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	now := time.Now()
	cleanID := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(targetID, "chatroom:"), "private:"), "contact:")

	record := SentRecord{
		ID:        now.UnixNano(),
		SessionID: cleanID,
		Role:      "assistant",
		Type:      msgType,
		Content:   content,
		MediaURL:  mediaURL,
		Duration:  duration,
		Timestamp: now,
	}
	ce.sentMessages[cleanID] = append(ce.sentMessages[cleanID], record)
	ce.saveSentStoreLocked()

	// 插入 statistics.db
	if ce.statisticsDBPath != "" {
		if db, err := sql.Open("sqlite", ce.statisticsDBPath); err == nil {
			defer db.Close()
			sender := botUsername
			dbType := "文本"
			if msgType == "image" {
				dbType = "图片"
			} else if msgType == "voice" {
				dbType = "语音"
			}
			_, _ = db.Exec(`
				INSERT INTO statistics (type, content, sender, receiver, member, raw, timestamp)
				VALUES (?, ?, ?, ?, '', '', datetime('now', 'localtime'))
			`, dbType, content, sender, cleanID)
		}
	}

	// 追加到 ai_history.json
	if ce.aiHistoryPath != "" {
		var hist AIHistoryFile
		data, err := os.ReadFile(ce.aiHistoryPath)
		if err == nil && json.Unmarshal(data, &hist) == nil && hist.Sessions != nil {
			foundKey, ok := ce.findSessionInHistory(&hist, cleanID)
			if ok {
				sess := hist.Sessions[foundKey]
				sess.UpdatedAt = now.Format(time.RFC3339Nano)
				var aiContent any = content
				if msgType == "image" && mediaURL != "" {
					aiContent = []any{
						map[string]any{"type": "text", "text": content},
						map[string]any{"type": "image_url", "image_url": map[string]any{"url": mediaURL}},
					}
				}
				sess.Messages = append(sess.Messages, AIHistoryMsg{
					Role:    "assistant",
					Content: aiContent,
				})
				hist.Sessions[foundKey] = sess
				if newData, err := json.MarshalIndent(hist, "", "  "); err == nil {
					_ = os.WriteFile(ce.aiHistoryPath, newData, 0644)
				}
			}
		}
	}

	return nil
}

// RecordSentMessage 记录纯文本代发消息
func (ce *ChatEngine) RecordSentMessage(targetID, content, botUsername string) error {
	return ce.RecordSentMediaMessage(targetID, "text", content, "", 0, botUsername)
}

// AppendSentMessage 兼容旧接口
func (ce *ChatEngine) AppendSentMessage(targetID string, content string) error {
	return ce.RecordSentMessage(targetID, content, "")
}

// findSessionInHistory 在 ai_history.json 中智能定位会话 Key
func (ce *ChatEngine) findSessionInHistory(hist *AIHistoryFile, sessionID string) (string, bool) {
	if hist == nil || hist.Sessions == nil {
		return "", false
	}
	candidates := []string{
		sessionID,
		"private:" + sessionID,
		"chatroom:" + sessionID,
		"contact:" + sessionID,
	}
	for _, c := range candidates {
		if _, ok := hist.Sessions[c]; ok {
			return c, true
		}
	}
	for k := range hist.Sessions {
		clean := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(k, "chatroom:"), "private:"), "contact:")
		if clean == sessionID {
			return k, true
		}
	}
	return "", false
}

// GetSessions 获取所有聊天会话列表
func (ce *ChatEngine) GetSessions(p *DashboardPlugin) ([]ChatSessionItem, error) {
	sessionMap := make(map[string]*ChatSessionItem)

	botUsername := ""
	if p != nil {
		botUsername = p.getBotUsername()
	}

	// 1. 从 contact 能力填充基础会话（好友与群聊，排除自身 bot）
	if p != nil && p.contact != nil {
		contacts := p.contact.List()
		for _, c := range contacts {
			if c == nil || c.GetUsername() == "" {
				continue
			}
			sID := c.GetUsername()
			if botUsername != "" && sID == botUsername {
				continue
			}
			sName := c.GetRemark()
			if sName == "" {
				sName = c.GetNickname()
			}
			if sName == "" {
				sName = sID
			}

			sType := "friend"
			if strings.HasSuffix(sID, "@chatroom") || c.GetType().String() == "CONTACT_TYPE_CHATROOM" {
				sType = "chatroom"
			} else if c.GetType().String() == "CONTACT_TYPE_OFFICIAL" {
				sType = "official"
			}

			avatarURL := c.GetAvatar()
			if avatarURL == "" && sType == "chatroom" && p.chatroom != nil {
				if info, err := p.chatroom.GetInfo(sID); err == nil && info != nil && info.GetAvatar() != "" {
					avatarURL = info.GetAvatar()
				}
			}

			sessionMap[sID] = &ChatSessionItem{
				ID:        sID,
				Name:      sName,
				Type:      sType,
				AvatarURL: avatarURL,
			}
		}
	}

	// 2. 读取 statistics.db 真实发言与统计（过滤系统协议和状态通知，正确归属对话会话）
	if ce.statisticsDBPath != "" {
		if _, err := os.Stat(ce.statisticsDBPath); err == nil {
			db, err := sql.Open("sqlite", ce.statisticsDBPath)
			if err == nil {
				defer db.Close()
				q := `
					WITH ranked AS (
						SELECT 
							id,
							CASE 
								WHEN sender LIKE '%@chatroom' THEN sender 
								WHEN receiver LIKE '%@chatroom' THEN receiver 
								WHEN sender = ? THEN receiver 
								ELSE sender 
							END as session_id,
							content,
							CAST(timestamp AS TEXT) as timestamp,
							ROW_NUMBER() OVER (
								PARTITION BY 
									CASE 
										WHEN sender LIKE '%@chatroom' THEN sender 
										WHEN receiver LIKE '%@chatroom' THEN receiver 
										WHEN sender = ? THEN receiver 
										ELSE sender 
									END 
								ORDER BY id DESC
							) as rn,
							COUNT(1) OVER (
								PARTITION BY 
									CASE 
										WHEN sender LIKE '%@chatroom' THEN sender 
										WHEN receiver LIKE '%@chatroom' THEN receiver 
										WHEN sender = ? THEN receiver 
										ELSE sender 
									END
							) as total_cnt
						FROM statistics 
						WHERE type NOT IN ('状态通知', '系统提示', '系统') 
						  AND content NOT LIKE '<msg><op%' 
						  AND content NOT LIKE '<sysmsg%'
					)
					SELECT session_id, content, timestamp, total_cnt
					FROM ranked 
					WHERE rn = 1
				`
				rows, err := db.Query(q, botUsername, botUsername, botUsername)
				if err == nil {
					defer rows.Close()
					for rows.Next() {
						var targetKey, content, ts string
						var cnt int
						if err := rows.Scan(&targetKey, &content, &ts, &cnt); err == nil {
							if targetKey == "" || (botUsername != "" && targetKey == botUsername) {
								continue
							}
							if pt := parseMsgTime(ts); !pt.IsZero() {
								ts = pt.In(time.Local).Format(time.RFC3339)
							}
							if s, ok := sessionMap[targetKey]; ok {
								s.LastMessage = content
								s.LastTime = ts
								s.MsgCount = cnt
							} else {
								sType := "friend"
								if strings.HasSuffix(targetKey, "@chatroom") {
									sType = "chatroom"
								}
								avatarURL := ""
								name := targetKey
								if p != nil && p.contact != nil {
									if c := p.contact.Get(targetKey); c != nil {
										avatarURL = c.GetAvatar()
										if c.GetRemark() != "" {
											name = c.GetRemark()
										} else if c.GetNickname() != "" {
											name = c.GetNickname()
										}
									}
								}
								if avatarURL == "" && sType == "chatroom" && p != nil && p.chatroom != nil {
									if info, err := p.chatroom.GetInfo(targetKey); err == nil && info != nil {
										avatarURL = info.GetAvatar()
										if info.GetNickname() != "" {
											name = info.GetNickname()
										}
									}
								}
								sessionMap[targetKey] = &ChatSessionItem{
									ID:          targetKey,
									Name:        name,
									Type:        sType,
									AvatarURL:   avatarURL,
									LastMessage: content,
									LastTime:    ts,
									MsgCount:    cnt,
								}
							}
						}
					}
				}
			}
		}
	}

	// 3. 从 ai_history.json 补充 AI 核心对话记录（优先展示真实对话）
	if ce.aiHistoryPath != "" {
		if content, err := os.ReadFile(ce.aiHistoryPath); err == nil {
			var hist AIHistoryFile
			if err := json.Unmarshal(content, &hist); err == nil {
				for sessKey, sess := range hist.Sessions {
					cleanKey := strings.TrimPrefix(sessKey, "chatroom:")
					cleanKey = strings.TrimPrefix(cleanKey, "private:")
					cleanKey = strings.TrimPrefix(cleanKey, "contact:")

					if botUsername != "" && cleanKey == botUsername {
						continue
					}

					s, ok := sessionMap[cleanKey]
					if !ok {
						s = &ChatSessionItem{
							ID:   cleanKey,
							Name: cleanKey,
							Type: "friend",
						}
						if strings.HasSuffix(cleanKey, "@chatroom") {
							s.Type = "chatroom"
						}
						sessionMap[cleanKey] = s
					}

					if len(sess.Messages) > 0 {
						last := sess.Messages[len(sess.Messages)-1]
						rawStr, _ := extractContentString(last.Content)
						cleanMsg := CleanThoughtTag(rawStr)
						if cleanMsg == "" && len(sess.Messages) > 1 {
							secondLast, _ := extractContentString(sess.Messages[len(sess.Messages)-2].Content)
							cleanMsg = CleanThoughtTag(secondLast)
						}
						s.LastMessage = cleanMsg
						s.LastTime = sess.UpdatedAt
						if s.MsgCount < len(sess.Messages) {
							s.MsgCount = len(sess.Messages)
						}
					}
				}
			}
		}
	}

	// 4. 从本地代发记录中更新最新消息摘要
	for cleanKey, sentList := range ce.sentMessages {
		if len(sentList) == 0 {
			continue
		}
		if botUsername != "" && cleanKey == botUsername {
			continue
		}
		lastSent := sentList[len(sentList)-1]
		s, ok := sessionMap[cleanKey]
		if !ok {
			s = &ChatSessionItem{
				ID:   cleanKey,
				Name: cleanKey,
				Type: "friend",
			}
			if strings.HasSuffix(cleanKey, "@chatroom") {
				s.Type = "chatroom"
			}
			sessionMap[cleanKey] = s
		}
		lastSentTime := lastSent.Timestamp.Format(time.RFC3339)
		if s.LastTime == "" || lastSentTime > s.LastTime {
			s.LastMessage = lastSent.Content
			s.LastTime = lastSentTime
		}
	}

	// 5. 补全所有会话的昵称、群名与头像（支持 chatroom.json / contact.json / 专属策略覆盖）
	var fileChatrooms map[string]map[string]struct {
		Username string `json:"username"`
		Nickname string `json:"nickname"`
		Remark   string `json:"remark"`
		Avatar   string `json:"avatar"`
	}
	var fileContacts map[string]struct {
		Username string `json:"username"`
		Nickname string `json:"nickname"`
		Remark   string `json:"remark"`
		Avatar   string `json:"avatar"`
	}

	loadChatroomFile := func() {
		if fileChatrooms != nil {
			return
		}
		fileChatrooms = make(map[string]map[string]struct {
			Username string `json:"username"`
			Nickname string `json:"nickname"`
			Remark   string `json:"remark"`
			Avatar   string `json:"avatar"`
		})
		for _, pth := range []string{filepath.Join("data", "chatroom.json"), "chatroom.json"} {
			if b, err := os.ReadFile(pth); err == nil {
				_ = json.Unmarshal(b, &fileChatrooms)
				if len(fileChatrooms) > 0 {
					break
				}
			}
		}
	}

	loadContactFile := func() {
		if fileContacts != nil {
			return
		}
		fileContacts = make(map[string]struct {
			Username string `json:"username"`
			Nickname string `json:"nickname"`
			Remark   string `json:"remark"`
			Avatar   string `json:"avatar"`
		})
		for _, pth := range []string{filepath.Join("data", "contact.json"), "contact.json"} {
			if b, err := os.ReadFile(pth); err == nil {
				_ = json.Unmarshal(b, &fileContacts)
				if len(fileContacts) > 0 {
					break
				}
			}
		}
	}

	for _, s := range sessionMap {
		cleanID := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(s.ID, "chatroom:"), "private:"), "contact:")
		if s.Type == "chatroom" || strings.HasSuffix(cleanID, "@chatroom") {
			s.Type = "chatroom"

			// 优先从群聊能力与联系人能力中补充
			if p != nil && p.chatroom != nil {
				if info, err := p.chatroom.GetInfo(cleanID); err == nil && info != nil {
					if s.AvatarURL == "" && info.GetAvatar() != "" {
						s.AvatarURL = info.GetAvatar()
					}
					if (s.Name == "" || s.Name == s.ID) && info.GetNickname() != "" {
						s.Name = info.GetNickname()
					}
				}
			}
			if p != nil && p.contact != nil {
				if c := p.contact.Get(cleanID); c != nil {
					if s.AvatarURL == "" && c.GetAvatar() != "" {
						s.AvatarURL = c.GetAvatar()
					}
					if (s.Name == "" || s.Name == s.ID) {
						if c.GetRemark() != "" {
							s.Name = c.GetRemark()
						} else if c.GetNickname() != "" {
							s.Name = c.GetNickname()
						}
					}
				}
			}

			// 从本地文件兜底补充群名或群头像
			if s.AvatarURL == "" || s.Name == "" || s.Name == s.ID {
				loadContactFile()
				if fc, ok := fileContacts[cleanID]; ok {
					if s.AvatarURL == "" && fc.Avatar != "" {
						s.AvatarURL = fc.Avatar
					}
					if (s.Name == "" || s.Name == s.ID) {
						if fc.Remark != "" {
							s.Name = fc.Remark
						} else if fc.Nickname != "" {
							s.Name = fc.Nickname
						}
					}
				}
			}

			// 如果群名依然是群 ID，尝试从群成员列表中组合前三位成员的名字作为默认群名
			if s.Name == "" || s.Name == s.ID {
				loadChatroomFile()
				if members, ok := fileChatrooms[cleanID]; ok && len(members) > 0 {
					var memberNames []string
					for _, m := range members {
						n := m.Remark
						if n == "" {
							n = m.Nickname
						}
						if n == "" {
							n = m.Username
						}
						if n != "" {
							memberNames = append(memberNames, n)
						}
						if len(memberNames) >= 3 {
							break
						}
					}
					if len(memberNames) > 0 {
						s.Name = strings.Join(memberNames, "、")
					}
				}
			}
		} else {
			// 单聊 / 好友
			if p != nil && p.contact != nil {
				if c := p.contact.Get(cleanID); c != nil {
					if s.AvatarURL == "" && c.GetAvatar() != "" {
						s.AvatarURL = c.GetAvatar()
					}
					if (s.Name == "" || s.Name == s.ID) {
						if c.GetRemark() != "" {
							s.Name = c.GetRemark()
						} else if c.GetNickname() != "" {
							s.Name = c.GetNickname()
						}
					}
				}
			}
			if s.AvatarURL == "" || s.Name == "" || s.Name == s.ID {
				loadContactFile()
				if fc, ok := fileContacts[cleanID]; ok {
					if s.AvatarURL == "" && fc.Avatar != "" {
						s.AvatarURL = fc.Avatar
					}
					if (s.Name == "" || s.Name == s.ID) {
						if fc.Remark != "" {
							s.Name = fc.Remark
						} else if fc.Nickname != "" {
							s.Name = fc.Nickname
						}
					}
				}
			}
		}

		// 检查是否有配置覆盖的专属名称
		if p != nil && p.store != nil {
			if ov, ok := p.store.GetOverride(cleanID); ok && ov != nil {
				if ov.Name != "" && (s.Name == "" || s.Name == s.ID) {
					s.Name = ov.Name
				}
			}
		}

		// 确保会话最后一条消息中的发言人前缀使用微信真实昵称，而非给 AI 称呼用的定制称呼
		if colon := strings.Index(s.LastMessage, ": "); colon > 0 && !strings.Contains(s.LastMessage[:colon], "\n") && utf8.RuneCountInString(s.LastMessage[:colon]) <= 40 {
			prefix := s.LastMessage[:colon]
			var allOv map[string]*TargetOverride
			if p != nil && p.store != nil {
				allOv = p.store.GetOverrides()
			}
			for _, ov := range allOv {
				if ov != nil && ov.CustomNickname != "" && ov.CustomNickname == prefix {
					realPrefix := ov.Remark
					if realPrefix == "" {
						realPrefix = ov.Name
					}
					if realPrefix != "" {
						s.LastMessage = realPrefix + s.LastMessage[colon:]
					}
					break
				}
			}
		}
	}

	// 转为列表并按最近发言时间倒序排序
	result := make([]ChatSessionItem, 0, len(sessionMap))
	for _, item := range sessionMap {
		result = append(result, *item)
	}

	sort.Slice(result, func(i, j int) bool {
		ti := parseMsgTime(result[i].LastTime)
		tj := parseMsgTime(result[j].LastTime)
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return result[i].MsgCount > result[j].MsgCount
	})

	return result, nil
}

// GetMessages 获取单个会话的聊天消息列表（优先读取完整的 AI 上下文对话）
// 返回值: 消息切片, 消息总数, 上下文截断点(contextCutoff), 截断点是否在所有历史消息末尾(cutoffAtEnd), 错误
func (ce *ChatEngine) GetMessages(p *DashboardPlugin, sessionID string, limit, offset int) ([]ChatMessageItem, int, int, bool, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	cleanID := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(sessionID, "chatroom:"), "private:"), "contact:")
	isChatroom := strings.HasSuffix(cleanID, "@chatroom")

	botUsername := ""
	botNickname := "Bot"
	if p != nil {
		botUsername = p.getBotUsername()
		botNickname = p.getBotNickname()
	} else if ce.botNickname != "" {
		botNickname = ce.botNickname
		botUsername = ce.botUsername
	}
	if botUsername != "" && cleanID == botUsername {
		return []ChatMessageItem{}, 0, 0, false, nil
	}

	memberAvatarMap := make(map[string]string)
	memberNameMap := make(map[string]string)

	// 本地 contact.json 缓存读取
	var fileContacts map[string]struct {
		Username string `json:"username"`
		Nickname string `json:"nickname"`
		Remark   string `json:"remark"`
		Avatar   string `json:"avatar"`
	}
	for _, pth := range []string{filepath.Join("data", "contact.json"), "contact.json"} {
		if b, err := os.ReadFile(pth); err == nil {
			if err := json.Unmarshal(b, &fileContacts); err == nil && len(fileContacts) > 0 {
				break
			}
		}
	}

	if isChatroom {
		if p != nil && p.chatroom != nil {
			for _, m := range p.chatroom.ListMembers(cleanID) {
				if m == nil {
					continue
				}
				name := m.GetDisplayName()
				if name == "" {
					name = m.GetRemark()
				}
				if name == "" {
					name = m.GetNickname()
				}
				if name != "" && m.GetUsername() != "" {
					memberNameMap[m.GetUsername()] = name
				}
				av := m.GetAvatar()
				if av == "" && p.contact != nil {
					if c := p.contact.Get(m.GetUsername()); c != nil {
						av = c.GetAvatar()
					}
				}
				if av == "" && fileContacts != nil {
					if fc, ok := fileContacts[m.GetUsername()]; ok && fc.Avatar != "" {
						av = fc.Avatar
					}
				}
				if av != "" {
					if m.GetUsername() != "" {
						memberAvatarMap[m.GetUsername()] = av
					}
					if m.GetDisplayName() != "" {
						memberAvatarMap[m.GetDisplayName()] = av
					}
					if m.GetNickname() != "" {
						memberAvatarMap[m.GetNickname()] = av
					}
					if m.GetRemark() != "" {
						memberAvatarMap[m.GetRemark()] = av
					}
				}
			}
		}

		// 从本地 data/chatroom.json 兜底补充群成员头像（无论群聊接口返回多少人，均尝试查漏补缺）
		var fileChatrooms map[string]map[string]struct {
			Username    string `json:"username"`
			Nickname    string `json:"nickname"`
			Remark      string `json:"remark"`
			DisplayName string `json:"display_name"`
			Avatar      string `json:"avatar"`
		}
		for _, pth := range []string{filepath.Join("data", "chatroom.json"), "chatroom.json"} {
			if b, err := os.ReadFile(pth); err == nil {
				if err := json.Unmarshal(b, &fileChatrooms); err == nil && len(fileChatrooms) > 0 {
					if crMembers, ok := fileChatrooms[cleanID]; ok {
						for _, m := range crMembers {
							name := m.DisplayName
							if name == "" {
								name = m.Remark
							}
							if name == "" {
								name = m.Nickname
							}
							if name != "" && m.Username != "" && memberNameMap[m.Username] == "" {
								memberNameMap[m.Username] = name
							}
							av := m.Avatar
							if av == "" && fileContacts != nil {
								if fc, ok := fileContacts[m.Username]; ok && fc.Avatar != "" {
									av = fc.Avatar
								}
							}
							if av != "" {
								if m.Username != "" && memberAvatarMap[m.Username] == "" {
									memberAvatarMap[m.Username] = av
								}
								if m.DisplayName != "" && memberAvatarMap[m.DisplayName] == "" {
									memberAvatarMap[m.DisplayName] = av
								}
								if m.Nickname != "" && memberAvatarMap[m.Nickname] == "" {
									memberAvatarMap[m.Nickname] = av
								}
								if m.Remark != "" && memberAvatarMap[m.Remark] == "" {
									memberAvatarMap[m.Remark] = av
								}
							}
						}
					}
					break
				}
			}
		}
	}

	if fileContacts != nil {
		for u, fc := range fileContacts {
			if _, ok := memberNameMap[u]; !ok {
				name := fc.Remark
				if name == "" {
					name = fc.Nickname
				}
				if name != "" {
					memberNameMap[u] = name
				}
			}
		}
	}
	if botUsername != "" {
		memberNameMap[botUsername] = "你"
	}

	// 载入专属配置，将设置了专属定制称呼(custom_nickname)或专属名称(name)的成员头像关联到对应称呼
	var overrides map[string]*TargetOverride
	if p != nil && p.store != nil {
		overrides = p.store.GetOverrides()
	} else {
		for _, pth := range []string{filepath.Join("data", "dashboard.json"), "dashboard.json"} {
			if b, err := os.ReadFile(pth); err == nil {
				var dStore DashboardStore
				if err := json.Unmarshal(b, &dStore); err == nil && len(dStore.Overrides) > 0 {
					overrides = dStore.Overrides
					break
				}
			}
		}
	}

	for _, ov := range overrides {
		if ov == nil {
			continue
		}
		cleanTarget := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(ov.TargetID, "chatroom:"), "contact:"), "private:")
		av := ""
		if a, ok := memberAvatarMap[cleanTarget]; ok && a != "" {
			av = a
		} else if a, ok := memberAvatarMap[ov.TargetID]; ok && a != "" {
			av = a
		}
		if av == "" && p != nil && p.contact != nil {
			if c := p.contact.Get(cleanTarget); c != nil && c.GetAvatar() != "" {
				av = c.GetAvatar()
			} else if c := p.contact.Get(ov.TargetID); c != nil && c.GetAvatar() != "" {
				av = c.GetAvatar()
			}
		}
		if av == "" && fileContacts != nil {
			if fc, ok := fileContacts[cleanTarget]; ok && fc.Avatar != "" {
				av = fc.Avatar
			} else if fc, ok := fileContacts[ov.TargetID]; ok && fc.Avatar != "" {
				av = fc.Avatar
			}
		}
		if av != "" {
			memberAvatarMap[cleanTarget] = av
			memberAvatarMap[ov.TargetID] = av
			if ov.CustomNickname != "" {
				memberAvatarMap[ov.CustomNickname] = av
			}
			if ov.Name != "" {
				memberAvatarMap[ov.Name] = av
			}
			if ov.Remark != "" {
				memberAvatarMap[ov.Remark] = av
			}
		}
	}

	resolveAvatar := func(senderID, senderName string, isSelf bool) string {
		if isSelf {
			if p != nil {
				return p.getBotAvatar()
			}
			return ""
		}
		if isChatroom {
			if senderID != "" {
				if av, ok := memberAvatarMap[senderID]; ok && av != "" {
					return av
				}
			}
			if senderName != "" {
				if av, ok := memberAvatarMap[senderName]; ok && av != "" {
					return av
				}
			}
		}

		// 检查通过 overrides 寻找对应联系人的头像
		for _, ov := range overrides {
			if ov == nil {
				continue
			}
			matched := false
			if senderName != "" && (ov.CustomNickname == senderName || ov.Name == senderName || ov.Remark == senderName || ov.TargetID == senderName) {
				matched = true
			}
			if senderID != "" && (ov.TargetID == senderID || ov.CustomNickname == senderID || ov.Name == senderID || ov.Remark == senderID) {
				matched = true
			}
			if matched {
				cleanTarget := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(ov.TargetID, "chatroom:"), "contact:"), "private:")
				if av, ok := memberAvatarMap[cleanTarget]; ok && av != "" {
					return av
				}
				if p != nil && p.contact != nil {
					if c := p.contact.Get(cleanTarget); c != nil && c.GetAvatar() != "" {
						return c.GetAvatar()
					}
				}
				if fileContacts != nil {
					if fc, ok := fileContacts[cleanTarget]; ok && fc.Avatar != "" {
						return fc.Avatar
					}
				}
			}
		}

		// 检查联系人插件
		if p != nil && p.contact != nil {
			if senderID != "" {
				if c := p.contact.Get(senderID); c != nil && c.GetAvatar() != "" {
					return c.GetAvatar()
				}
			}
			if senderName != "" {
				if c := p.contact.Get("nickname::" + senderName); c != nil && c.GetAvatar() != "" {
					return c.GetAvatar()
				}
				if c := p.contact.Get("remark::" + senderName); c != nil && c.GetAvatar() != "" {
					return c.GetAvatar()
				}
				if c := p.contact.Get(senderName); c != nil && c.GetAvatar() != "" {
					return c.GetAvatar()
				}
			}
			if !isChatroom {
				if c := p.contact.Get(cleanID); c != nil && c.GetAvatar() != "" {
					return c.GetAvatar()
				}
			}
		}

		// 检查本地 contact.json
		if fileContacts != nil {
			if senderID != "" {
				if fc, ok := fileContacts[senderID]; ok && fc.Avatar != "" {
					return fc.Avatar
				}
			}
			if senderName != "" {
				if fc, ok := fileContacts[senderName]; ok && fc.Avatar != "" {
					return fc.Avatar
				}
				for _, fc := range fileContacts {
					if (fc.Nickname == senderName || fc.Remark == senderName) && fc.Avatar != "" {
						return fc.Avatar
					}
				}
			}
			if !isChatroom {
				if fc, ok := fileContacts[cleanID]; ok && fc.Avatar != "" {
					return fc.Avatar
				}
			}
		}

		return ""
	}

	var messages []ChatMessageItem

	// 提前准备来自 statistics.db 的多媒体记录（图片和语音），供关联匹配
	type statMediaRecord struct {
		id       int64
		msgType  string
		content  string
		raw      string
		ts       string
		consumed bool
	}
	var statMediaList []statMediaRecord
	if ce.statisticsDBPath != "" {
		if db, err := sql.Open("sqlite", ce.statisticsDBPath); err == nil {
			defer db.Close()
			rows, err := db.Query(`
				SELECT id, type, content, COALESCE(raw, ''), CAST(timestamp AS TEXT) as timestamp 
				FROM statistics 
				WHERE (sender = ? OR receiver = ? OR sender = ? OR receiver = ?) 
				  AND type IN ('图片', '语音', 'image', 'voice')
				ORDER BY id ASC
			`, cleanID, cleanID, sessionID, sessionID)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var r statMediaRecord
					if err := rows.Scan(&r.id, &r.msgType, &r.content, &r.raw, &r.ts); err == nil {
						statMediaList = append(statMediaList, r)
					}
				}
			}
		}
	}

	matchStatMedia := func(targetType string) (int64, string, int, string) {
		for i := range statMediaList {
			if statMediaList[i].consumed {
				continue
			}
			st := statMediaList[i].msgType
			if (targetType == "image" && (st == "图片" || st == "image")) ||
				(targetType == "voice" && (st == "语音" || st == "voice")) {
				statMediaList[i].consumed = true
				dur := 0
				if targetType == "voice" {
					dur = parseVoiceDuration(statMediaList[i].content, statMediaList[i].raw)
				}
				mediaURL := fmt.Sprintf("/api/chats/media?id=%d", statMediaList[i].id)
				return statMediaList[i].id, mediaURL, dur, statMediaList[i].ts
			}
		}
		return 0, "", 0, ""
	}

	// 1. 优先从 ai_history.json 读取完整高质量问答流
	contextCutoff := 0
	cutoffPointFound := false
	if ce.aiHistoryPath != "" {
		if content, err := os.ReadFile(ce.aiHistoryPath); err == nil {
			var hist AIHistoryFile
			if err := json.Unmarshal(content, &hist); err == nil {
				targetKey, found := ce.findSessionInHistory(&hist, cleanID)
				if found {
					sess := hist.Sessions[targetKey]
					contextCutoff = sess.ContextCutoff

					type statTimeRow struct {
						ts      string
						content string
					}
					var statTimeline []statTimeRow
					if ce.statisticsDBPath != "" {
						if db, err := sql.Open("sqlite", ce.statisticsDBPath); err == nil {
							defer db.Close()
							q := `
								SELECT CAST(timestamp AS TEXT) as timestamp, content FROM statistics 
								WHERE (sender = ? OR receiver = ?) AND type NOT IN ('状态通知', '系统提示', '系统')
								ORDER BY timestamp ASC
							`
							if sRows, err := db.Query(q, cleanID, cleanID); err == nil {
								defer sRows.Close()
								for sRows.Next() {
									var strTS, strCnt string
									if err := sRows.Scan(&strTS, &strCnt); err == nil {
										statTimeline = append(statTimeline, statTimeRow{ts: strTS, content: strCnt})
									}
								}
							}
						}
					}

					statIdx := 0
					var lastAssignedTime time.Time

					for idx, m := range sess.Messages {
						speakerName := "对方"
						isSelf := m.Role == "assistant"
						if isSelf {
							speakerName = botNickname
						}

						contentStr, msgType, mediaURL, duration := extractContent(m.Content)
						if m.MediaURL != "" && mediaURL == "" {
							mediaURL = m.MediaURL
							msgType = "image"
						}
						if !isSelf {
							if colon := strings.Index(contentStr, ": "); colon > 0 && !strings.Contains(contentStr[:colon], "\n") {
								// 检查前缀字符数（微信昵称最长通常不超过 32 个字符）
								prefix := contentStr[:colon]
								if strings.HasPrefix(prefix, "[距离上一轮") || strings.HasPrefix(prefix, "[已隔夜") {
									if closeBracket := strings.Index(prefix, "] "); closeBracket > 0 {
										prefix = strings.TrimSpace(prefix[closeBracket+2:])
									}
								}
								if utf8.RuneCountInString(prefix) <= 40 {
									speakerName = prefix
									contentStr = contentStr[colon+2:]
								}
							} else if !isChatroom {
								speakerName = cleanID
								if p != nil && p.contact != nil {
									if c := p.contact.Get(cleanID); c != nil {
										if c.GetRemark() != "" {
											speakerName = c.GetRemark()
										} else if c.GetNickname() != "" {
											speakerName = c.GetNickname()
										}
									}
								}
							}

							// 实时对话页面统一显示微信原生昵称/备注名，而非给 AI 称呼用的定制称呼
							for _, ov := range overrides {
								if ov != nil && ov.CustomNickname != "" && ov.CustomNickname == speakerName {
									realName := ov.Remark
									if realName == "" {
										realName = ov.Name
									}
									if realName != "" {
										speakerName = realName
									}
									break
								}
							}
						}

						cleanContent := CleanThoughtTag(contentStr)
						if cleanContent == "" && msgType != "image" && msgType != "voice" {
							continue
						}

						var msgTime time.Time
						cleanTxt := strings.TrimSpace(cleanContent)
						if cleanTxt != "" {
							cRunes := []rune(cleanTxt)
							for i := statIdx; i < len(statTimeline); i++ {
								sCnt := strings.TrimSpace(statTimeline[i].content)
								if sCnt == "" {
									continue
								}
								sRunes := []rune(sCnt)
								isMatch := false
								if cleanTxt == sCnt {
									isMatch = true
								} else if len(cRunes) >= 6 && len(sRunes) >= 6 {
									if strings.Contains(cleanTxt, sCnt) || strings.Contains(sCnt, cleanTxt) {
										isMatch = true
									}
								}
								if isMatch {
									if pt := parseMsgTime(statTimeline[i].ts); !pt.IsZero() {
										msgTime = pt
										statIdx = i + 1
										break
									}
								}
							}
						}

						if msgTime.IsZero() {
							if !lastAssignedTime.IsZero() {
								msgTime = lastAssignedTime.Add(2 * time.Second)
							} else if len(statTimeline) > 0 {
								if pt := parseMsgTime(statTimeline[0].ts); !pt.IsZero() {
									msgTime = pt
								}
							}
							if msgTime.IsZero() {
								if pt := parseMsgTime(sess.UpdatedAt); !pt.IsZero() {
									msgTime = pt
								} else {
									msgTime = time.Now()
								}
							}
						}
						lastAssignedTime = msgTime
						ts := msgTime.In(time.Local).Format(time.RFC3339)

						// 补充多媒体资源关联（仅针对接收到的对方消息，避免机器人自身消息误匹配到历史旧媒体）
						if !isSelf && (msgType == "image" || msgType == "voice") && mediaURL == "" {
							_, statMediaURL, statDur, statTS := matchStatMedia(msgType)
							if statMediaURL != "" {
								mediaURL = statMediaURL
								if duration <= 0 && statDur > 0 {
									duration = statDur
								}
								if statTS != "" {
									if pt := parseMsgTime(statTS); !pt.IsZero() {
										ts = pt.In(time.Local).Format(time.RFC3339)
									}
								}
							}
						}

						// 兜底补全由插件生成的自拍/生图等本地媒体文件
						if isSelf && (msgType == "image" || strings.Contains(contentStr, "生活自拍照") || strings.Contains(contentStr, "绘制的图片")) {
							msgType = "image"
							if mediaURL == "" {
								if strings.Contains(contentStr, "什刹海") {
									if _, err := os.Stat(filepath.Join("data", "cache", "media", "generated_shichahai_selfie.jpg")); err == nil {
										mediaURL = "/api/chats/media?file=generated_shichahai_selfie.jpg"
									}
								}
							}
						}

					av := resolveAvatar(speakerName, speakerName, isSelf)
					isCutoffPoint := false
					if sess.ContextCutoff > 0 && idx >= sess.ContextCutoff && !cutoffPointFound {
						isCutoffPoint = true
						cutoffPointFound = true
					}

					messages = append(messages, ChatMessageItem{
						ID:            int64(idx + 1),
						SessionID:     cleanID,
						SenderID:      speakerName,
						SenderName:    speakerName,
						AvatarURL:     av,
						IsSelf:        isSelf,
						Role:          m.Role,
						Type:          msgType,
						Content:       cleanContent,
						MediaURL:      mediaURL,
						Duration:      duration,
						Timestamp:     ts,
						IsCutoffPoint: isCutoffPoint,
					})
				}
			}
		}
	}
}

	// 2. 从 statistics.db 补充群聊/私聊中未被 ai 记录的真实上行消息（如被海龟汤等插件直接消费的消息）
	if ce.statisticsDBPath != "" {
		if _, err := os.Stat(ce.statisticsDBPath); err == nil {
			db, err := sql.Open("sqlite", ce.statisticsDBPath)
			if err == nil {
				defer db.Close()
				q := `
					SELECT id, type, content, sender, receiver, member, COALESCE(raw, ''), CAST(timestamp AS TEXT) as timestamp 
					FROM statistics 
					WHERE (
						(? LIKE '%@chatroom' AND (sender = ? OR receiver = ?)) OR
						(? NOT LIKE '%@chatroom' AND ((sender = ? AND receiver = ?) OR (sender = ? AND receiver = ?)))
					)
					  AND type != '状态通知' 
					  AND content NOT LIKE '<msg><op%' 
					ORDER BY id ASC
				`
				rows, err := db.Query(q, cleanID, cleanID, cleanID, cleanID, cleanID, botUsername, botUsername, cleanID)
				if err == nil {
					defer rows.Close()
					for rows.Next() {
						var id int64
						var msgType, content, sender, receiver, member, rawStr, ts string
						if err := rows.Scan(&id, &msgType, &content, &sender, &receiver, &member, &rawStr, &ts); err == nil {
							itemTS := ts
							if pt := parseMsgTime(ts); !pt.IsZero() {
								itemTS = pt.In(time.Local).Format(time.RFC3339)
							}

							// 判断是否为系统通知类消息（拍一拍、进群通知、改名、撤回、好友添加等）
							isSysMsg := (msgType == "系统提示" || msgType == "系统" ||
								strings.HasPrefix(strings.TrimSpace(content), "<sysmsg") ||
								strings.Contains(content, "<sysmsg type=") ||
								strings.Contains(rawStr, "<sysmsg type="))

							if isSysMsg {
								noticeText := parseSystemNotice(content, rawStr, cleanID, p, botUsername, memberNameMap)
								if noticeText == "" {
									// 属于协议状态/配置广播等内部无展示意义的噪声，直接跳过
									continue
								}
								exists := false
								for _, em := range messages {
									if em.Type == "system" && em.Content == noticeText {
										exists = true
										break
									}
								}
								if exists {
									continue
								}
								messages = append(messages, ChatMessageItem{
									ID:        id,
									SessionID: cleanID,
									Role:      "system",
									Type:      "system",
									Content:   noticeText,
									Timestamp: itemTS,
								})
								continue
							}

							senderName := member
							if senderName == "" {
								senderName = sender
							}
							role := "user"
							isSelf := false
							if botUsername != "" && sender == botUsername {
								isSelf = true
								role = "assistant"
								senderName = botNickname
							} else if !isChatroom {
								isSelf = false
								role = "user"
								senderName = cleanID
								if p != nil && p.contact != nil {
									if c := p.contact.Get(cleanID); c != nil {
										if c.GetRemark() != "" {
											senderName = c.GetRemark()
										} else if c.GetNickname() != "" {
											senderName = c.GetNickname()
										}
									}
								}
							}

							// 去重检查：如果该消息已经存在于 messages 中，则跳过
							exists := false
							cTrim := strings.TrimSpace(content)
							for _, em := range messages {
								if em.Content == cTrim || (len(cTrim) > 10 && strings.Contains(em.Content, cTrim)) {
									exists = true
									break
								}
							}
							if exists {
								continue
							}

							av := resolveAvatar(sender, senderName, isSelf)
							cleanType := "text"
							mediaURL := ""
							duration := 0

							if msgType == "图片" || msgType == "image" {
								cleanType = "image"
								mediaURL = fmt.Sprintf("/api/chats/media?id=%d", id)
							} else if msgType == "语音" || msgType == "voice" {
								cleanType = "voice"
								mediaURL = fmt.Sprintf("/api/chats/media?id=%d", id)
								duration = parseVoiceDuration(content, rawStr)
							} else if msgType == "文本" || msgType == "text" {
								cleanType = "text"
							} else {
								cleanType = msgType
							}

							messages = append(messages, ChatMessageItem{
								ID:         id,
								SessionID:  cleanID,
								SenderID:   sender,
								SenderName: senderName,
								AvatarURL:  av,
								IsSelf:     isSelf,
								Role:       role,
								Type:       cleanType,
								Content:    content,
								MediaURL:   mediaURL,
								Duration:   duration,
								Timestamp:  itemTS,
							})
						}
					}
				}
			}
		}
	}

	// 3. 合并所有通过 Dashboard 发送的消息 (ce.sentMessages) 以及所有插件出站回复 (plugin_sent.jsonl)
	var allSent []SentRecord
	if sentRecords, ok := ce.sentMessages[cleanID]; ok && len(sentRecords) > 0 {
		allSent = append(allSent, sentRecords...)
	}
	allSent = append(allSent, ce.loadPluginSentRecords(cleanID)...)

	if len(allSent) > 0 {
		botAv := ""
		if p != nil {
			botAv = p.getBotAvatar()
		}
		for _, sr := range allSent {
			srContentTrim := strings.TrimSpace(sr.Content)
			exists := false
			for i := range messages {
				if messages[i].IsSelf {
					mContentTrim := strings.TrimSpace(messages[i].Content)
					if mContentTrim == srContentTrim || (len(srContentTrim) > 15 && strings.Contains(mContentTrim, srContentTrim)) {
						exists = true
						if messages[i].MediaURL == "" && sr.MediaURL != "" {
							messages[i].MediaURL = sr.MediaURL
							messages[i].Type = sr.Type
						}
						break
					}
				}
			}
			if !exists {
				st := sr.Type
				if st == "" {
					st = "text"
				}
				messages = append(messages, ChatMessageItem{
					ID:         sr.ID,
					SessionID:  cleanID,
					SenderID:   botUsername,
					SenderName: botNickname,
					AvatarURL:  botAv,
					IsSelf:     true,
					Role:       "assistant",
					Type:       st,
					Content:    sr.Content,
					MediaURL:   sr.MediaURL,
					Duration:   sr.Duration,
					Timestamp:  sr.Timestamp.In(time.Local).Format(time.RFC3339),
				})
			}
		}
	}

	// 4. 按真实时间戳严格升序排序所有消息（保证管理员代发的消息精准插入在对应时序位置）
	sort.SliceStable(messages, func(i, j int) bool {
		ti := parseMsgTime(messages[i].Timestamp)
		tj := parseMsgTime(messages[j].Timestamp)
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return messages[i].ID < messages[j].ID
	})

	total := len(messages)
	start := 0
	end := total

	if offset > 0 {
		start = offset
		if start > total {
			start = total
		}
		end = start + limit
		if end > total {
			end = total
		}
	} else if total > limit {
		// 默认取最新的 limit 条消息
		start = total - limit
		end = total
	}

	paged := messages[start:end]
	if paged == nil {
		paged = []ChatMessageItem{}
	}

	cutoffAtEnd := (contextCutoff > 0 && !cutoffPointFound)
	return paged, total, contextCutoff, cutoffAtEnd, nil
}

// SearchMessages 全文检索消息
func (ce *ChatEngine) SearchMessages(query string, limit int) ([]ChatMessageItem, error) {
	if limit <= 0 {
		limit = 50
	}
	queryLower := strings.ToLower(strings.TrimSpace(query))
	if queryLower == "" {
		return []ChatMessageItem{}, nil
	}

	var results []ChatMessageItem

	// 1. 优先搜索 AI 核心对话记录
	if ce.aiHistoryPath != "" {
		if content, err := os.ReadFile(ce.aiHistoryPath); err == nil {
			var hist AIHistoryFile
			if err := json.Unmarshal(content, &hist); err == nil {
				for sessKey, sess := range hist.Sessions {
					cleanKey := strings.TrimPrefix(sessKey, "chatroom:")
					cleanKey = strings.TrimPrefix(cleanKey, "private:")
					cleanKey = strings.TrimPrefix(cleanKey, "contact:")
					for idx, m := range sess.Messages {
						rawStr, msgType := extractContentString(m.Content)
						cleanContent := CleanThoughtTag(rawStr)
						if strings.Contains(strings.ToLower(cleanContent), queryLower) {
							speaker := "Bot"
							if ce.botNickname != "" {
								speaker = ce.botNickname
							}
							if m.Role == "user" {
								speaker = cleanKey
							}
							results = append(results, ChatMessageItem{
								ID:         int64(idx + 1),
								SessionID:  cleanKey,
								SenderID:   speaker,
								SenderName: speaker,
								Role:       m.Role,
								Type:       msgType,
								Content:    cleanContent,
								Timestamp:  parseMsgTime(sess.UpdatedAt).In(time.Local).Format(time.RFC3339),
							})
							if len(results) >= limit {
								return results, nil
							}
						}
					}
				}
			}
		}
	}

	// 2. 补充从 SQLite 搜索
	if len(results) < limit && ce.statisticsDBPath != "" {
		if _, err := os.Stat(ce.statisticsDBPath); err == nil {
			db, err := sql.Open("sqlite", ce.statisticsDBPath)
			if err == nil {
				defer db.Close()
				likePattern := "%" + queryLower + "%"
				rows, err := db.Query(`
					SELECT id, type, content, sender, receiver, member, CAST(timestamp AS TEXT) as timestamp 
					FROM statistics 
					WHERE LOWER(content) LIKE ? 
					  AND type NOT IN ('状态通知', '系统提示', '系统') 
					  AND content NOT LIKE '<msg><op%' 
					ORDER BY id DESC 
					LIMIT ?
				`, likePattern, limit-len(results))
				if err == nil {
					defer rows.Close()
					for rows.Next() {
						var id int64
						var msgType, content, sender, receiver, member, ts string
						if err := rows.Scan(&id, &msgType, &content, &sender, &receiver, &member, &ts); err == nil {
							senderName := member
							if senderName == "" {
								senderName = sender
							}
							itemTS := ts
							if pt := parseMsgTime(ts); !pt.IsZero() {
								itemTS = pt.In(time.Local).Format(time.RFC3339)
							}
							results = append(results, ChatMessageItem{
								ID:         id,
								SessionID:  sender,
								SenderID:   sender,
								SenderName: senderName,
								Type:       msgType,
								Content:    content,
								Timestamp:  itemTS,
							})
						}
					}
				}
			}
		}
	}

	return results, nil
}

// ClearSessionContext 清理指定会话的上下文记忆与记录。
// mode: "ai_memory" 仅清除大模型会话短期记忆与时间感知，完整保留聊天记录；
// mode: "complete" 彻底清除，同时清空 AI 记忆并永久删除数据库历史记录与代发缓存。
func (ce *ChatEngine) ClearSessionContext(p *DashboardPlugin, sessionID, mode string) (string, error) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	cleanID := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(sessionID, "chatroom:"), "private:"), "contact:")
	if cleanID == "" {
		return "", fmt.Errorf("会话标识无效: %s", sessionID)
	}

	// 1. 清除 AI 记忆 (无论 mode 是 ai_memory 还是 complete 均需执行)
	// 1.1 尝试通过跨插件能力调用 ai.clear_context (实时生效当前内存)
	if p != nil && p.caller != nil {
		candidates := []string{
			sessionID,
			cleanID,
			"private:" + cleanID,
			"chatroom:" + cleanID,
		}
		for _, cand := range candidates {
			_, _, _ = p.caller.CallPlugin("ai.clear_context", map[string]string{
				"session": cand,
				"mode":    mode,
			})
		}
	}

	// 1.2 处理本地持久化文件 ai_history.json
	if ce.aiHistoryPath != "" {
		if data, err := os.ReadFile(ce.aiHistoryPath); err == nil {
			var hist AIHistoryFile
			if err := json.Unmarshal(data, &hist); err == nil && hist.Sessions != nil {
				changed := false
				if mode == "complete" {
					// 彻底清除：从 ai_history.json 中删除该会话所有记录
					keysToDelete := []string{}
					for k := range hist.Sessions {
						cleanKey := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(k, "chatroom:"), "private:"), "contact:")
						if cleanKey == cleanID || k == sessionID {
							keysToDelete = append(keysToDelete, k)
						}
					}
					for _, k := range keysToDelete {
						delete(hist.Sessions, k)
						changed = true
					}
				} else {
					// 仅清除 AI 记忆：绝不删除历史消息！仅更新 ContextCutoff 为当前消息条数
					for k, sess := range hist.Sessions {
						cleanKey := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(k, "chatroom:"), "private:"), "contact:")
						if cleanKey == cleanID || k == sessionID {
							sess.ContextCutoff = len(sess.Messages)
							hist.Sessions[k] = sess
							changed = true
						}
					}
				}
				if changed {
					if updated, err := json.MarshalIndent(hist, "", "  "); err == nil {
						_ = os.WriteFile(ce.aiHistoryPath, updated, 0644)
					}
				}
			}
		}
	}

	// 2. 如果是彻底清除模式 (complete)，同时删除聊天记录与代发缓存
	if mode == "complete" {
		// 2.1 从 statistics.db 删除该会话所有记录
		if ce.statisticsDBPath != "" {
			if _, err := os.Stat(ce.statisticsDBPath); err == nil {
				db, err := sql.Open("sqlite", ce.statisticsDBPath)
				if err == nil {
					defer db.Close()
					q := `
						DELETE FROM statistics 
						WHERE (
							(? LIKE '%@chatroom' AND (sender = ? OR receiver = ?)) OR
							(? NOT LIKE '%@chatroom' AND (sender = ? OR receiver = ?) AND sender NOT LIKE '%@chatroom' AND receiver NOT LIKE '%@chatroom')
						)
					`
					_, _ = db.Exec(q, cleanID, cleanID, cleanID, cleanID, cleanID, cleanID)
				}
			}
		}

		// 2.2 清理 sentStore 记录
		delete(ce.sentMessages, cleanID)
		delete(ce.sentMessages, sessionID)
		delete(ce.sentMessages, "private:"+cleanID)
		delete(ce.sentMessages, "chatroom:"+cleanID)
		ce.saveSentStoreLocked()

		return "会话聊天记录与 AI 记忆已彻底清除", nil
	}

	return "AI 记忆已成功清除，聊天记录已完整保留", nil
}

