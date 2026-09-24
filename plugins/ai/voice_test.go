package main

import (
	"os"
	"os/exec"
	"testing"
)

func TestTranscodeVoiceToWav(t *testing.T) {
	// 用 ffmpeg 动态生成 1 秒的测试音频 (WAV)
	tmpIn, err := os.CreateTemp("", "test-voice-*.wav")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpIn.Name())
	_ = tmpIn.Close()

	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "sine=frequency=1000:duration=1", "-ar", "8000", "-ac", "1", tmpIn.Name())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg 生成测试音频失败: %v, out: %s", err, string(out))
	}

	rawBytes, err := os.ReadFile(tmpIn.Name())
	if err != nil {
		t.Fatalf("读取测试音频失败: %v", err)
	}

	p := &AiPlugin{}
	wavBytes, err := p.transcodeVoiceToWav(rawBytes)
	if err != nil {
		t.Fatalf("transcodeVoiceToWav 失败: %v", err)
	}

	if len(wavBytes) == 0 {
		t.Fatal("转码后的 WAV 字节为空")
	}

	// 验证标准 WAV 头部 "RIFF" ... "WAVE"
	if len(wavBytes) < 12 || string(wavBytes[0:4]) != "RIFF" || string(wavBytes[8:12]) != "WAVE" {
		t.Fatalf("转码结果不是有效的 WAV 格式: 长度=%d", len(wavBytes))
	}
}

func TestTranscodeSilkToWav(t *testing.T) {
	encoderPath := "/Volumes/GuanMac/Code/Golem/tools/silk_v3_encoder"
	if _, err := os.Stat(encoderPath); err != nil {
		t.Skip("未找到 silk_v3_encoder，跳过 SILK 回环测试")
	}

	// 1. 生成 1 秒 24000Hz PCM
	pcmTmp, err := os.CreateTemp("", "test-silk-*.pcm")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(pcmTmp.Name())
	_ = pcmTmp.Close()

	cmdPCM := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "sine=frequency=800:duration=1", "-ar", "24000", "-ac", "1", "-f", "s16le", pcmTmp.Name())
	if out, err := cmdPCM.CombinedOutput(); err != nil {
		t.Fatalf("生成测试 PCM 失败: %v, out: %s", err, string(out))
	}

	// 2. 用 silk_v3_encoder 编码为 SILK (tencent 变体)
	silkTmp, err := os.CreateTemp("", "test-silk-*.silk")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(silkTmp.Name())
	_ = silkTmp.Close()

	cmdEnc := exec.Command(encoderPath, pcmTmp.Name(), silkTmp.Name(), "-Fs_API", "24000", "-rate", "24000", "-tencent")
	if out, err := cmdEnc.CombinedOutput(); err != nil {
		t.Fatalf("silk 编码失败: %v, out: %s", err, string(out))
	}

	silkBytes, err := os.ReadFile(silkTmp.Name())
	if err != nil {
		t.Fatal(err)
	}

	// 3. 模拟微信入站语音（带 0x02 前导字节）
	wechatSilk := append([]byte{0x02}, silkBytes...)

	p := &AiPlugin{}
	wavBytes, err := p.transcodeVoiceToWav(wechatSilk)
	if err != nil {
		t.Fatalf("微信 SILK 解码转码 WAV 失败: %v", err)
	}

	if len(wavBytes) < 12 || string(wavBytes[0:4]) != "RIFF" || string(wavBytes[8:12]) != "WAVE" {
		t.Fatalf("解码转码结果不是有效的 WAV: len=%d", len(wavBytes))
	}
}
