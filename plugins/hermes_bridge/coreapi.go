package main

// 临时：语音/文件入站下载打原生 core HTTP（本机 8080），不经 host SDK。
// host lib 的 DownloadVoice/DownloadFile 仍是 stub；core 接好后只换这一层。

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	coreAPIBase         = "http://127.0.0.1:8080"
	coreFileChunkMax    = 64 << 10 // 实测服务端把 chunk_size 截成 64KB
	coreDownloadTimeout = 60 * time.Second
)

type coreAPIEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type coreBuffer struct {
	Size uint32 `json:"size"`
	Data string `json:"data"` // Base64
}

func (b coreBuffer) bytes() ([]byte, error) {
	s := strings.TrimSpace(b.Data)
	if s == "" {
		if b.Size == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("buffer size=%d 但 data 为空", b.Size)
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("base64 解码失败: %w", err)
	}
	return raw, nil
}

func (p *BridgePlugin) coreHTTP() *http.Client {
	if p.mediaClient != nil {
		return p.mediaClient
	}
	return &http.Client{Timeout: coreDownloadTimeout}
}

func (p *BridgePlugin) postCoreJSON(path string, body any) (json.RawMessage, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(coreAPIBase, "/") + path
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.coreHTTP().Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 core %s 失败: %w", path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, fetchMediaMaxBytes+4096))
	if err != nil {
		return nil, fmt.Errorf("读 core 响应失败: %w", err)
	}
	var env coreAPIEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("core 响应非 JSON（http %d）: %w", resp.StatusCode, err)
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("core %s code=%d message=%s", path, env.Code, env.Message)
	}
	return env.Data, nil
}

// downloadVoiceViaCore 原生 POST /api/message/download/voice。
// 字段见 Apifox：id/new_id/length/buffer_id/chatroom_id（私聊空串）。
func (p *BridgePlugin) downloadVoiceViaCore(newID uint64, length, bufID uint32, chatroomID string) []byte {
	if newID == 0 || length == 0 {
		return nil
	}
	body := map[string]any{
		"id":          newID,
		"new_id":      newID,
		"length":      length,
		"buffer_id":   bufID,
		"chatroom_id": chatroomID,
	}
	data, err := p.postCoreJSON("/api/message/download/voice", body)
	if err != nil {
		slog.Warn("[hermes_bridge] core 下载语音失败", "new_id", newID, "err", err)
		return nil
	}
	var resp struct {
		Size     uint32     `json:"size"`
		Duration uint32     `json:"duration"`
		EndFlag  uint32     `json:"end_flag"`
		Data     coreBuffer `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		slog.Warn("[hermes_bridge] core 语音响应解析失败", "err", err)
		return nil
	}
	raw, err := resp.Data.bytes()
	if err != nil {
		slog.Warn("[hermes_bridge] core 语音 buffer 解码失败", "err", err)
		return nil
	}
	if len(raw) == 0 {
		slog.Warn("[hermes_bridge] core 语音返回空数据", "new_id", newID, "resp_size", resp.Size)
		return nil
	}
	return raw
}

// downloadFileViaCore 原生 POST /api/message/download/file，按 64KB 分片拼满 totallen。
func (p *BridgePlugin) downloadFileViaCore(username, attachID, appID string, total uint32) []byte {
	username = strings.TrimSpace(username)
	attachID = strings.TrimSpace(attachID)
	if username == "" || attachID == "" || total == 0 {
		return nil
	}
	if int64(total) > fetchMediaMaxBytes {
		slog.Warn("[hermes_bridge] 入站文件超过上限，拒绝下载", "size", total, "limit", fetchMediaMaxBytes)
		return nil
	}
	out := make([]byte, 0, total)
	var offset uint32
	for offset < total {
		remain := total - offset
		reqN := remain
		if reqN > coreFileChunkMax {
			reqN = coreFileChunkMax
		}
		body := map[string]any{
			"username":   username,
			"attach_id":  attachID,
			"size":       total,
			"offset":     offset,
			"chunk_size": reqN,
		}
		if appID = strings.TrimSpace(appID); appID != "" {
			body["app_id"] = appID
		}
		data, err := p.postCoreJSON("/api/message/download/file", body)
		if err != nil {
			slog.Warn("[hermes_bridge] core 下载文件失败",
				"offset", offset, "err", err)
			return nil
		}
		var resp struct {
			Size      uint32     `json:"size"`
			Offset    uint32     `json:"offset"`
			ChunkSize uint32     `json:"chunk_size"`
			Chunk     coreBuffer `json:"chunk"`
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			slog.Warn("[hermes_bridge] core 文件响应解析失败", "err", err)
			return nil
		}
		chunk, err := resp.Chunk.bytes()
		if err != nil {
			slog.Warn("[hermes_bridge] core 文件 chunk 解码失败", "err", err)
			return nil
		}
		if len(chunk) == 0 {
			slog.Warn("[hermes_bridge] core 文件空分片", "offset", offset)
			return nil
		}
		out = append(out, chunk...)
		offset += uint32(len(chunk))
	}
	if uint32(len(out)) != total {
		slog.Warn("[hermes_bridge] core 文件拼片长度不符", "got", len(out), "want", total)
		return nil
	}
	return out
}
