package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/sbgayhub/golem/sdk/message"
)

// TTSConfig TTS 语音合成与转码配置
type TTSConfig struct {
	Enable          bool   `toml:"enable"`
	Model           string `toml:"model"`
	VoiceDesign     string `toml:"voice_design"`
	Voice           string `toml:"voice"`
	BaseURL         string `toml:"base_url"`
	APIKey          string `toml:"api_key"`
	SilkEncoderPath string `toml:"silk_encoder_path"`
	FFmpegPath      string `toml:"ffmpeg_path"`
	FFprobePath     string `toml:"ffprobe_path"`
}

func defaultDashboardTTSConfig() TTSConfig {
	return TTSConfig{
		Enable:          true,
		Model:           "mimo-v2.5-tts-voicedesign",
		VoiceDesign:     "一位三十多岁的成熟男性朋友。嗓音富有磁性有质感，但音调自然轻松不沉闷。说话亲切温和、随性自如，语速轻快，带有自然的口语起伏和笑意，像日常随手拿起手机给朋友发微信语音闲聊。",
		Voice:           "白桦",
		BaseURL:         "https://token-plan-cn.xiaomimimo.com/v1",
		APIKey:          "tp-ckyf7ojdhr6al7jx9o7wdycdbbfhrmz6wogkvwjocjt8xjkd",
		SilkEncoderPath: "/Volumes/GuanMac/Code/Golem/tools/silk_v3_encoder",
		FFmpegPath:      "/opt/homebrew/bin/ffmpeg",
		FFprobePath:     "/opt/homebrew/bin/ffprobe",
	}
}

func (p *DashboardPlugin) loadTTSConfig() TTSConfig {
	cfg := defaultDashboardTTSConfig()

	// 优先从 plugins/config.toml 读取 [ai.config.tts]
	paths := []string{"plugins/config.toml", "data/config.toml"}
	for _, confPath := range paths {
		if data, err := os.ReadFile(confPath); err == nil {
			var root struct {
				AI struct {
					Config struct {
						TTS       TTSConfig `toml:"tts"`
						Providers map[string]struct {
							APIKey  string `toml:"api_key"`
							BaseURL string `toml:"base_url"`
						} `toml:"providers"`
					} `toml:"config"`
				} `toml:"ai"`
			}
			if err := toml.Unmarshal(data, &root); err == nil {
				tts := root.AI.Config.TTS
				if tts.VoiceDesign != "" {
					cfg.VoiceDesign = tts.VoiceDesign
				}
				if tts.Model != "" {
					cfg.Model = tts.Model
				}
				if tts.Voice != "" {
					cfg.Voice = tts.Voice
				}
				if tts.BaseURL != "" {
					cfg.BaseURL = tts.BaseURL
				}
				if tts.APIKey != "" {
					cfg.APIKey = tts.APIKey
				}
				if tts.SilkEncoderPath != "" {
					cfg.SilkEncoderPath = tts.SilkEncoderPath
				}
				if tts.FFmpegPath != "" {
					cfg.FFmpegPath = tts.FFmpegPath
				}
				if tts.FFprobePath != "" {
					cfg.FFprobePath = tts.FFprobePath
				}

				if cfg.BaseURL == "" || cfg.APIKey == "" {
					if xiaomi, ok := root.AI.Config.Providers["xiaomi"]; ok {
						if cfg.BaseURL == "" {
							cfg.BaseURL = xiaomi.BaseURL
						}
						if cfg.APIKey == "" {
							cfg.APIKey = xiaomi.APIKey
						}
					}
				}
				break
			}
		}
	}
	return cfg
}

func (p *DashboardPlugin) synthesizeAndSendVoice(targetID, text, customVoiceDesign string) (int, string, error) {
	if p.message == nil {
		return 0, "", errors.New("微信消息能力未就绪或未注入")
	}

	cleanTarget := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(targetID, "chatroom:"), "private:"), "contact:")
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, "", errors.New("语音内容不能为空")
	}

	cfg := p.loadTTSConfig()
	voiceDesign := strings.TrimSpace(customVoiceDesign)
	if voiceDesign == "" {
		voiceDesign = cfg.VoiceDesign
	}

	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"

	// 构造 MiMo TTS 请求
	type reqMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	var msgs []reqMsg
	if voiceDesign != "" {
		msgs = append(msgs, reqMsg{Role: "user", Content: voiceDesign})
	}
	msgs = append(msgs, reqMsg{Role: "assistant", Content: text})

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "mimo-v2.5-tts-voicedesign"
	}
	voiceName := strings.TrimSpace(cfg.Voice)
	if voiceName == "" {
		voiceName = "白桦"
	}

	audioParam := map[string]string{
		"format": "wav",
	}
	if model == "mimo-v2.5-tts" {
		audioParam["voice"] = voiceName
	}

	reqPayload := map[string]any{
		"model":    model,
		"messages": msgs,
		"audio":    audioParam,
	}

	wavBytes, err := requestTTSAudio(endpoint, cfg.APIKey, reqPayload)
	if err != nil && model == "mimo-v2.5-tts-voicedesign" {
		slog.Warn("[dashboard] voicedesign 合成失败，尝试降级到预设发音人 mimo-v2.5-tts", "err", err)
		fallbackPayload := map[string]any{
			"model": "mimo-v2.5-tts",
			"messages": []reqMsg{
				{Role: "assistant", Content: text},
			},
			"audio": map[string]string{
				"format": "wav",
				"voice":  voiceName,
			},
		}
		if fbWav, fbErr := requestTTSAudio(endpoint, cfg.APIKey, fallbackPayload); fbErr == nil {
			wavBytes = fbWav
			err = nil
		}
	}
	if err != nil {
		return 0, "", err
	}

	// 写入临时 WAV 文件
	wavTmp, err := os.CreateTemp("", "tts-voice-*.wav")
	if err != nil {
		return 0, "", err
	}
	wavPath := wavTmp.Name()
	defer os.Remove(wavPath)
	if _, err := wavTmp.Write(wavBytes); err != nil {
		_ = wavTmp.Close()
		return 0, "", err
	}
	_ = wavTmp.Close()

	// 用 ffprobe 测量音频时长
	ffprobe := cfg.FFprobePath
	if _, err := os.Stat(ffprobe); err != nil {
		if p, err := exec.LookPath("ffprobe"); err == nil {
			ffprobe = p
		}
	}
	durationMs := 1500
	if ffprobe != "" {
		durCmd := exec.Command(ffprobe, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", wavPath)
		if durOutput, err := durCmd.Output(); err == nil {
			if sec, parseErr := strconv.ParseFloat(strings.TrimSpace(string(durOutput)), 64); parseErr == nil {
				durationMs = int(sec * 1000)
				if durationMs < 1000 {
					durationMs = 1000
				}
			}
		}
	}

	// 转为 24000Hz PCM
	ffmpeg := cfg.FFmpegPath
	if _, err := os.Stat(ffmpeg); err != nil {
		if p, err := exec.LookPath("ffmpeg"); err == nil {
			ffmpeg = p
		}
	}
	if ffmpeg == "" {
		return 0, "", errors.New("未找到 ffmpeg 工具")
	}

	pcmTmp, err := os.CreateTemp("", "tts-voice-*.pcm")
	if err != nil {
		return 0, "", err
	}
	pcmPath := pcmTmp.Name()
	_ = pcmTmp.Close()
	defer os.Remove(pcmPath)

	ffCmd := exec.Command(ffmpeg, "-y", "-i", wavPath, "-ar", "24000", "-ac", "1", "-f", "s16le", pcmPath)
	if ffOut, err := ffCmd.CombinedOutput(); err != nil {
		return 0, "", fmt.Errorf("ffmpeg 转 PCM 失败: %w, output: %s", err, string(ffOut))
	}

	// 用 silk_v3_encoder 编码为 Tencent SILK
	silkEncoder := cfg.SilkEncoderPath
	if _, err := os.Stat(silkEncoder); err != nil {
		if p, err := exec.LookPath("silk_v3_encoder"); err == nil {
			silkEncoder = p
		}
	}
	if silkEncoder == "" {
		return 0, "", errors.New("未找到 silk_v3_encoder 工具")
	}

	silkTmp, err := os.CreateTemp("", "tts-voice-*.silk")
	if err != nil {
		return 0, "", err
	}
	silkPath := silkTmp.Name()
	_ = silkTmp.Close()
	defer os.Remove(silkPath)

	silkCmd := exec.Command(silkEncoder, pcmPath, silkPath, "-Fs_API", "24000", "-rate", "24000", "-tencent")
	if silkOut, err := silkCmd.CombinedOutput(); err != nil {
		return 0, "", fmt.Errorf("silk_v3_encoder 转码失败: %w, output: %s", err, string(silkOut))
	}

	silkBytes, err := os.ReadFile(silkPath)
	if err != nil {
		return 0, "", fmt.Errorf("读取 SILK 文件失败: %w", err)
	}

	if n := len(silkBytes); n >= 2 && silkBytes[n-2] == 0xFF && silkBytes[n-1] == 0xFF {
		silkBytes = silkBytes[:n-2]
	}

	// 发送到微信
	formatSilk := int32(4)
	voice := &message.VoiceData{
		Media:    &message.Media{Data: silkBytes, Size: uint32(len(silkBytes))},
		Duration: uint32(durationMs),
		Format:   &formatSilk,
	}
	receiver := p.resolveReceiver(cleanTarget)
	msg := &message.Message{
		Type:     message.TypeVoice,
		Receiver: receiver,
		Content:  "[语音]",
		Data:     &message.Message_Voice{Voice: voice},
	}

	if _, err := p.message.Send(msg); err != nil {
		return 0, "", fmt.Errorf("发送微信语音失败: %w", err)
	}

	durationSec := (durationMs + 500) / 1000
	if durationSec <= 0 {
		durationSec = 1
	}

	// 缓存 WAV 到本地
	cacheDir := filepath.Join("data", "cache", "media")
	_ = os.MkdirAll(cacheDir, 0755)
	cacheWavPath := filepath.Join(cacheDir, fmt.Sprintf("sent_%d.wav", time.Now().UnixNano()))
	_ = os.WriteFile(cacheWavPath, wavBytes, 0644)

	// 本地播放用的 Data URI
	mediaURL := fmt.Sprintf("data:audio/wav;base64,%s", base64.StdEncoding.EncodeToString(wavBytes))

	// 记录到本地存储和历史库
	if p.chatEngine != nil {
		_ = p.chatEngine.RecordSentMediaMessage(cleanTarget, "voice", text, mediaURL, durationSec, p.getBotUsername())
	}

	slog.Info("[dashboard] 微信语音消息发送成功", "target", cleanTarget, "dur_sec", durationSec, "text", text)
	return durationSec, mediaURL, nil
}

func requestTTSAudio(endpoint, apiKey string, reqPayload map[string]any) ([]byte, error) {
	jsonBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("序列化 TTS 请求失败: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("创建 TTS 请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("调用 TTS 接口网络失败: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 TTS 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TTS 接口返回错误码 %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var ttsResp struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Audio struct {
					Data string `json:"data"`
				} `json:"audio"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(bodyBytes, &ttsResp); err != nil {
		return nil, fmt.Errorf("解析 TTS 响应失败: %w (raw=%s)", err, string(bodyBytes))
	}

	if ttsResp.Error != nil && ttsResp.Error.Message != "" {
		return nil, fmt.Errorf("TTS 合成报错: %s", ttsResp.Error.Message)
	}

	if len(ttsResp.Choices) == 0 || ttsResp.Choices[0].Message.Audio.Data == "" {
		return nil, errors.New("TTS 接口未返回有效的音频数据")
	}

	audioBase64 := ttsResp.Choices[0].Message.Audio.Data
	wavBytes, err := base64.StdEncoding.DecodeString(audioBase64)
	if err != nil {
		return nil, fmt.Errorf("Base64 解码 WAV 失败: %w", err)
	}
	return wavBytes, nil
}

