package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	defaultSilkDecoderPath = "/Volumes/GuanMac/Code/Golem/tools/silk-v3-decoder/silk/decoder"
	defaultFFmpegPath      = "/opt/homebrew/bin/ffmpeg"
)

var (
	mediaAesKeyRegex   = regexp.MustCompile(`aeskey="([^"]+)"`)
	mediaMidURLRegex   = regexp.MustCompile(`cdnmidimgurl="([^"]+)"`)
	mediaBigURLRegex   = regexp.MustCompile(`cdnbigimgurl="([^"]+)"`)
	mediaThumbURLRegex = regexp.MustCompile(`cdnthumburl="([^"]+)"`)
	mediaVoiceURLRegex = regexp.MustCompile(`voiceurl="([^"]+)"`)
	voiceLengthRegex   = regexp.MustCompile(`voicelength="([0-9]+)"`)
)

func extractRegexValue(re *regexp.Regexp, s string) string {
	matches := re.FindStringSubmatch(s)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func extractXMLPayload(raw, content string) string {
	var rawJSON struct {
		Content struct {
			Value string `json:"value"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(raw), &rawJSON); err == nil && rawJSON.Content.Value != "" {
		return rawJSON.Content.Value
	}
	if strings.Contains(content, "<msg>") {
		return content
	}
	return raw
}

func parseVoiceDuration(content, raw string) int {
	xml := extractXMLPayload(raw, content)
	if msStr := extractRegexValue(voiceLengthRegex, xml); msStr != "" {
		if ms, err := strconv.Atoi(msStr); err == nil && ms > 0 {
			sec := (ms + 500) / 1000
			if sec == 0 {
				sec = 1
			}
			return sec
		}
	}
	if strings.HasPrefix(content, "[") {
		if end := strings.Index(content, "s]"); end > 1 {
			if ms, err := strconv.Atoi(content[1:end]); err == nil && ms > 0 {
				sec := (ms + 500) / 1000
				if sec == 0 {
					sec = 1
				}
				return sec
			}
		}
	}
	return 1
}

func (p *DashboardPlugin) handleChatMedia(w http.ResponseWriter, r *http.Request) {
	// 验证权限：支持 Bearer、Cookie 以及 URL ?token=
	var tokenStr string
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
	} else if cookie, err := r.Cookie("golem_token"); err == nil {
		tokenStr = cookie.Value
	} else if qToken := r.URL.Query().Get("token"); qToken != "" {
		tokenStr = qToken
	}

	if tokenStr == "" {
		writeError(w, http.StatusUnauthorized, "缺少认证 Token")
		return
	}
	if _, err := ParseAndVerifyJWT(tokenStr, p.store.GetAdmin().JWTSecret); err != nil {
		writeError(w, http.StatusUnauthorized, "Token 验证失败: "+err.Error())
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "缺少 id 参数")
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的 id 参数")
		return
	}

	cacheDir := filepath.Join("data", "cache", "media")
	_ = os.MkdirAll(cacheDir, 0755)

	jpgCache := filepath.Join(cacheDir, fmt.Sprintf("%d.jpg", id))
	wavCache := filepath.Join(cacheDir, fmt.Sprintf("%d.wav", id))

	// 1. 如果本地缓存存在，直接静态响应
	if _, err := os.Stat(jpgCache); err == nil {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, jpgCache)
		return
	}
	if _, err := os.Stat(wavCache); err == nil {
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, wavCache)
		return
	}

	// 2. 从 statistics.db 查询媒体原始 XML
	if p.chatEngine.statisticsDBPath == "" {
		writeError(w, http.StatusNotFound, "统计数据库未就绪")
		return
	}

	db, err := sql.Open("sqlite", p.chatEngine.statisticsDBPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "打开统计数据库失败: "+err.Error())
		return
	}
	defer db.Close()

	var msgType, content, raw string
	row := db.QueryRow("SELECT type, content, raw FROM statistics WHERE id = ?", id)
	if err := row.Scan(&msgType, &content, &raw); err != nil {
		writeError(w, http.StatusNotFound, "媒体消息未找到: "+err.Error())
		return
	}

	xmlPayload := extractXMLPayload(raw, content)

	// 图片消息
	if msgType == "图片" || msgType == "image" {
		data, err := p.downloadImageFromXML(xmlPayload)
		if err != nil {
			slog.Warn("[dashboard] 图片下载失败", "id", id, "err", err)
			writeError(w, http.StatusInternalServerError, "下载图片失败: "+err.Error())
			return
		}
		_ = os.WriteFile(jpgCache, data, 0644)
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(data)
		return
	}

	// 语音消息
	if msgType == "语音" || msgType == "voice" {
		wavData, err := p.downloadAndTranscodeVoice(raw, xmlPayload)
		if err != nil {
			slog.Warn("[dashboard] 语音下载或转码失败", "id", id, "err", err)
			writeError(w, http.StatusInternalServerError, "下载或转码语音失败: "+err.Error())
			return
		}
		_ = os.WriteFile(wavCache, wavData, 0644)
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(wavData)
		return
	}

	writeError(w, http.StatusBadRequest, "不支持的媒体消息类型")
}

func (p *DashboardPlugin) downloadImageFromXML(raw string) ([]byte, error) {
	if p.cdn == nil {
		return nil, errors.New("CDN 传输能力未注入或未就绪")
	}
	aesKey := extractRegexValue(mediaAesKeyRegex, raw)
	if aesKey == "" {
		return nil, errors.New("XML 中未提取到有效的 aeskey")
	}

	candidates := []string{
		extractRegexValue(mediaMidURLRegex, raw),
		extractRegexValue(mediaBigURLRegex, raw),
		extractRegexValue(mediaThumbURLRegex, raw),
	}

	for _, fileID := range candidates {
		if fileID == "" {
			continue
		}
		rc, err := p.cdn.DownloadImage(fileID, aesKey)
		if err == nil && rc != nil {
			buf, readErr := io.ReadAll(rc)
			_ = rc.Close()
			if readErr == nil && len(buf) > 0 {
				return buf, nil
			}
		}
	}

	return nil, errors.New("CDN 下载图片所有候选 URL 均失败")
}

func (p *DashboardPlugin) downloadAndTranscodeVoice(raw, xmlPayload string) ([]byte, error) {
	// 1. 优先从微信协议原始包中的 ImgBuf 提取（微信协议大部分语音消息直接附带在同步包内）
	var rawData struct {
		ImageBuffer struct {
			Data []byte `json:"data"`
		} `json:"image_buffer"`
	}
	if err := json.Unmarshal([]byte(raw), &rawData); err == nil && len(rawData.ImageBuffer.Data) > 0 {
		slog.Info("[dashboard] 成功从协议 ImgBuf 提取语音数据", "bytes", len(rawData.ImageBuffer.Data))
		return transcodeVoiceToWav(rawData.ImageBuffer.Data)
	}

	// 2. 如果 ImgBuf 为空，尝试通过 CDN 下载
	if p.cdn != nil {
		aesKey := extractRegexValue(mediaAesKeyRegex, xmlPayload)
		voiceURL := extractRegexValue(mediaVoiceURLRegex, xmlPayload)
		if aesKey != "" && voiceURL != "" {
			rc, err := p.cdn.DownloadImage(voiceURL, aesKey)
			if err == nil && rc != nil {
				audioBytes, err := io.ReadAll(rc)
				_ = rc.Close()
				if err == nil && len(audioBytes) > 0 {
					return transcodeVoiceToWav(audioBytes)
				}
			}
		}
	}

	return nil, errors.New("未能获取到语音数据（ImgBuf 与 CDN 均无有效数据）")
}

func transcodeVoiceToWav(rawAudio []byte) ([]byte, error) {
	if len(rawAudio) == 0 {
		return nil, errors.New("音频数据为空")
	}

	decoderPath := defaultSilkDecoderPath
	if _, err := os.Stat(decoderPath); err != nil {
		if path, err := exec.LookPath("decoder"); err == nil {
			decoderPath = path
		}
	}

	ffmpegPath := defaultFFmpegPath
	if _, err := os.Stat(ffmpegPath); err != nil {
		if path, err := exec.LookPath("ffmpeg"); err == nil {
			ffmpegPath = path
		}
	}

	// 1. 微信 SILK 格式检查与剥离前导 0x02
	isSilk := false
	silkData := rawAudio
	if bytes.HasPrefix(rawAudio, []byte("\x02#!SILK_V3")) {
		isSilk = true
		silkData = rawAudio[1:]
	} else if bytes.HasPrefix(rawAudio, []byte("#!SILK_V3")) {
		isSilk = true
	} else if len(rawAudio) > 1 && rawAudio[0] == 0x02 {
		isSilk = true
		silkData = rawAudio[1:]
	}

	if isSilk && decoderPath != "" {
		wav, err := decodeSilkWithBinary(silkData, decoderPath, ffmpegPath)
		if err == nil && len(wav) > 0 {
			return wav, nil
		}
		slog.Warn("[dashboard] SILK 专有解码失败，尝试直接使用 ffmpeg 兜底", "err", err)
	}

	// 2. 兜底直接使用 ffmpeg 转码（支持 AMR、MP3、AAC、WAV 等通用格式）
	return transcodeDirectWithFFmpeg(rawAudio, ffmpegPath)
}

func decodeSilkWithBinary(silkData []byte, decoderPath, ffmpeg string) ([]byte, error) {
	silkTmp, err := os.CreateTemp("", "inbound-*.silk")
	if err != nil {
		return nil, fmt.Errorf("创建临时 silk 文件失败: %w", err)
	}
	silkPath := silkTmp.Name()
	defer os.Remove(silkPath)

	if _, err := silkTmp.Write(silkData); err != nil {
		_ = silkTmp.Close()
		return nil, fmt.Errorf("写入临时 silk 文件失败: %w", err)
	}
	_ = silkTmp.Close()

	pcmPath := silkPath + ".pcm"
	defer os.Remove(pcmPath)

	wavPath := silkPath + ".wav"
	defer os.Remove(wavPath)

	cmdDec := exec.Command(decoderPath, silkPath, pcmPath, "-Fs_API", "24000", "-quiet")
	if out, err := cmdDec.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("silk decoder 解码失败: %w, output: %s", err, string(out))
	}

	cmdFF := exec.Command(ffmpeg, "-y", "-f", "s16le", "-ar", "24000", "-ac", "1", "-i", pcmPath, "-ar", "16000", "-ac", "1", "-acodec", "pcm_s16le", wavPath)
	if out, err := cmdFF.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg 转码 WAV 失败: %w, output: %s", err, string(out))
	}

	return os.ReadFile(wavPath)
}

func transcodeDirectWithFFmpeg(audioData []byte, ffmpeg string) ([]byte, error) {
	inTmp, err := os.CreateTemp("", "inbound-direct-*")
	if err != nil {
		return nil, err
	}
	inPath := inTmp.Name()
	defer os.Remove(inPath)

	if _, err := inTmp.Write(audioData); err != nil {
		_ = inTmp.Close()
		return nil, err
	}
	_ = inTmp.Close()

	wavPath := inPath + ".wav"
	defer os.Remove(wavPath)

	cmd := exec.Command(ffmpeg, "-y", "-i", inPath, "-ar", "16000", "-ac", "1", "-acodec", "pcm_s16le", wavPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg 直接转码失败: %w, output: %s", err, string(out))
	}

	return os.ReadFile(wavPath)
}
