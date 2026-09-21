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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
)

// TTSConfig TTS 语音合成配置
type TTSConfig struct {
	Enable          bool   `toml:"enable" comment:"是否开启语音回复"`
	Model           string `toml:"model" comment:"TTS 模型：mimo-v2.5-tts-voicedesign 或 mimo-v2.5-tts"`
	VoiceDesign     string `toml:"voice_design" comment:"VoiceDesign 提示词，默认一个有磁性的开朗的男声"`
	Voice           string `toml:"voice,omitempty" comment:"预设发音人（用于 mimo-v2.5-tts）"`
	BaseURL         string `toml:"base_url,omitempty" comment:"TTS 接口地址，为空时复用 active provider"`
	APIKey          string `toml:"api_key,omitempty" comment:"TTS APIKey，为空时复用 active provider"`
	Mode            string `toml:"mode,omitempty" comment:"兼容保留"`
	SilkEncoderPath string `toml:"silk_encoder_path,omitempty" comment:"silk_v3_encoder 路径"`
	FFmpegPath      string `toml:"ffmpeg_path,omitempty" comment:"ffmpeg 路径"`
	FFprobePath     string `toml:"ffprobe_path,omitempty" comment:"ffprobe 路径"`
}

type mimoAudioParam struct {
	Format string `json:"format,omitempty"`
	Voice  string `json:"voice,omitempty"`
}

type mimoTTSRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Audio    *mimoAudioParam `json:"audio,omitempty"`
}

type mimoTTSResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content string `json:"content,omitempty"`
			Audio   struct {
				ID   string `json:"id"`
				Data string `json:"data"`
			} `json:"audio"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

var (
	voiceTagRegex      = regexp.MustCompile(`(?i)<voice(?:\s+design=["']([^"']*)["'])?\s*>([\s\S]*?)</voice>`)
	voiceDesignRegex   = regexp.MustCompile(`(?:用|模仿|学|扮演|装扮|假装|假扮|换成|换个|换)(?:一个)?([^，。！？\s]{1,20}?(?:音|声|声音|腔调|腔|声线|语调|口音|方言|角色|皮套))`)
	specificVoiceRegex = regexp.MustCompile(`(?:用|学|换|以|来个|整一个)(?:一个)?([^，。！？\s]{1,20}?(?:音|声|声音|腔调|腔|声线|语调|口音|方言))`)
)

// defaultTTSConfig 默认 TTS 配置
func defaultTTSConfig() TTSConfig {
	return TTSConfig{
		Enable:          true,
		Model:           "mimo-v2.5-tts-voicedesign",
		VoiceDesign:     "一位三十多岁的成熟男性朋友。嗓音富有磁性有质感，但音调自然轻松不沉闷。说话亲切温和、随性自如，语速轻快，带有自然的口语起伏和笑意，像日常随手拿起手机给朋友发微信语音闲聊。",
		Voice:           "白桦",
		SilkEncoderPath: "/Volumes/GuanMac/Code/Golem/tools/silk_v3_encoder",
		FFmpegPath:      "/opt/homebrew/bin/ffmpeg",
		FFprobePath:     "/opt/homebrew/bin/ffprobe",
	}
}

// stripVoiceTags 移除或提取 voice 标签内的文字
func stripVoiceTags(s string) string {
	return voiceTagRegex.ReplaceAllStringFunc(s, func(match string) string {
		sub := voiceTagRegex.FindStringSubmatch(match)
		if len(sub) >= 3 {
			return strings.TrimSpace(sub[2])
		}
		return ""
	})
}

// hasCustomVoiceRequest 检查用户是否特意要求装扮/模仿其他角色或指定特殊音色
func hasCustomVoiceRequest(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	// 明确的角色扮演/模仿动作词或知名音色类型
	roleKeywords := []string{
		"模仿", "扮演", "装扮", "假扮", "假装", "cos", "cosplay", "演一下", "演一个", "演个",
		"换声音", "换个声音", "换一种声音", "换音色", "换个音色", "换声线", "换个声线",
		"低音炮", "萝莉", "正太", "御姐", "大叔音", "大爷音", "太监音", "烟嗓",
		"夹子音", "娃娃音", "少女音", "少御音", "青年音", "童音", "方言", "东北话", "四川话", "粤语",
	}
	for _, kw := range roleKeywords {
		if strings.Contains(t, kw) {
			return true
		}
	}

	matches := specificVoiceRegex.FindStringSubmatch(t)
	if len(matches) >= 2 {
		val := strings.TrimSpace(matches[1])
		// 排除普通的 "语音", "声音", "普通话"
		if val != "" && val != "语音" && val != "声音" && val != "话" {
			return true
		}
	}
	return false
}

// hasVoiceIntent 检查用户文本是否表达了发语音的意图
func hasVoiceIntent(text string) bool {
	keywords := []string{
		"发语音", "发条语音", "发个语音", "用语音", "说段语音", "说条语音",
		"语音回复", "语音回答", "语音说", "语音发", "来条语音", "来段语音",
		"留条语音", "念一下", "读一下", "听你说话", "听你说", "说句话",
	}
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return hasCustomVoiceRequest(text)
}

// extractVoiceDesign 从用户文本中提取想要模仿的声音/声线
func extractVoiceDesign(text string) string {
	if !hasCustomVoiceRequest(text) {
		return ""
	}
	matches := voiceDesignRegex.FindStringSubmatch(text)
	if len(matches) >= 2 {
		desc := strings.TrimSpace(matches[1])
		if desc != "" && desc != "语音" && desc != "声音" {
			return desc
		}
	}
	roleKeywords := []string{
		"低音炮", "萝莉音", "正太音", "御姐音", "大叔音", "太监音", "烟嗓",
		"夹子音", "娃娃音", "少女音", "少御音", "青年音", "东北话", "四川话", "粤语",
	}
	for _, kw := range roleKeywords {
		if strings.Contains(text, kw) {
			return kw
		}
	}
	return ""
}

// isFillerText 判断外部文本是否仅仅是无实质内容的垫话（如“好的”、“语音来啦”等）
func isFillerText(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return true
	}
	fillers := []string{
		"好的", "好的！", "好的~", "好嘞", "没问题", "收到", "请听", "给你发语音了",
		"请听语音", "语音如下", "语音来啦", "听好咯", "这就来",
	}
	for _, f := range fillers {
		t = strings.TrimPrefix(t, f)
		t = strings.TrimSpace(t)
		t = strings.Trim(t, ":：，,！!~。. ")
	}
	return t == ""
}

// sanitizeVoiceText 清理语音文本格式并限制长度
func sanitizeVoiceText(text string) string {
	cleanText := strings.TrimSpace(text)
	cleanText = strings.ReplaceAll(cleanText, "\n\n", "，")
	cleanText = strings.ReplaceAll(cleanText, "\n", "，")
	runes := []rune(cleanText)
	if len(runes) > 300 {
		cleanText = string(runes[:300]) + "..."
	}
	return cleanText
}

// resolveTools 查找外部编解码工具路径
func (p *AiPlugin) resolveTools(cfg TTSConfig) (string, string, string, error) {
	ffmpeg := cfg.FFmpegPath
	if ffmpeg == "" {
		if p, err := exec.LookPath("ffmpeg"); err == nil {
			ffmpeg = p
		} else if _, err := os.Stat("/opt/homebrew/bin/ffmpeg"); err == nil {
			ffmpeg = "/opt/homebrew/bin/ffmpeg"
		} else {
			return "", "", "", errors.New("未找到 ffmpeg 可执行文件")
		}
	}

	ffprobe := cfg.FFprobePath
	if ffprobe == "" {
		if p, err := exec.LookPath("ffprobe"); err == nil {
			ffprobe = p
		} else if _, err := os.Stat("/opt/homebrew/bin/ffprobe"); err == nil {
			ffprobe = "/opt/homebrew/bin/ffprobe"
		} else {
			return "", "", "", errors.New("未找到 ffprobe 可执行文件")
		}
	}

	silkEncoder := cfg.SilkEncoderPath
	if silkEncoder == "" {
		if p, err := exec.LookPath("silk_v3_encoder"); err == nil {
			silkEncoder = p
		} else if _, err := os.Stat("/Volumes/GuanMac/Code/Golem/tools/silk_v3_encoder"); err == nil {
			silkEncoder = "/Volumes/GuanMac/Code/Golem/tools/silk_v3_encoder"
		} else {
			return "", "", "", errors.New("未找到 silk_v3_encoder 可执行文件")
		}
	}

	return ffmpeg, ffprobe, silkEncoder, nil
}

// requestTTSAudio 向小米 TTS 接口发送请求并提取 Base64 音频数据
func (p *AiPlugin) requestTTSAudio(endpoint, apiKey string, reqBody mimoTTSRequest) (string, error) {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化 TTS 请求失败: %w", err)
	}

	format := ""
	if reqBody.Audio != nil {
		format = reqBody.Audio.Format
	}
	slog.Info("[ai] 正在请求 TTS 语音合成", "model", reqBody.Model, "endpoint", endpoint, "format", format, "messages_count", len(reqBody.Messages))

	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("创建 TTS HTTP 请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("请求小米 TTS 接口失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取 TTS 响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("小米 TTS 接口返回异常状态码 %d: %s", resp.StatusCode, string(respBytes))
	}

	var ttsResp mimoTTSResponse
	if err := json.Unmarshal(respBytes, &ttsResp); err != nil {
		return "", fmt.Errorf("解析小米 TTS 响应 JSON 失败: %w (raw=%s)", err, string(respBytes))
	}

	if ttsResp.Error != nil && ttsResp.Error.Message != "" {
		return "", fmt.Errorf("小米 TTS 报错: %s", ttsResp.Error.Message)
	}

	if len(ttsResp.Choices) == 0 {
		return "", fmt.Errorf("小米 TTS 未返回 choices (HTTP %d, raw=%s)", resp.StatusCode, string(respBytes))
	}

	choice := ttsResp.Choices[0]
	if choice.Message.Audio.Data == "" {
		slog.Error("[ai] 小米 TTS 未返回有效音频", "model", reqBody.Model, "finish_reason", choice.FinishReason, "content", choice.Message.Content, "raw", string(respBytes))
		return "", fmt.Errorf("小米 TTS 未返回有效音频数据 (finish_reason=%s, content=%s)", choice.FinishReason, choice.Message.Content)
	}

	return choice.Message.Audio.Data, nil
}

// synthesizeSpeech 调用小米 MiMo TTS 合成语音并转码为腾讯微信 SILK 格式
func (p *AiPlugin) synthesizeSpeech(text string, customVoiceDesign string) ([]byte, int, error) {
	config := p.configSnapshot()
	ttsCfg := config.TTS

	ffmpeg, ffprobe, silkEncoder, err := p.resolveTools(ttsCfg)
	if err != nil {
		return nil, 0, err
	}

	// 确定 BaseURL 与 APIKey
	baseURL := strings.TrimSpace(ttsCfg.BaseURL)
	apiKey := strings.TrimSpace(ttsCfg.APIKey)
	if baseURL == "" || apiKey == "" {
		// 从当前活跃的 Provider 继承
		activeProvName := config.ActiveProvider
		if prov, ok := config.Providers[activeProvName]; ok && prov != nil {
			if baseURL == "" {
				baseURL = prov.BaseURL
			}
			if apiKey == "" {
				apiKey = prov.APIKey
			}
		}
	}
	if baseURL == "" || apiKey == "" {
		return nil, 0, errors.New("未配置 TTS API 地址或 Key（且当前 Provider 无可用配置）")
	}

	model := strings.TrimSpace(ttsCfg.Model)
	if model == "" {
		model = "mimo-v2.5-tts-voicedesign"
	}

	voiceName := strings.TrimSpace(ttsCfg.Voice)
	if voiceName == "" || voiceName == "default" || voiceName == "mimo_default" {
		voiceName = "白桦"
	}

	// 构造消息列表
	design := strings.TrimSpace(customVoiceDesign)
	if design == "" {
		design = strings.TrimSpace(ttsCfg.VoiceDesign)
	}
	if design == "" {
		design = "一位三十多岁的成熟男性朋友。嗓音富有磁性有质感，但音调自然轻松不沉闷。说话亲切温和、随性自如，语速轻快，带有自然的口语起伏和笑意，像日常随手拿起手机给朋友发微信语音闲聊。"
	}

	var messages []openAIMessage
	if design != "" {
		messages = []openAIMessage{
			{Role: "user", Content: design},
			{Role: "assistant", Content: text},
		}
	} else {
		messages = []openAIMessage{
			{Role: "assistant", Content: text},
		}
	}

	reqBody := mimoTTSRequest{
		Model:    model,
		Messages: messages,
		Audio: &mimoAudioParam{
			Format: "wav",
		},
	}
	if model == "mimo-v2.5-tts" {
		reqBody.Audio.Voice = voiceName
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/chat/completions"
	audioData, err := p.requestTTSAudio(endpoint, apiKey, reqBody)
	if err != nil && model == "mimo-v2.5-tts-voicedesign" {
		slog.Warn("[ai] voicedesign 合成失败，尝试降级到预设发音人 mimo-v2.5-tts", "err", err)
		fallbackReq := mimoTTSRequest{
			Model: "mimo-v2.5-tts",
			Messages: []openAIMessage{
				{Role: "assistant", Content: text},
			},
			Audio: &mimoAudioParam{
				Format: "wav",
				Voice:  voiceName,
			},
		}
		if fbData, fbErr := p.requestTTSAudio(endpoint, apiKey, fallbackReq); fbErr == nil {
			audioData = fbData
			err = nil
		}
	}
	if err != nil {
		return nil, 0, err
	}

	// 1. 解码 Base64 WAV
	wavBytes, err := base64.StdEncoding.DecodeString(audioData)
	if err != nil {
		return nil, 0, fmt.Errorf("Base64 解码音频失败: %w", err)
	}

	// 2. 写入临时 WAV 文件
	wavTmp, err := os.CreateTemp("", "mimo-tts-*.wav")
	if err != nil {
		return nil, 0, err
	}
	wavPath := wavTmp.Name()
	defer os.Remove(wavPath)
	if _, err := wavTmp.Write(wavBytes); err != nil {
		_ = wavTmp.Close()
		return nil, 0, err
	}
	_ = wavTmp.Close()

	// 3. 用 ffprobe 获取时长
	durCmd := exec.Command(ffprobe, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", wavPath)
	durOutput, err := durCmd.Output()
	durationMs := 1500
	if err == nil {
		if sec, parseErr := strconv.ParseFloat(strings.TrimSpace(string(durOutput)), 64); parseErr == nil {
			durationMs = int(sec * 1000)
			if durationMs < 1000 {
				durationMs = 1000
			}
		}
	}

	// 4. 自适应码率与预算控制（微信单条语音上传通道上限 ~28KB，超限直接报 -104 拒绝落库）
	const (
		minRate      = 8000
		maxRate      = 24000
		silkMaxBytes = 25000 // 预留安全余量至 25KB
	)

	rate := maxRate
	actualMs := durationMs
	if durationMs > 0 {
		if need := silkMaxBytes * 8 * 1000 / durationMs; need < rate {
			rate = need
		}
		if rate < minRate {
			rate = minRate
			actualMs = silkMaxBytes * 8 * 1000 / rate
		}
	}

	// 5. 用 ffmpeg 将 WAV 转为 24000Hz s16le PCM（超预算时 -t 截断，避免微信拒收）
	pcmTmp, err := os.CreateTemp("", "mimo-tts-*.pcm")
	if err != nil {
		return nil, 0, err
	}
	pcmPath := pcmTmp.Name()
	_ = pcmTmp.Close()
	defer os.Remove(pcmPath)

	ffArgs := []string{"-y", "-i", wavPath, "-ar", "24000", "-ac", "1", "-f", "s16le"}
	if actualMs < durationMs {
		ffArgs = append(ffArgs, "-t", fmt.Sprintf("%.3f", float64(actualMs)/1000.0))
	}
	ffArgs = append(ffArgs, pcmPath)

	ffCmd := exec.Command(ffmpeg, ffArgs...)
	if ffOut, err := ffCmd.CombinedOutput(); err != nil {
		return nil, 0, fmt.Errorf("ffmpeg 转 PCM 失败: %w, output: %s", err, string(ffOut))
	}

	// 6. 用 silk_v3_encoder 将 PCM 转为腾讯变体 SILK，使用自适应 rate
	silkTmp, err := os.CreateTemp("", "mimo-tts-*.silk")
	if err != nil {
		return nil, 0, err
	}
	silkPath := silkTmp.Name()
	_ = silkTmp.Close()
	defer os.Remove(silkPath)

	silkCmd := exec.Command(silkEncoder, pcmPath, silkPath, "-Fs_API", "24000", "-rate", strconv.Itoa(rate), "-tencent")
	if silkOut, err := silkCmd.CombinedOutput(); err != nil {
		return nil, 0, fmt.Errorf("silk_v3_encoder 转码失败: %w, output: %s", err, string(silkOut))
	}

	silkBytes, err := os.ReadFile(silkPath)
	if err != nil {
		return nil, 0, fmt.Errorf("读取 SILK 文件失败: %w", err)
	}

	// 去除尾部 0xFF 0xFF 修正
	if n := len(silkBytes); n >= 2 && silkBytes[n-2] == 0xFF && silkBytes[n-1] == 0xFF {
		silkBytes = silkBytes[:n-2]
	}

	slog.Info("[ai] 语音转码完成", "raw_dur_ms", durationMs, "final_dur_ms", actualMs, "rate_bps", rate, "silk_bytes", len(silkBytes))

	return silkBytes, actualMs, nil
}

// sendVoice 发送原生微信语音消息
func (p *AiPlugin) sendVoice(receiver *contact.Contact, silkData []byte, durationMs int) error {
	if p.message == nil {
		return errors.New("message ability is not injected")
	}
	if receiver == nil || strings.TrimSpace(receiver.GetUsername()) == "" {
		return errors.New("receiver is empty")
	}

	formatSilk := int32(4)
	voice := &message.VoiceData{
		Media:    &message.Media{Data: silkData, Size: uint32(len(silkData))},
		Duration: uint32(durationMs),
		Format:   &formatSilk,
	}
	msg := &message.Message{
		Type:     message.TypeVoice,
		Receiver: receiver,
		Content:  "[语音]",
		Data:     &message.Message_Voice{Voice: voice},
	}
	_, err := p.message.Send(msg)
	return err
}

// sendSplitText 分段发送纯文本消息（控制在1-2段以内，拉长两段之间的间隔）
func (p *AiPlugin) sendSplitText(receiver *contact.Contact, content string) error {
	rawParts := strings.Split(content, "\n\n")
	var parts []string
	for _, s := range rawParts {
		t := strings.TrimSpace(s)
		if t != "" {
			parts = append(parts, t)
		}
	}
	if len(parts) == 0 {
		return nil
	}

	// 严格控制在 1~2 段以内：超过 2 段的，将后续内容合并到第 2 段，避免多条刷屏
	if len(parts) > 2 {
		secondPart := strings.Join(parts[1:], "\n")
		parts = []string{parts[0], secondPart}
	}

	for i, t := range parts {
		if err := p.sendText(receiver, t); err != nil {
			return err
		}
		if i < len(parts)-1 {
			// 两段之间的中间打字间隔：更充足的真人打字呼吸感（2.5s ~ 4.5s）
			charCount := len([]rune(t))
			bubbleDelay := 2500*time.Millisecond + time.Duration(min(charCount, 50)*35)*time.Millisecond
			if bubbleDelay > 4500*time.Millisecond {
				bubbleDelay = 4500 * time.Millisecond
			}
			time.Sleep(bubbleDelay)
		}
	}
	return nil
}

// handleAIReply 综合处理文本与语音发送：发语音时不发相同文本，发文本时不发相同语音
func (p *AiPlugin) handleAIReply(receiver *contact.Contact, reply string, userText string) error {
	reply = stripThinkingContent(reply)
	if isLeakedReasoningOrRefusal(reply) {
		reply = getRandomSensualDeflection()
	}

	ttsCfg := p.configSnapshot().TTS

	// 1. 如果未启用 TTS，剥离 voice 标签后纯文本发送
	if !ttsCfg.Enable {
		clean := stripVoiceTags(reply)
		return p.sendSplitText(receiver, clean)
	}

	// 2. 检查大模型回复中是否包含 <voice> 标签
	matches := voiceTagRegex.FindAllStringSubmatchIndex(reply, -1)
	if len(matches) > 0 {
		// 大模型决定使用语音
		var nonVoiceParts []string
		lastIdx := 0
		type voiceTask struct {
			design string
			text   string
		}
		var tasks []voiceTask

		for _, match := range matches {
			startTag := match[0]
			endTag := match[1]
			if startTag > lastIdx {
				nonVoiceParts = append(nonVoiceParts, reply[lastIdx:startTag])
			}
			lastIdx = endTag

			var design string
			if match[2] >= 0 && match[3] >= 0 {
				design = reply[match[2]:match[3]]
			}
			var voiceText string
			if match[4] >= 0 && match[5] >= 0 {
				voiceText = reply[match[4]:match[5]]
			}
			tasks = append(tasks, voiceTask{design: strings.TrimSpace(design), text: strings.TrimSpace(voiceText)})
		}
		if lastIdx < len(reply) {
			nonVoiceParts = append(nonVoiceParts, reply[lastIdx:])
		}

		// 处理标签外部的文本：若有独立有意义文本则发送，若是套话垫话或与语音重复则不发送
		outsideText := strings.TrimSpace(strings.Join(nonVoiceParts, "\n"))
		if !isFillerText(outsideText) {
			_ = p.sendSplitText(receiver, outsideText)
			time.Sleep(300 * time.Millisecond)
		}

		// 合成并发送语音（不发送相同文本）
		userWantsCustomVoice := hasCustomVoiceRequest(userText)
		for _, task := range tasks {
			if task.text == "" {
				continue
			}
			finalDesign := task.design
			if !userWantsCustomVoice {
				// 除非用户特意要求装扮/模仿别的角色，否则严格忽略模型生成的临时 design，使用系统默认角色
				finalDesign = ""
			} else if finalDesign == "" {
				// 用户特意要求了角色/音色，但模型未输出 design，从用户文本提取
				finalDesign = extractVoiceDesign(userText)
			}

			cleanText := sanitizeVoiceText(task.text)
			logDesign := finalDesign
			if logDesign == "" {
				logDesign = "[默认配置角色] " + strings.TrimSpace(ttsCfg.VoiceDesign)
			}
			slog.Info("[ai] 模型决策发送语音", "text", cleanText, "voice_design", logDesign, "custom_requested", userWantsCustomVoice)
			silkData, durMs, err := p.synthesizeSpeech(cleanText, finalDesign)
			if err != nil {
				slog.Error("[ai] 语音合成失败，降级发送文本", "err", err)
				_ = p.sendText(receiver, task.text)
				continue
			}
			if err := p.sendVoice(receiver, silkData, durMs); err != nil {
				slog.Error("[ai] 发送语音失败，降级发送文本", "err", err)
				_ = p.sendText(receiver, task.text)
			}
		}
		return nil
	}

	// 3. 回复中未包含 <voice> 标签：检查用户是否要求发语音
	if hasVoiceIntent(userText) {
		voiceDesign := ""
		userWantsCustomVoice := hasCustomVoiceRequest(userText)
		if userWantsCustomVoice {
			voiceDesign = extractVoiceDesign(userText)
		}
		cleanText := sanitizeVoiceText(reply)
		logDesign := voiceDesign
		if logDesign == "" {
			logDesign = "[默认配置角色] " + strings.TrimSpace(ttsCfg.VoiceDesign)
		}
		slog.Info("[ai] 用户明确要求语音，转为语音发送", "voice_design", logDesign, "custom_requested", userWantsCustomVoice, "text_len", len(cleanText))
		silkData, durMs, err := p.synthesizeSpeech(cleanText, voiceDesign)
		if err != nil {
			slog.Error("[ai] 语音合成失败，降级发送文本", "err", err)
			return p.sendSplitText(receiver, reply)
		}
		if err := p.sendVoice(receiver, silkData, durMs); err != nil {
			slog.Error("[ai] 发送语音失败，降级发送文本", "err", err)
			return p.sendSplitText(receiver, reply)
		}
		// 用户要求发语音，成功发出语音后不发相同文本
		return nil
	}

	// 4. 用户未要求发语音且模型未加 voice 标签：仅发送纯文本，不发语音
	return p.sendSplitText(receiver, reply)
}

