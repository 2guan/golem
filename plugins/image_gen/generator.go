package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type imageGenerationRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Image  string `json:"image,omitempty"`
}

type imageGenerationResponse struct {
	Created int64 `json:"created,omitempty"`
	Data    []struct {
		URL string `json:"url"`
	} `json:"data,omitempty"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

type ImageGenerator struct {
	cfg    Config
	client *http.Client
}

func newImageGenerator(cfg Config) *ImageGenerator {
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 90
	}
	return &ImageGenerator{
		cfg: cfg,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

// GenerateImage 请求 Seedream 生成图片，并返回转码后的 JPEG 二进制流与 MIME 类型
func (g *ImageGenerator) GenerateImage(prompt string, refImage string) ([]byte, string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, "", fmt.Errorf("生图提示词不能为空")
	}

	model := strings.TrimSpace(g.cfg.Model)
	if model == "" {
		model = "seedream-4.7"
	}

	reqBody := imageGenerationRequest{
		Model:  model,
		Prompt: prompt,
		Image:  strings.TrimSpace(refImage),
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", fmt.Errorf("构造生图请求失败: %w", err)
	}

	baseURL := strings.TrimRight(strings.TrimSpace(g.cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://any2api.ai.guantools.top"
	}
	apiURL := baseURL + "/v1/images/generations"

	slog.Info("[image_gen] 发起生图请求", "model", model, "prompt", prompt, "has_ref_image", refImage != "")

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, "", fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if g.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(g.cfg.APIKey))
	}

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, "", fmt.Errorf("生图接口请求超时或失败: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("读取生图响应失败: %w", err)
	}

	var genResp imageGenerationResponse
	if err := json.Unmarshal(respBytes, &genResp); err != nil {
		return nil, "", fmt.Errorf("解析生图响应 JSON 失败: %w (原始回包: %s)", err, string(respBytes))
	}

	if genResp.Error != nil && genResp.Error.Message != "" {
		return nil, "", fmt.Errorf("生图服务报错: %s", genResp.Error.Message)
	}

	if len(genResp.Data) == 0 || strings.TrimSpace(genResp.Data[0].URL) == "" {
		return nil, "", fmt.Errorf("生图接口未返回有效图片地址: %s", string(respBytes))
	}

	targetURL := strings.TrimSpace(genResp.Data[0].URL)
	if strings.HasPrefix(targetURL, "/") {
		targetURL = baseURL + targetURL
	}

	slog.Info("[image_gen] 生图成功，开始下载图片", "url", targetURL)

	// 下载图片媒体数据
	imgData, err := g.downloadFile(targetURL)
	if err != nil {
		return nil, "", fmt.Errorf("下载生成图片失败: %w", err)
	}

	// 转码为 JPEG 以保证微信 100% 完美展示
	jpegData, err := g.transcodeToJPEG(imgData)
	if err != nil {
		slog.Warn("[image_gen] 转码 JPEG 失败，回退使用原始图像", "err", err)
		return imgData, "image/webp", nil
	}

	return jpegData, "image/jpeg", nil
}

func (g *ImageGenerator) downloadFile(fileURL string) ([]byte, error) {
	dlReq, err := http.NewRequest(http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	dlReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")

	dlResp, err := g.client.Do(dlReq)
	if err != nil {
		return nil, err
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载图片 HTTP 状态码错误: %d", dlResp.StatusCode)
	}

	return io.ReadAll(dlResp.Body)
}

func (g *ImageGenerator) transcodeToJPEG(input []byte) ([]byte, error) {
	ffmpegBin := strings.TrimSpace(g.cfg.FFmpegPath)
	if ffmpegBin == "" {
		ffmpegBin = "ffmpeg"
	}

	cmd := exec.Command(ffmpegBin,
		"-hide_banner",
		"-loglevel", "error",
		"-i", "pipe:0",
		"-f", "image2",
		"-c:v", "mjpeg",
		"-q:v", "2",
		"pipe:1",
	)

	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg 转码执行失败: %w, stderr: %s", err, stderr.String())
	}

	if stdout.Len() == 0 {
		return nil, fmt.Errorf("ffmpeg 输出为空")
	}

	return stdout.Bytes(), nil
}
