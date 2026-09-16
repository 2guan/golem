package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	// 诊断上传比媒体上限多留 1MB，给 multipart 头尾。
	diagnoseMaxUpload = maxVideoBytes + (1 << 20)
	diagnoseMemForm   = 32 << 20
)

type diagnoseReq struct {
	Kind     string
	ChatID   string
	URL      string
	Md5      string
	File     []byte
	Filename string
}

// adminDiagnose POST /admin/diagnose
// JSON: { "kind": "image"|"video"|"voice"|"emoji", "chat_id": "...", "url": "..." }
// emoji 也支持 md5 字段（32 位 hex）代替 url。
// multipart/form-data：字段 kind / chat_id / url / md5，文件字段 file（与 url 二选一，有文件则忽略 url）。
func (p *BridgePlugin) adminDiagnose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, diagnoseMaxUpload)

	req, err := parseDiagnoseReq(r)
	if err != nil {
		code := http.StatusBadRequest
		if isTooLarge(err) {
			code = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, code, map[string]any{"error": err.Error()})
		return
	}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	chatID := strings.TrimSpace(req.ChatID)
	url := strings.TrimSpace(req.URL)
	md5hex := strings.ToLower(strings.TrimSpace(req.Md5))
	if chatID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "chat_id 必填"})
		return
	}
	if kind != "image" && kind != "video" && kind != "voice" && kind != "emoji" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "kind 须为 image / video / voice / emoji"})
		return
	}
	if err := p.guardSendTarget(chatID); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})
		return
	}
	if p.message == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "消息能力未注入"})
		return
	}

	data := req.File
	if len(data) > 0 {
		if err := checkDiagnoseFileSize(kind, int64(len(data))); err != nil {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": err.Error()})
			return
		}
		p.sendDiagnoseBytes(w, kind, chatID, data, req.Filename)
		return
	}

	switch kind {
	case "image":
		if url == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "image 需要 url 或本地文件"})
			return
		}
		data, err := p.downloadImage(url)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "下载失败: " + err.Error()})
			return
		}
		outcome, err := p.sendImageMessage(chatID, data)
		writeDiagnoseResult(w, kind, chatID, len(data), 0, "", outcome, err)

	case "video":
		if url == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "video 需要 url 或本地文件"})
			return
		}
		data, err := p.downloadBytes(url, maxVideoBytes)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "下载失败: " + err.Error()})
			return
		}
		outcome, err := p.sendVideoMessage(chatID, data)
		writeDiagnoseResult(w, kind, chatID, len(data), 0, "", outcome, err)

	case "voice":
		if url == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "voice 需要 url 或本地文件"})
			return
		}
		data, err := p.downloadBytes(url, maxVoiceBytes)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "下载失败: " + err.Error()})
			return
		}
		err = p.sendVoiceBytes(chatID, data)
		outcome := uploadOK
		if err != nil {
			outcome = uploadFailed
		}
		writeDiagnoseResult(w, kind, chatID, len(data), 0, "", outcome, err)

	case "emoji":
		if md5hex != "" && len(md5hex) == 32 {
			if _, err := hex.DecodeString(md5hex); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "md5 非法"})
				return
			}
			outcome, err := p.sendEmojiByMd5(chatID, md5hex)
			writeDiagnoseResult(w, kind, chatID, 0, 0, "", outcome, err)
			return
		}
		if url == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "emoji 需要 url、32 位 md5 或本地文件"})
			return
		}
		data, err := p.downloadEmoji(url)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "下载失败: " + err.Error()})
			return
		}
		before := len(data)
		data = ensureEmojiBytes(data)
		outcome, err := p.sendEmojiMessage(chatID, data)
		writeDiagnoseResult(w, kind, chatID, before, len(data), "", outcome, err)
	}
}

func (p *BridgePlugin) sendDiagnoseBytes(w http.ResponseWriter, kind, chatID string, data []byte, filename string) {
	switch kind {
	case "image":
		outcome, err := p.sendImageMessage(chatID, data)
		writeDiagnoseResult(w, kind, chatID, len(data), 0, filename, outcome, err)
	case "video":
		outcome, err := p.sendVideoMessage(chatID, data)
		writeDiagnoseResult(w, kind, chatID, len(data), 0, filename, outcome, err)
	case "voice":
		err := p.sendVoiceBytes(chatID, data)
		outcome := uploadOK
		if err != nil {
			outcome = uploadFailed
		}
		writeDiagnoseResult(w, kind, chatID, len(data), 0, filename, outcome, err)
	case "emoji":
		before := len(data)
		data = ensureEmojiBytes(data)
		outcome, err := p.sendEmojiMessage(chatID, data)
		writeDiagnoseResult(w, kind, chatID, before, len(data), filename, outcome, err)
	}
}

func parseDiagnoseReq(r *http.Request) (diagnoseReq, error) {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(diagnoseMemForm); err != nil {
			if isTooLarge(err) {
				return diagnoseReq{}, fmt.Errorf("上传超过 %dMB 上限", diagnoseMaxUpload>>20)
			}
			return diagnoseReq{}, fmt.Errorf("multipart 解析失败: %w", err)
		}
		req := diagnoseReq{
			Kind:   r.FormValue("kind"),
			ChatID: r.FormValue("chat_id"),
			URL:    r.FormValue("url"),
			Md5:    r.FormValue("md5"),
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			if errors.Is(err, http.ErrMissingFile) {
				return req, nil
			}
			return diagnoseReq{}, fmt.Errorf("读取文件失败: %w", err)
		}
		defer func() { _ = f.Close() }()
		if hdr != nil {
			req.Filename = hdr.Filename
		}
		data, err := io.ReadAll(io.LimitReader(f, maxVideoBytes+1))
		if err != nil {
			if isTooLarge(err) {
				return diagnoseReq{}, fmt.Errorf("上传超过 %dMB 上限", diagnoseMaxUpload>>20)
			}
			return diagnoseReq{}, fmt.Errorf("读取文件失败: %w", err)
		}
		if len(data) == 0 {
			return diagnoseReq{}, errors.New("上传文件为空")
		}
		req.File = data
		return req, nil
	}

	var body struct {
		Kind   string `json:"kind"`
		ChatID string `json:"chat_id"`
		URL    string `json:"url"`
		Md5    string `json:"md5"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		if isTooLarge(err) {
			return diagnoseReq{}, fmt.Errorf("请求体超过 %dMB 上限", diagnoseMaxUpload>>20)
		}
		return diagnoseReq{}, fmt.Errorf("invalid json: %w", err)
	}
	return diagnoseReq{Kind: body.Kind, ChatID: body.ChatID, URL: body.URL, Md5: body.Md5}, nil
}

func checkDiagnoseFileSize(kind string, n int64) error {
	var max int64
	switch kind {
	case "voice":
		max = maxVoiceBytes
	case "emoji":
		max = maxEmojiRawBytes
	default:
		max = maxVideoBytes
	}
	if n > max {
		return fmt.Errorf("%s 文件超过 %dMB 上限", kind, max>>20)
	}
	return nil
}

func isTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

func writeDiagnoseResult(w http.ResponseWriter, kind, chatID string, bytesIn, bytesOut int, filename string, outcome uploadOutcome, err error) {
	ok := outcome == uploadOK
	resp := map[string]any{
		"ok":       ok,
		"kind":     kind,
		"chat_id":  chatID,
		"outcome":  int(outcome),
		"bytes_in": bytesIn,
	}
	if bytesOut > 0 {
		resp["bytes_out"] = bytesOut
	}
	if filename != "" {
		resp["filename"] = filename
	}
	if err != nil {
		resp["error"] = err.Error()
	}
	if !ok && err == nil {
		resp["error"] = fmt.Sprintf("发送未确认 outcome=%d", outcome)
	}
	code := http.StatusOK
	if !ok {
		code = http.StatusBadGateway
	}
	writeJSON(w, code, resp)
}
