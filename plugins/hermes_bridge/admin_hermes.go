package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

var errOpsNotConfigured = fmt.Errorf("未配置 hermes_ops_url")

const (
	opsJSONBodyLimit = 128 << 10
	opsReadLimit     = 32 << 20
)

func copyOpsHeader(dst, src http.Header, key string) {
	if dst == nil || src == nil {
		return
	}
	if v := strings.TrimSpace(src.Get(key)); v != "" {
		dst.Set(key, v)
	}
}

// hermesOpsDo 桥 → hermes_ops；method 支持 GET/POST/PUT/DELETE。
// extraReq 仅用于转发 If-Match 等受控头；Authorization 始终用桥配置的 ops token。
func (p *BridgePlugin) hermesOpsDo(method, path string, query url.Values, body io.Reader, contentType string, extraReq http.Header) (int, []byte, http.Header, error) {
	cfg := p.configSnapshot()
	base := strings.TrimSpace(cfg.HermesOpsURL)
	if base == "" {
		return 0, nil, nil, errOpsNotConfigured
	}
	base = strings.TrimRight(base, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u, err := url.Parse(base + path)
	if err != nil {
		return 0, nil, nil, err
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}
	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return 0, nil, nil, err
	}
	if tok := strings.TrimSpace(cfg.HermesOpsToken); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if contentType != "" && body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	copyOpsHeader(req.Header, extraReq, "If-Match")
	client := p.dlClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, opsReadLimit))
	if err != nil {
		return resp.StatusCode, nil, resp.Header.Clone(), err
	}
	return resp.StatusCode, raw, resp.Header.Clone(), nil
}

func (p *BridgePlugin) writeOpsError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if err == errOpsNotConfigured {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error":   err.Error(),
			"hint":    "在 config.toml 设置 hermes_ops_url（Hermes 侧 hermes_ops 的监听地址）",
			"example": "hermes_ops_url = \"http://<hermes-host>:8650\"",
		})
		return true
	}
	writeJSON(w, http.StatusBadGateway, map[string]any{"error": "ops 请求失败: " + err.Error()})
	return true
}

func writeOpsResponse(w http.ResponseWriter, code int, body []byte, hdr http.Header) {
	ct := ""
	if hdr != nil {
		ct = hdr.Get("Content-Type")
	}
	if ct == "" {
		ct = "application/json"
	}
	w.Header().Set("Content-Type", ct)
	if hdr != nil {
		copyOpsHeader(w.Header(), hdr, "ETag")
		copyOpsHeader(w.Header(), hdr, "Retry-After")
	}
	if strings.HasPrefix(ct, "image/") {
		w.Header().Set("Cache-Control", "private, max-age=3600")
	}
	w.WriteHeader(code)
	_, _ = w.Write(body)
}

func isAllowedPersonaID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || utf8.RuneCountInString(id) > 64 || strings.ContainsAny(id, "/\\") {
		return false
	}
	return utf8.ValidString(id)
}

func isAllowedBindingID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 512 || strings.ContainsAny(id, "/\\") {
		return false
	}
	for _, r := range id {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

// mapAdminHermesPath 把 /admin/hermes/... 映到 ops 路径；ok=false 表示非法。
func mapAdminHermesPath(rest string) (opsPath string, ok bool) {
	rest = strings.TrimSpace(rest)
	if rest == "" || rest == "/" {
		return "", false
	}
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		rest = rest[:i]
	}
	if dec, err := url.PathUnescape(rest); err == nil {
		rest = dec
	}
	if !strings.HasPrefix(rest, "/") {
		rest = "/" + rest
	}
	if rest != "/" {
		rest = strings.TrimSuffix(rest, "/")
		if rest == "" {
			rest = "/"
		}
		if !strings.HasPrefix(rest, "/") {
			rest = "/" + rest
		}
	}
	switch rest {
	case "/health", "/overview", "/tools/check", "/sessions", "/logs",
		"/stickers", "/stickers/facets", "/member_profiles",
		"/personas", "/persona_bindings":
		return rest, true
	}
	if strings.HasPrefix(rest, "/stickers/") {
		tail := strings.TrimPrefix(rest, "/stickers/")
		if tail == "" {
			return "", false
		}
		parts := strings.Split(tail, "/")
		if len(parts) == 1 {
			return "/stickers/" + parts[0], true
		}
		if len(parts) == 2 && parts[1] == "file" && parts[0] != "" {
			return "/stickers/" + parts[0] + "/file", true
		}
		return "", false
	}
	if strings.HasPrefix(rest, "/member_profiles/") {
		tail := strings.TrimPrefix(rest, "/member_profiles/")
		if tail == "" || strings.Contains(tail, "/") {
			return "", false
		}
		return "/member_profiles/" + tail, true
	}
	if strings.HasPrefix(rest, "/personas/") {
		tail := strings.TrimPrefix(rest, "/personas/")
		if !isAllowedPersonaID(tail) {
			return "", false
		}
		return "/personas/" + url.PathEscape(tail), true
	}
	if strings.HasPrefix(rest, "/persona_bindings/") {
		tail := strings.TrimPrefix(rest, "/persona_bindings/")
		if !isAllowedBindingID(tail) {
			return "", false
		}
		return "/persona_bindings/" + url.PathEscape(tail), true
	}
	return "", false
}

func limitJSONBody(r *http.Request) (io.Reader, error) {
	if r.Body == nil {
		return http.NoBody, nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, opsJSONBodyLimit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > opsJSONBodyLimit {
		return nil, errBodyTooLarge
	}
	return bytes.NewReader(raw), nil
}

var errBodyTooLarge = fmt.Errorf("请求体超过 128 KiB")

func (p *BridgePlugin) adminHermesProxy(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	rest := strings.TrimPrefix(path, "/admin/hermes")
	opsPath, ok := mapAdminHermesPath(rest)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error":      "桥侧不识别该 hermes 路径（请确认已编译并重载 hermes_bridge ≥0.17）",
			"path":       path,
			"rest":       rest,
			"hint":       "允许：health|overview|tools/check|sessions|logs|stickers|stickers/<md5>[/file]|member_profiles|member_profiles/<wxid>|personas|personas/<id>|persona_bindings|persona_bindings/<id>",
			"bridge_ver": p.GetMetadata().GetVersion(),
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		code, body, hdr, err := p.hermesOpsDo(http.MethodGet, opsPath, r.URL.Query(), nil, "", nil)
		if p.writeOpsError(w, err) {
			return
		}
		writeOpsResponse(w, code, body, hdr)

	case http.MethodPost:
		if opsPath != "/personas" {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "仅 /personas 支持 POST"})
			return
		}
		limited, err := limitJSONBody(r)
		if err != nil {
			if err == errBodyTooLarge {
				writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		code, body, hdr, err := p.hermesOpsDo(http.MethodPost, opsPath, nil, limited, "application/json", nil)
		if p.writeOpsError(w, err) {
			return
		}
		writeOpsResponse(w, code, body, hdr)

	case http.MethodPut:
		if !strings.HasPrefix(opsPath, "/member_profiles/") && !strings.HasPrefix(opsPath, "/personas/") {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "仅 member_profiles/<wxid> 与 personas/<id> 支持 PUT"})
			return
		}
		limited, err := limitJSONBody(r)
		if err != nil {
			if err == errBodyTooLarge {
				writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		code, body, hdr, err := p.hermesOpsDo(http.MethodPut, opsPath, nil, limited, "application/json", r.Header)
		if p.writeOpsError(w, err) {
			return
		}
		writeOpsResponse(w, code, body, hdr)

	case http.MethodDelete:
		if !strings.HasPrefix(opsPath, "/member_profiles/") &&
			!strings.HasPrefix(opsPath, "/personas/") &&
			!strings.HasPrefix(opsPath, "/persona_bindings/") {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "仅 member_profiles/<wxid>、personas/<id>、persona_bindings/<id> 支持 DELETE"})
			return
		}
		code, body, hdr, err := p.hermesOpsDo(http.MethodDelete, opsPath, nil, nil, "", r.Header)
		if p.writeOpsError(w, err) {
			return
		}
		writeOpsResponse(w, code, body, hdr)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func healthHasCapability(health map[string]any, name string) bool {
	raw, ok := health["capabilities"]
	if !ok || raw == nil {
		return false
	}
	switch caps := raw.(type) {
	case map[string]any:
		v, exists := caps[name]
		if !exists {
			return false
		}
		switch t := v.(type) {
		case bool:
			return t
		case string:
			return t == "true" || t == "1"
		default:
			return v != nil
		}
	case []any:
		for _, item := range caps {
			if s, ok := item.(string); ok && s == name {
				return true
			}
		}
	}
	return false
}

// adminHermesMeta 告诉 UI 是否配置了 ops，并探活 ops 能力（不再用 version 字符串比较）。
func (p *BridgePlugin) adminHermesMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg := p.configSnapshot()
	u := strings.TrimSpace(cfg.HermesOpsURL)
	out := map[string]any{
		"configured": u != "",
		"ops_url":    u,
		"token_set":  strings.TrimSpace(cfg.HermesOpsToken) != "",
		"bridge_ver": p.GetMetadata().GetVersion(),
		"paths": []string{
			"/admin/hermes/health",
			"/admin/hermes/overview",
			"/admin/hermes/tools/check",
			"/admin/hermes/sessions",
			"/admin/hermes/logs",
			"/admin/hermes/stickers",
			"/admin/hermes/stickers/facets",
			"/admin/hermes/stickers/<md5>/file",
			"/admin/hermes/member_profiles",
			"/admin/hermes/personas",
			"/admin/hermes/personas/<id>",
			"/admin/hermes/persona_bindings",
			"/admin/hermes/persona_bindings/<id>",
		},
	}
	if u != "" {
		code, body, _, err := p.hermesOpsDo(http.MethodGet, "/health", nil, nil, "", nil)
		if err != nil {
			out["ops_reachable"] = false
			out["ops_error"] = err.Error()
		} else {
			out["ops_reachable"] = code >= 200 && code < 300
			out["ops_http_status"] = code
			var health map[string]any
			if json.Unmarshal(body, &health) == nil {
				out["ops_health"] = health
				if ver, _ := health["version"].(string); ver != "" {
					out["ops_version"] = ver
				}
				if caps, ok := health["capabilities"]; ok {
					out["ops_capabilities"] = caps
				} else if code >= 200 && code < 300 {
					out["ops_warn"] = "ops /health 未声明 capabilities：人格管理可能不可用，请更新 hermes_ops.py 后重启 ops 服务"
				}
				if code >= 200 && code < 300 && !healthHasCapability(health, "personas.read") {
					if _, ok := health["capabilities"]; ok {
						out["ops_warn"] = "ops 未开放 personas.read：管理台人格页会降级"
					}
				}
			} else {
				out["ops_body"] = string(body)
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}
