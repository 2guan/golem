package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/sbgayhub/golem/sdk/message"
)

const (
	defaultSilkDecoderPath = "/Volumes/GuanMac/Code/Golem/tools/silk-v3-decoder/silk/decoder"
	defaultFFmpegPath      = "/opt/homebrew/bin/ffmpeg"
)

type voiceXMLAttrs struct {
	AesKey   string `xml:"aeskey,attr"`
	VoiceURL string `xml:"voiceurl,attr"`
	Size     uint32 `xml:"length,attr"`
	Duration uint32 `xml:"voicelength,attr"`
	BufID    uint32 `xml:"bufid,attr"`
}

type voiceXML struct {
	XMLName xml.Name      `xml:"msg"`
	Voice   voiceXMLAttrs `xml:"voicemsg"`
	Voice2  voiceXMLAttrs `xml:"voice"`
}

func parseVoiceInfo(rawXML string) voiceXMLAttrs {
	rawXML = strings.TrimSpace(rawXML)
	if rawXML == "" {
		return voiceXMLAttrs{}
	}
	var temp voiceXML
	if err := xml.Unmarshal([]byte(rawXML), &temp); err != nil {
		return voiceXMLAttrs{}
	}
	attrs := temp.Voice
	if attrs.AesKey == "" && attrs.VoiceURL == "" && attrs.Size == 0 {
		attrs = temp.Voice2
	}
	return voiceXMLAttrs{
		AesKey:   strings.TrimSpace(attrs.AesKey),
		VoiceURL: strings.TrimSpace(attrs.VoiceURL),
		Size:     attrs.Size,
		Duration: attrs.Duration,
		BufID:    attrs.BufID,
	}
}

// downloadVoice 从微信消息中获取入站语音二进制数据（优先 ImgBuf 原始缓冲，兜底 CDN）
func (p *AiPlugin) downloadVoice(msg *message.Message) ([]byte, error) {
	if msg == nil {
		return nil, errors.New("消息为空")
	}

	raw := msg.GetRaw()

	// 1. 优先从微信协议原始包中的 ImgBuf 提取（微信协议大部分语音消息直接附带在同步包内）
	if buf := rawImageBuffer(raw); len(buf) > 0 {
		slog.Info("[ai] 成功从协议 ImgBuf 提取语音数据", "bytes", len(buf))
		return buf, nil
	}

	// 2. 如果 ImgBuf 为空，尝试解析 XML 并走 CDN 下载
	info := parseVoiceInfo(rawContentValue(raw))
	if info.VoiceURL == "" {
		info = parseVoiceInfo(msg.GetContent())
	}

	if p.cdn != nil && info.VoiceURL != "" && info.AesKey != "" {
		rc, err := p.cdn.DownloadImage(info.VoiceURL, info.AesKey)
		if err == nil && rc != nil {
			buf, readErr := io.ReadAll(rc)
			_ = rc.Close()
			if readErr == nil && len(buf) > 0 {
				slog.Info("[ai] 成功通过 CDN 下载语音数据", "bytes", len(buf))
				return buf, nil
			}
		} else {
			slog.Debug("[ai] CDN 下载语音失败", "err", err)
		}
	}

	// 3. 尝试调用底层 message.Download
	if p.message != nil {
		if rc, err := p.message.Download(msg); err == nil && rc != nil {
			buf, readErr := io.ReadAll(rc)
			_ = rc.Close()
			if readErr == nil && len(buf) > 0 {
				slog.Info("[ai] 成功通过 message.Download 获取语音数据", "bytes", len(buf))
				return buf, nil
			}
		}
	}

	return nil, errors.New("未能获取到语音数据（ImgBuf、CDN 与 Download 均无有效数据）")
}

// transcodeVoiceToWav 将微信原始音频（SILK v3 / AMR 等）转码为标准 16kHz 单声道 PCM WAV
func (p *AiPlugin) transcodeVoiceToWav(rawAudio []byte) ([]byte, error) {
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
	if ttsCfg := p.configSnapshot().TTS; ttsCfg.FFmpegPath != "" {
		if _, err := os.Stat(ttsCfg.FFmpegPath); err == nil {
			ffmpegPath = ttsCfg.FFmpegPath
		}
	}
	if _, err := os.Stat(ffmpegPath); err != nil {
		if path, err := exec.LookPath("ffmpeg"); err == nil {
			ffmpegPath = path
		}
	}

	// 1. 判断是否为微信 SILK 格式
	// 微信 SILK 通常以 0x02 开头，随后跟着 "#!SILK_V3"，或者直接以 "#!SILK_V3" 开头
	isSilk := false
	silkData := rawAudio
	if bytes.HasPrefix(rawAudio, []byte("\x02#!SILK_V3")) {
		isSilk = true
		silkData = rawAudio[1:] // 剥离微信前导 0x02
	} else if bytes.HasPrefix(rawAudio, []byte("#!SILK_V3")) {
		isSilk = true
	} else if len(rawAudio) > 1 && rawAudio[0] == 0x02 {
		// 部分老版本或变体仅带 0x02
		isSilk = true
		silkData = rawAudio[1:]
	}

	if isSilk && decoderPath != "" {
		wav, err := decodeSilkWithBinary(silkData, decoderPath, ffmpegPath)
		if err == nil && len(wav) > 0 {
			return wav, nil
		}
		slog.Warn("[ai] SILK 专有解码失败，尝试直接使用 ffmpeg 兜底", "err", err)
	}

	// 2. 兜底直接使用 ffmpeg 转码（支持 AMR、MP3、AAC、WAV 等通用格式）
	return transcodeDirectWithFFmpeg(rawAudio, ffmpegPath)
}

func decodeSilkWithBinary(silkData []byte, decoderPath, ffmpegPath string) ([]byte, error) {
	// 创建临时 silk 文件
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

	// 调用 silk decoder 解码为 24000Hz s16le PCM
	cmdDec := exec.Command(decoderPath, silkPath, pcmPath, "-Fs_API", "24000", "-quiet")
	if out, err := cmdDec.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("silk decoder 解码失败: %w, output: %s", err, string(out))
	}

	// 调用 ffmpeg 将 24000Hz 单声道 PCM 转码为 16000Hz 单声道标准 WAV
	cmdFF := exec.Command(ffmpegPath, "-y", "-f", "s16le", "-ar", "24000", "-ac", "1", "-i", pcmPath, "-ar", "16000", "-ac", "1", "-acodec", "pcm_s16le", wavPath)
	if out, err := cmdFF.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg 转码 WAV 失败: %w, output: %s", err, string(out))
	}

	wavBytes, err := os.ReadFile(wavPath)
	if err != nil {
		return nil, fmt.Errorf("读取转码后 WAV 失败: %w", err)
	}
	return wavBytes, nil
}

func transcodeDirectWithFFmpeg(audioData []byte, ffmpegPath string) ([]byte, error) {
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

	cmd := exec.Command(ffmpegPath, "-y", "-i", inPath, "-ar", "16000", "-ac", "1", "-acodec", "pcm_s16le", wavPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg 直接转码失败: %w, output: %s", err, string(out))
	}

	return os.ReadFile(wavPath)
}
