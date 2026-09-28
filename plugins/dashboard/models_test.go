package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelsConfig_CRUD_And_ExcludeAIFromPlugins(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "models_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.toml")
	initialTOML := `
[ai]
enable = true
mode = 'blacklist'

[ai.config]
active_provider = 'xiaomi'
fallback_provider = 'gemini'
provider_order = ['xiaomi', 'gemini']

[ai.config.providers.xiaomi]
base_url = 'https://token-plan-cn.xiaomimimo.com/v1'
api_key = 'test-xiaomi-key'
model = 'mimo-v2.6-flash'

[ai.config.providers.gemini]
base_url = 'https://generativelanguage.googleapis.com/v1beta/openai'
api_key = 'test-gemini-key'
model = 'gemini-3.5-flash-lite'
fallback_models = ['gemma-4-31b-it']

[dashboard]
enable = true
`
	if err := os.WriteFile(configPath, []byte(initialTOML), 0644); err != nil {
		t.Fatalf("写入测试配置文件失败: %v", err)
	}

	cm := NewConfigManager(filepath.Join(tempDir, "global.toml"), configPath)

	// 1. 验证 ListPlugins 不包含 ai
	plugins, err := cm.ListPlugins()
	if err != nil {
		t.Fatalf("ListPlugins 失败: %v", err)
	}
	for _, p := range plugins {
		if p.Name == "ai" {
			t.Errorf("ListPlugins 不应该包含 ai 插件，期望被排除")
		}
	}

	// 2. 验证 ReadModelsConfig
	resp, err := cm.ReadModelsConfig()
	if err != nil {
		t.Fatalf("ReadModelsConfig 失败: %v", err)
	}
	if resp.ActiveProvider != "xiaomi" {
		t.Errorf("期望 active_provider 为 xiaomi, 实际为 %s", resp.ActiveProvider)
	}
	if resp.FallbackProvider != "gemini" {
		t.Errorf("期望 fallback_provider 为 gemini, 实际为 %s", resp.FallbackProvider)
	}
	if len(resp.ProviderOrder) != 2 || resp.ProviderOrder[0] != "xiaomi" || resp.ProviderOrder[1] != "gemini" {
		t.Errorf("期望 provider_order 为 [xiaomi, gemini], 实际为 %v", resp.ProviderOrder)
	}
	if len(resp.Presets) == 0 {
		t.Errorf("期望 Presets 不为空")
	}

	// 3. 验证 SaveModelsConfig (调整排序并增加 deepseek)
	newOrder := []string{"deepseek", "xiaomi", "gemini"}
	newProviders := map[string]ProviderConfigItem{
		"deepseek": {
			Name:        "deepseek",
			DisplayName: "深度求索",
			BaseURL:     "https://api.deepseek.com",
			APIKey:      "test-deepseek-key",
			Model:       "deepseek-chat",
		},
		"xiaomi": {
			Name:        "xiaomi",
			DisplayName: "小米 MiMo",
			BaseURL:     "https://token-plan-cn.xiaomimimo.com/v1",
			APIKey:      "test-xiaomi-key",
			Model:       "mimo-v2.6-flash",
		},
		"gemini": {
			Name:        "gemini",
			DisplayName: "谷歌 Gemini",
			BaseURL:     "https://generativelanguage.googleapis.com/v1beta/openai",
			APIKey:      "test-gemini-key",
			Model:       "gemini-3.5-flash-lite",
		},
	}
	err = cm.SaveModelsConfig(SaveModelsRequest{
		ProviderOrder: newOrder,
		Providers:     newProviders,
	})
	if err != nil {
		t.Fatalf("SaveModelsConfig 失败: %v", err)
	}

	// 4. 再次读取验证更新后排序与 active_provider
	updated, err := cm.ReadModelsConfig()
	if err != nil {
		t.Fatalf("重新读取模型配置失败: %v", err)
	}
	if updated.ActiveProvider != "deepseek" {
		t.Errorf("期望新 active_provider 为 deepseek, 实际为 %s", updated.ActiveProvider)
	}
	if updated.FallbackProvider != "xiaomi" {
		t.Errorf("期望新 fallback_provider 为 xiaomi, 实际为 %s", updated.FallbackProvider)
	}
	if len(updated.ProviderOrder) != 3 || updated.ProviderOrder[0] != "deepseek" {
		t.Errorf("期望首位调用顺位为 deepseek, 实际为 %v", updated.ProviderOrder)
	}
}

func TestGuessDisplayName(t *testing.T) {
	tests := []struct {
		key      string
		baseURL  string
		model    string
		expected string
	}{
		{"xiaomi", "https://token-plan-cn.xiaomimimo.com/v1", "mimo-v2.6-flash", "小米 (Xiaomi MiMo)"},
		{"gemini", "https://generativelanguage.googleapis.com/v1beta/openai", "gemini-3.5-flash-lite", "谷歌 (Google Gemini)"},
		{"deepseek", "https://api.deepseek.com", "deepseek-chat", "深度求索 (DeepSeek)"},
		{"openai", "https://api.openai.com/v1", "gpt-4o", "OpenAI (ChatGPT)"},
		{"my_custom_model", "https://custom.api/v1", "custom-1", "my_custom_model"},
	}

	for _, tt := range tests {
		actual := guessDisplayName(tt.key, tt.baseURL, tt.model)
		if actual != tt.expected {
			t.Errorf("guessDisplayName(%s) = %s, expected %s", tt.key, actual, tt.expected)
		}
	}
}

func TestModelsAPI_Handlers(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "models_api_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.toml")
	initialTOML := `
[ai.config]
active_provider = 'xiaomi'
provider_order = ['xiaomi']

[ai.config.providers.xiaomi]
base_url = 'https://token-plan-cn.xiaomimimo.com/v1'
api_key = 'test-key'
model = 'mimo-v2.6-flash'
`
	_ = os.WriteFile(configPath, []byte(initialTOML), 0644)

	cm := NewConfigManager(filepath.Join(tempDir, "global.toml"), configPath)
	plugin := &DashboardPlugin{
		configMgr: cm,
	}

	// 1. GET /api/models
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	w := httptest.NewRecorder()
	plugin.handleModelsConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望状态码 200, 实际为 %d", w.Code)
	}
	var resp ModelsConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("解码响应失败: %v", err)
	}
	if resp.ActiveProvider != "xiaomi" {
		t.Errorf("期望 active_provider 为 xiaomi, 实际为 %s", resp.ActiveProvider)
	}

	// 2. POST /api/models/save
	saveBody, _ := json.Marshal(SaveModelsRequest{
		ProviderOrder: []string{"xiaomi"},
		Providers: map[string]ProviderConfigItem{
			"xiaomi": {
				Name:    "xiaomi",
				BaseURL: "https://token-plan-cn.xiaomimimo.com/v1",
				APIKey:  "new-key",
				Model:   "mimo-v2.6-flash",
			},
		},
	})
	postReq := httptest.NewRequest(http.MethodPost, "/api/models/save", bytes.NewReader(saveBody))
	postW := httptest.NewRecorder()
	plugin.handleModelsSave(postW, postReq)

	if postW.Code != http.StatusOK {
		t.Fatalf("保存期望状态码 200, 实际为 %d: %s", postW.Code, postW.Body.String())
	}
}

func TestTTSConfig_Read_And_Save(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tts_config_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.toml")
	initialTOML := `
[ai.config.tts]
enable = true
base_url = 'https://token-plan-cn.xiaomimimo.com/v1'
api_key = 'test-tts-key'
model = 'mimo-v2.5-tts-voicedesign'
voice = '白桦'
voice_design = '测试声音人设描述'
silk_encoder_path = '/usr/local/bin/silk_v3_encoder'
ffmpeg_path = '/usr/local/bin/ffmpeg'
ffprobe_path = '/usr/local/bin/ffprobe'
`
	if err := os.WriteFile(configPath, []byte(initialTOML), 0644); err != nil {
		t.Fatalf("写入测试配置文件失败: %v", err)
	}

	cm := NewConfigManager(filepath.Join(tempDir, "global.toml"), configPath)

	// 1. 读取 TTS 配置
	cfg, err := cm.ReadTTSConfig()
	if err != nil {
		t.Fatalf("ReadTTSConfig 失败: %v", err)
	}
	if !cfg.Enable {
		t.Errorf("期望 Enable 为 true")
	}
	if cfg.Model != "mimo-v2.5-tts-voicedesign" {
		t.Errorf("期望 model 为 mimo-v2.5-tts-voicedesign, 实际为 %s", cfg.Model)
	}
	if cfg.Voice != "白桦" {
		t.Errorf("期望 voice 为 白桦, 实际为 %s", cfg.Voice)
	}
	if len(cfg.Presets) == 0 {
		t.Errorf("期望 Presets 不为空")
	}

	// 2. 修改并保存 TTS 配置
	saveReq := SaveTTSConfigRequest{
		Enable:          false,
		BaseURL:         "https://api.openai.com/v1",
		APIKey:          "new-openai-tts-key",
		Model:           "tts-1",
		Voice:           "alloy",
		VoiceDesign:     "",
		SilkEncoderPath: "/tools/silk_v3_encoder",
		FFmpegPath:      "/opt/homebrew/bin/ffmpeg",
		FFprobePath:     "/opt/homebrew/bin/ffprobe",
	}
	if err := cm.SaveTTSConfig(saveReq); err != nil {
		t.Fatalf("SaveTTSConfig 失败: %v", err)
	}

	// 3. 重新读取并验证
	updated, err := cm.ReadTTSConfig()
	if err != nil {
		t.Fatalf("重新读取 TTS 配置失败: %v", err)
	}
	if updated.Enable {
		t.Errorf("期望 Enable 为 false")
	}
	if updated.Model != "tts-1" {
		t.Errorf("期望 model 为 tts-1, 实际为 %s", updated.Model)
	}
	if updated.Voice != "alloy" {
		t.Errorf("期望 voice 为 alloy, 实际为 %s", updated.Voice)
	}
	if updated.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("期望 BaseURL 更新为 OpenAI 端点, 实际为 %s", updated.BaseURL)
	}
}

func TestTTSAPI_Handlers(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tts_api_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.toml")
	initialTOML := `
[ai.config.tts]
enable = true
base_url = 'https://token-plan-cn.xiaomimimo.com/v1'
api_key = 'test-tts-key'
model = 'mimo-v2.5-tts-voicedesign'
voice = '白桦'
`
	_ = os.WriteFile(configPath, []byte(initialTOML), 0644)

	cm := NewConfigManager(filepath.Join(tempDir, "global.toml"), configPath)
	plugin := &DashboardPlugin{
		configMgr: cm,
	}

	// 1. GET /api/models/tts
	req := httptest.NewRequest(http.MethodGet, "/api/models/tts", nil)
	w := httptest.NewRecorder()
	plugin.handleGetFullTTSConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/models/tts 期望 200, 实际为 %d", w.Code)
	}
	var resp TTSFullConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("解码响应失败: %v", err)
	}
	if resp.Model != "mimo-v2.5-tts-voicedesign" {
		t.Errorf("期望 model 为 mimo-v2.5-tts-voicedesign, 实际为 %s", resp.Model)
	}

	// 2. POST /api/models/tts/save
	saveBody, _ := json.Marshal(SaveTTSConfigRequest{
		Enable:      true,
		BaseURL:     "https://token-plan-cn.xiaomimimo.com/v1",
		APIKey:      "updated-key",
		Model:       "mimo-v2.5-tts",
		Voice:       "云雀",
		VoiceDesign: "温柔大方",
	})
	postReq := httptest.NewRequest(http.MethodPost, "/api/models/tts/save", bytes.NewReader(saveBody))
	postW := httptest.NewRecorder()
	plugin.handleSaveTTSConfig(postW, postReq)

	if postW.Code != http.StatusOK {
		t.Fatalf("POST /api/models/tts/save 期望 200, 实际为 %d: %s", postW.Code, postW.Body.String())
	}

	// 3. 验证再次读取
	req2 := httptest.NewRequest(http.MethodGet, "/api/models/tts", nil)
	w2 := httptest.NewRecorder()
	plugin.handleGetFullTTSConfig(w2, req2)
	var resp2 TTSFullConfigResponse
	_ = json.NewDecoder(w2.Body).Decode(&resp2)
	if resp2.Voice != "云雀" {
		t.Errorf("期望 voice 更新为 云雀, 实际为 %s", resp2.Voice)
	}
}

func TestTTSAPI_UploadSample(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tts_upload_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.toml")
	_ = os.WriteFile(configPath, []byte("[ai.config.tts]\nenable=true\n"), 0644)

	cm := NewConfigManager(filepath.Join(tempDir, "global.toml"), configPath)
	plugin := &DashboardPlugin{
		configMgr: cm,
	}
	plugin.Config.DataDir = tempDir

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "test_sample.wav")
	if err != nil {
		t.Fatalf("创建表单文件失败: %v", err)
	}
	_, _ = part.Write([]byte("RIFF1234WAVEfmt test audio data"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/models/tts/upload_sample", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	plugin.handleUploadTTSSample(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("上传期望 200, 实际为 %d: %s", w.Code, w.Body.String())
	}

	var resp UploadSampleAudioResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("解码上传响应失败: %v", err)
	}
	if resp.Filename != "test_sample.wav" {
		t.Errorf("期望 filename 为 test_sample.wav, 实际为 %s", resp.Filename)
	}
	if !strings.HasPrefix(resp.DataURI, "data:audio/wav;base64,") {
		t.Errorf("期望 DataURI 以 data:audio/wav;base64, 开头, 实际为 %s", resp.DataURI)
	}
	if _, err := os.Stat(resp.Path); err != nil {
		t.Errorf("期望上传的文件存在于磁盘: %v", err)
	}
}

func TestPromptsConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")
	initialTOML := `
[ai]
enable = true

[ai.config]
active_prompt = "default"

[ai.config.prompts]
default = "你是一个默认助手"
custom_persona = "你是一个贴心伙伴助手"
`
	if err := os.WriteFile(configPath, []byte(initialTOML), 0644); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	cm := NewConfigManager(filepath.Join(tempDir, "global.toml"), configPath)

	// 1. 读取提示词配置
	resp, err := cm.ReadPromptsConfig()
	if err != nil {
		t.Fatalf("ReadPromptsConfig 失败: %v", err)
	}
	if resp.ActivePrompt != "default" {
		t.Errorf("期望 ActivePrompt 为 default, 实际为 %s", resp.ActivePrompt)
	}
	if resp.Prompts["custom_persona"] != "你是一个贴心伙伴助手" {
		t.Errorf("期望 custom_persona 提示词匹配, 实际为 %s", resp.Prompts["custom_persona"])
	}

	// 2. 保存提示词配置
	saveReq := SavePromptsConfigRequest{
		ActivePrompt: "custom_persona",
		Prompts: map[string]string{
			"default":        "更新后的默认助手",
			"custom_persona": "新助手提示词设定",
			"test":           "测试人设",
		},
	}
	if err := cm.SavePromptsConfig(saveReq); err != nil {
		t.Fatalf("SavePromptsConfig 失败: %v", err)
	}

	// 3. 再次读取验证
	updated, err := cm.ReadPromptsConfig()
	if err != nil {
		t.Fatalf("读取更新后的配置失败: %v", err)
	}
	if updated.ActivePrompt != "custom_persona" {
		t.Errorf("期望 ActivePrompt 为 custom_persona, 实际为 %s", updated.ActivePrompt)
	}
	if updated.Prompts["test"] != "测试人设" {
		t.Errorf("期望新增 test 人设, 实际为 %s", updated.Prompts["test"])
	}
}

func TestPromptsHTTPHandlers(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")
	initialTOML := `
[ai]
enable = true

[ai.config]
active_prompt = "default"

[ai.config.prompts]
default = "默认提示词"
`
	if err := os.WriteFile(configPath, []byte(initialTOML), 0644); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	cm := NewConfigManager(filepath.Join(tempDir, "global.toml"), configPath)
	plugin := &DashboardPlugin{
		configMgr: cm,
	}

	// 1. 测试 GET /api/models/prompts
	req := httptest.NewRequest(http.MethodGet, "/api/models/prompts", nil)
	w := httptest.NewRecorder()
	plugin.handleGetPromptsConfig(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/models/prompts 期望 200, 实际为 %d: %s", w.Code, w.Body.String())
	}

	var resp PromptsConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("解码响应失败: %v", err)
	}
	if resp.ActivePrompt != "default" {
		t.Errorf("期望 default, 实际为 %s", resp.ActivePrompt)
	}

	// 2. 测试 POST /api/models/prompts/save
	saveBody, _ := json.Marshal(SavePromptsConfigRequest{
		ActivePrompt: "new_active",
		Prompts: map[string]string{
			"default":    "更新后的默认",
			"new_active": "新激活提示词",
		},
	})
	postReq := httptest.NewRequest(http.MethodPost, "/api/models/prompts/save", bytes.NewReader(saveBody))
	postW := httptest.NewRecorder()
	plugin.handleSavePromptsConfig(postW, postReq)
	if postW.Code != http.StatusOK {
		t.Fatalf("POST /api/models/prompts/save 期望 200, 实际为 %d: %s", postW.Code, postW.Body.String())
	}

	// 3. 验证再次 GET
	req2 := httptest.NewRequest(http.MethodGet, "/api/models/prompts", nil)
	w2 := httptest.NewRecorder()
	plugin.handleGetPromptsConfig(w2, req2)
	var resp2 PromptsConfigResponse
	_ = json.NewDecoder(w2.Body).Decode(&resp2)
	if resp2.ActivePrompt != "new_active" {
		t.Errorf("期望 new_active, 实际为 %s", resp2.ActivePrompt)
	}
	if resp2.Prompts["new_active"] != "新激活提示词" {
		t.Errorf("期望新激活提示词匹配, 实际为 %s", resp2.Prompts["new_active"])
	}
}




