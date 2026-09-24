package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
)

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error":   http.StatusText(status),
		"message": message,
	})
}

// ----------------- Auth Handlers -----------------

func (p *DashboardPlugin) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求参数解析失败")
		return
	}

	admin := p.store.GetAdmin()
	if req.Username != admin.Username {
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	if !CheckPasswordHash(req.Password, admin.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	// 签发 7 天有效期的 Token
	token, expireAt, err := GenerateJWT(admin.Username, admin.JWTSecret, 7*24*time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "签发 Token 失败: "+err.Error())
		return
	}

	// 设置 Cookie 方便浏览器携带
	http.SetCookie(w, &http.Cookie{
		Name:     "golem_token",
		Value:    token,
		Path:     "/",
		Expires:  time.Unix(expireAt, 0),
		HttpOnly: true,
	})

	writeJSON(w, http.StatusOK, LoginResponse{
		Token:    token,
		Username: admin.Username,
		ExpireAt: expireAt,
	})
}

func (p *DashboardPlugin) handleProfile(w http.ResponseWriter, r *http.Request) {
	admin := p.store.GetAdmin()
	writeJSON(w, http.StatusOK, map[string]any{
		"username":   admin.Username,
		"created_at": admin.CreatedAt,
		"updated_at": admin.UpdatedAt,
	})
}

func (p *DashboardPlugin) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求参数解析失败")
		return
	}

	if len(req.NewPassword) < 5 {
		writeError(w, http.StatusBadRequest, "新密码长度不能少于 5 位")
		return
	}

	admin := p.store.GetAdmin()
	if !CheckPasswordHash(req.OldPassword, admin.PasswordHash) {
		writeError(w, http.StatusBadRequest, "原密码不正确")
		return
	}

	newHash, err := HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "密码哈希失败")
		return
	}

	if err := p.store.UpdateAdminPassword(newHash); err != nil {
		writeError(w, http.StatusInternalServerError, "保存新密码失败: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "密码修改成功，请重新登录"})
}

// ----------------- Overview Handler -----------------

func (p *DashboardPlugin) handleOverview(w http.ResponseWriter, r *http.Request) {
	var resp OverviewResponse
	resp.Bot.Status = "online"
	resp.Bot.Uptime = int64(time.Since(p.startTime).Seconds())

	// 从 contact 获取机器人信息
	if p.contact != nil {
		self := p.contact.GetSelf()
		if self != nil {
			resp.Bot.Username = self.GetUsername()
			resp.Bot.Nickname = self.GetNickname()
			resp.Bot.AvatarURL = self.GetAvatar()
		}
		if resp.Bot.AvatarURL == "" && resp.Bot.Username != "" {
			if c := p.contact.Get(resp.Bot.Username); c != nil && c.GetAvatar() != "" {
				resp.Bot.AvatarURL = c.GetAvatar()
			}
		}
		owner := p.contact.GetOwner()
		if owner != nil {
			resp.Bot.Owner = owner.GetNickname()
			if resp.Bot.Owner == "" {
				resp.Bot.Owner = owner.GetUsername()
			}
		}

		contacts := p.contact.List()
		for _, c := range contacts {
			if c == nil {
				continue
			}
			if strings.HasSuffix(c.GetUsername(), "@chatroom") || c.GetType().String() == "CONTACT_TYPE_CHATROOM" {
				resp.Stats.TotalGroups++
			} else {
				resp.Stats.TotalContacts++
			}
		}
	}

	// 插件总数
	plugins, _ := p.configMgr.ListPlugins()
	resp.Stats.PluginCount = len(plugins)

	// 会话统计
	sessions, _ := p.chatEngine.GetSessions(p)
	resp.Stats.ActiveSessions = len(sessions)
	for _, s := range sessions {
		resp.Stats.TotalMessages += s.MsgCount
	}

	writeJSON(w, http.StatusOK, resp)
}

// ----------------- Config Handlers -----------------

func (p *DashboardPlugin) handleGlobalConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		structured, rawToml, err := p.configMgr.ReadGlobalConfigStructured()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "读取全局配置失败: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"structured": structured,
			"toml":       rawToml,
		})
		return
	}

	if r.Method == http.MethodPut || r.Method == http.MethodPost {
		var body struct {
			Structured map[string]any `json:"structured"`
			Toml       string         `json:"toml"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "解析请求参数失败")
			return
		}

		if len(body.Structured) > 0 {
			if err := p.configMgr.WriteGlobalConfigStructured(body.Structured); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"message": "全局配置更新成功"})
			return
		}

		if body.Toml != "" {
			if err := p.configMgr.WriteGlobalConfig(body.Toml); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"message": "全局 TOML 配置保存成功"})
			return
		}

		writeError(w, http.StatusBadRequest, "缺少配置参数")
		return
	}

	writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (p *DashboardPlugin) handlePluginsConfig(w http.ResponseWriter, r *http.Request) {
	list, err := p.configMgr.ListPlugins()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取插件列表失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (p *DashboardPlugin) handlePluginToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Name   string `json:"name"`
		Enable bool   `json:"enable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "解析参数失败")
		return
	}

	if err := p.configMgr.TogglePlugin(req.Name, req.Enable); err != nil {
		writeError(w, http.StatusInternalServerError, "切换插件状态失败: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"message": "插件状态已更新", "name": req.Name, "enable": req.Enable})
}

func (p *DashboardPlugin) handlePluginSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Name    string         `json:"name"`
		Section map[string]any `json:"section,omitempty"`
		RawToml string         `json:"raw_toml,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "解析参数失败: "+err.Error())
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "缺少插件名称")
		return
	}

	if strings.TrimSpace(req.RawToml) != "" {
		if err := p.configMgr.SavePluginRawToml(req.Name, req.RawToml); err != nil {
			writeError(w, http.StatusBadRequest, "保存 TOML 失败: "+err.Error())
			return
		}
	} else if req.Section != nil {
		if err := p.configMgr.SavePluginSection(req.Name, req.Section); err != nil {
			writeError(w, http.StatusInternalServerError, "保存插件配置失败: "+err.Error())
			return
		}
	} else {
		writeError(w, http.StatusBadRequest, "未提供配置内容")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "插件配置更新成功，已立即生效"})
}

// ----------------- Target Overrides Handlers -----------------

func (p *DashboardPlugin) handleTargets(w http.ResponseWriter, r *http.Request) {
	overrides := p.store.GetOverrides()

	targetMap := make(map[string]map[string]any)

	// 填充现有通讯录
	if p.contact != nil {
		contacts := p.contact.List()
		for _, c := range contacts {
			if c == nil || c.GetUsername() == "" {
				continue
			}
			tID := c.GetUsername()
			tType := "friend"
			if strings.HasSuffix(tID, "@chatroom") || c.GetType().String() == "CONTACT_TYPE_CHATROOM" {
				tType = "chatroom"
			} else if c.GetType().String() == "CONTACT_TYPE_OFFICIAL" {
				tType = "official"
			}

			name := c.GetNickname()
			if name == "" {
				name = tID
			}

			item := map[string]any{
				"target_id":    tID,
				"target_type":  tType,
				"name":         name,
				"remark":       c.GetRemark(),
				"avatar_url":   c.GetAvatar(),
				"has_override": false,
			}
			targetMap[tID] = item
		}
	}

	// 附加定制策略
	for tID, ov := range overrides {
		if item, ok := targetMap[tID]; ok {
			item["has_override"] = true
			item["custom_prompt"] = ov.CustomPrompt
			item["provider"] = ov.Provider
			item["reply_rate"] = ov.ReplyRate
			item["silence"] = ov.Silence
			item["notes"] = ov.Notes
		} else {
			targetMap[tID] = map[string]any{
				"target_id":     tID,
				"target_type":   ov.TargetType,
				"name":          ov.Name,
				"remark":        ov.Remark,
				"has_override":  true,
				"custom_prompt": ov.CustomPrompt,
				"provider":      ov.Provider,
				"reply_rate":    ov.ReplyRate,
				"silence":       ov.Silence,
				"notes":         ov.Notes,
			}
		}
	}

	var list []any
	for _, item := range targetMap {
		list = append(list, item)
	}

	writeJSON(w, http.StatusOK, list)
}

func (p *DashboardPlugin) handleTargetSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req TargetOverride
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "解析参数失败")
		return
	}

	if req.TargetID == "" {
		writeError(w, http.StatusBadRequest, "target_id 不能为空")
		return
	}

	// 保存到 dashboard.json
	if err := p.store.SetOverride(&req); err != nil {
		writeError(w, http.StatusInternalServerError, "保存定制策略失败: "+err.Error())
		return
	}

	// 同步写回 plugins/config.toml 的 [ai.config.session_configs]
	if err := p.configMgr.SaveTargetOverrideToAIConfig(req.TargetID, req.CustomPrompt, req.Provider, req.ReplyRate, req.Silence); err != nil {
		writeError(w, http.StatusInternalServerError, "同步至 AI 配置失败: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "会话专属策略保存并生效成功"})
}

func (p *DashboardPlugin) handleTargetDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		TargetID string `json:"target_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "解析参数失败")
		return
	}

	if err := p.store.DeleteOverride(req.TargetID); err != nil {
		writeError(w, http.StatusInternalServerError, "移除策略失败: "+err.Error())
		return
	}

	if err := p.configMgr.RemoveTargetOverrideFromAIConfig(req.TargetID); err != nil {
		writeError(w, http.StatusInternalServerError, "从 AI 配置中移除失败: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "专属策略已清除，恢复全局默认"})
}

// ----------------- Chat Explorer Handlers -----------------

func (p *DashboardPlugin) handleChatSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := p.chatEngine.GetSessions(p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取会话列表失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (p *DashboardPlugin) handleChatMessages(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "缺少 session 参数")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	messages, total, cutoff, cutoffAtEnd, err := p.chatEngine.GetMessages(p, sessionID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取聊天消息失败: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"session":        sessionID,
		"total":          total,
		"limit":          limit,
		"offset":         offset,
		"context_cutoff": cutoff,
		"cutoff_at_end":  cutoffAtEnd,
		"messages":       messages,
	})
}

func (p *DashboardPlugin) handleChatSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	results, err := p.chatEngine.SearchMessages(q, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "检索消息失败: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, results)
}

type ClearChatContextRequest struct {
	Session string `json:"session"`
	Mode    string `json:"mode"` // "ai_memory" 或 "complete"
}

func (p *DashboardPlugin) handleChatClearContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req ClearChatContextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求参数解析失败")
		return
	}

	req.Session = strings.TrimSpace(req.Session)
	if req.Session == "" {
		writeError(w, http.StatusBadRequest, "缺少 session 参数")
		return
	}
	if req.Mode != "ai_memory" && req.Mode != "complete" {
		writeError(w, http.StatusBadRequest, "无效的清除模式，可选: ai_memory (清除AI记忆) 或 complete (彻底清除)")
		return
	}

	msg, err := p.chatEngine.ClearSessionContext(p, req.Session, req.Mode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "清除上下文失败: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": msg,
		"mode":    req.Mode,
		"session": req.Session,
	})
}

func (p *DashboardPlugin) resolveReceiver(id string) *contact.Contact {
	if p.contact != nil {
		if c := p.contact.Get("username::" + id); c != nil {
			return c
		}
	}
	return &contact.Contact{Username: id}
}

func (p *DashboardPlugin) handleChatSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Session string `json:"session"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "解析参数失败: "+err.Error())
		return
	}

	targetID := strings.TrimSpace(req.Session)
	content := strings.TrimSpace(req.Content)

	if targetID == "" {
		writeError(w, http.StatusBadRequest, "缺少目标会话 ID")
		return
	}
	if content == "" {
		writeError(w, http.StatusBadRequest, "发送内容不能为空")
		return
	}

	if p.message == nil {
		writeError(w, http.StatusInternalServerError, "消息服务能力未就绪或未注入")
		return
	}

	// 剥离可能存在的 session 前缀
	cleanTarget := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(targetID, "chatroom:"), "private:"), "contact:")

	// 1. 发送微信消息
	receiver := p.resolveReceiver(cleanTarget)
	msg := &message.Message{
		Type:     message.TypeText,
		Receiver: receiver,
		Content:  content,
		Data: &message.Message_Text{
			Text: &message.TextData{
				Content: content,
			},
		},
	}

	if _, err := p.message.Send(msg); err != nil {
		writeError(w, http.StatusInternalServerError, "微信消息发送失败: "+err.Error())
		return
	}

	// 2. 追加到本地上下文历史 (ai_history.json) 与 sent store 以及 statistics.db
	if p.chatEngine != nil {
		_ = p.chatEngine.RecordSentMessage(cleanTarget, content, p.getBotUsername())
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":   "发送成功",
		"session":   cleanTarget,
		"content":   content,
		"sent_time": time.Now().Format(time.RFC3339),
	})
}

func (p *DashboardPlugin) getBotUsername() string {
	if p.contact != nil {
		self := p.contact.GetSelf()
		if self != nil && self.GetUsername() != "" {
			return self.GetUsername()
		}
	}
	return ""
}

func (p *DashboardPlugin) getBotNickname() string {
	if p.contact != nil {
		self := p.contact.GetSelf()
		if self != nil && self.GetNickname() != "" {
			return self.GetNickname()
		}
	}
	return "Bot"
}

func (p *DashboardPlugin) handleChatSendImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Session string `json:"session"`
		Image   string `json:"image"` // base64 data URI
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "解析参数失败: "+err.Error())
		return
	}

	targetID := strings.TrimSpace(req.Session)
	rawImage := strings.TrimSpace(req.Image)

	if targetID == "" {
		writeError(w, http.StatusBadRequest, "缺少目标会话 ID")
		return
	}
	if rawImage == "" {
		writeError(w, http.StatusBadRequest, "图片数据不能为空")
		return
	}

	if p.message == nil {
		writeError(w, http.StatusInternalServerError, "消息服务能力未就绪或未注入")
		return
	}

	cleanTarget := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(targetID, "chatroom:"), "private:"), "contact:")

	// 提取 Base64 图片二进制
	b64Data := rawImage
	if idx := strings.Index(b64Data, ","); idx != -1 {
		b64Data = b64Data[idx+1:]
	}
	imgBytes, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Base64 图片解码失败: "+err.Error())
		return
	}

	receiver := p.resolveReceiver(cleanTarget)
	msg := &message.Message{
		Type:     message.TypeImage,
		Receiver: receiver,
		Content:  "[图片]",
		Data: &message.Message_Image{
			Image: &message.ImageData{
				Media: &message.Media{
					Data: imgBytes,
					Size: uint32(len(imgBytes)),
				},
			},
		},
	}

	if _, err := p.message.Send(msg); err != nil {
		writeError(w, http.StatusInternalServerError, "微信图片发送失败: "+err.Error())
		return
	}

	// 缓存该图片至本地
	cacheDir := filepath.Join("data", "cache", "media")
	_ = os.MkdirAll(cacheDir, 0755)
	cacheImgPath := filepath.Join(cacheDir, fmt.Sprintf("sent_%d.jpg", time.Now().UnixNano()))
	_ = os.WriteFile(cacheImgPath, imgBytes, 0644)

	// 记录到本地存储和历史库
	if p.chatEngine != nil {
		_ = p.chatEngine.RecordSentMediaMessage(cleanTarget, "image", "[图片]", rawImage, 0, p.getBotUsername())
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":   "图片发送成功",
		"session":   cleanTarget,
		"media_url": rawImage,
		"sent_time": time.Now().Format(time.RFC3339),
	})
}

func (p *DashboardPlugin) handleChatSendVoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Session     string `json:"session"`
		Text        string `json:"text"`
		VoiceDesign string `json:"voice_design"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "解析参数失败: "+err.Error())
		return
	}

	targetID := strings.TrimSpace(req.Session)
	text := strings.TrimSpace(req.Text)
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "缺少目标会话 ID")
		return
	}
	if text == "" {
		writeError(w, http.StatusBadRequest, "语音文本内容不能为空")
		return
	}

	durationSec, mediaURL, err := p.synthesizeAndSendVoice(targetID, text, req.VoiceDesign)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "语音合成或发送失败: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":   "语音发送成功",
		"session":   targetID,
		"duration":  durationSec,
		"media_url": mediaURL,
		"text":      text,
		"sent_time": time.Now().Format(time.RFC3339),
	})
}

func (p *DashboardPlugin) handleGetTTSConfig(w http.ResponseWriter, r *http.Request) {
	cfg := p.loadTTSConfig()
	writeJSON(w, http.StatusOK, map[string]any{
		"voice_design": cfg.VoiceDesign,
		"model":        cfg.Model,
		"voice":        cfg.Voice,
		"enable":       cfg.Enable,
	})
}

// ----------------- AI Analyze Handler -----------------

func (p *DashboardPlugin) handleAIAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req AIAnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "解析参数失败")
		return
	}

	if req.TargetID == "" {
		writeError(w, http.StatusBadRequest, "缺少 target_id")
		return
	}

	// 读取该会话消息历史
	messages, _, _, _, err := p.chatEngine.GetMessages(p, req.TargetID, 100, 0)
	if err != nil || len(messages) == 0 {
		writeError(w, http.StatusBadRequest, "该对象无足够历史消息供分析")
		return
	}

	targetName := req.TargetID
	if p.contact != nil {
		c := p.contact.Get(req.TargetID)
		if c != nil {
			targetName = c.GetRemark()
			if targetName == "" {
				targetName = c.GetNickname()
			}
		}
	}

	result, modelUsed, err := p.aiService.Analyze(targetName, messages, req.Type, req.PromptHint)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "AI 分析失败: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, AIAnalyzeResponse{
		TargetID:  req.TargetID,
		Type:      req.Type,
		Result:    result,
		ModelUsed: modelUsed,
	})
}

// ----------------- Log Explorer Handlers -----------------

func (p *DashboardPlugin) handleLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	opts := LogFilterOptions{
		Level:   q.Get("level"),
		Keyword: q.Get("keyword"),
		Tag:     q.Get("tag"),
		Since:   q.Get("since"),
		Until:   q.Get("until"),
		Order:   q.Get("order"),
		Limit:   limit,
		Offset:  offset,
	}

	resp, err := p.logEngine.Query(opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "检索日志失败: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (p *DashboardPlugin) handleLogsTags(w http.ResponseWriter, r *http.Request) {
	tags, err := p.logEngine.GetDistinctTags()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "获取日志标签失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

func (p *DashboardPlugin) handleLogsExport(w http.ResponseWriter, r *http.Request) {
	file, err := os.Open(p.logEngine.logFilePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "日志文件不存在")
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"golem.log\"")
	_, _ = io.Copy(w, file)
}

func (p *DashboardPlugin) getBotAvatar() string {
	if p.contact != nil {
		if self := p.contact.GetSelf(); self != nil && self.GetAvatar() != "" {
			return self.GetAvatar()
		}
		if username := p.getBotUsername(); username != "" {
			if c := p.contact.Get(username); c != nil && c.GetAvatar() != "" {
				return c.GetAvatar()
			}
		}
	}
	return ""
}

