package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sbgayhub/golem/sdk/cdn"
	"github.com/sbgayhub/golem/sdk/chatroom"
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

//go:embed web/*
var webFS embed.FS

func main() {
	plugin.Start(&DashboardPlugin{
		ConfigAbility: plugin.ConfigAbility[Config]{
			Config: Config{
				Host:    "0.0.0.0",
				Port:    8899,
				DataDir: "data",
			},
		},
	})
}

type DashboardPlugin struct {
	plugin.ConfigAbility[Config]

	contact  contact.Ability
	chatroom chatroom.Ability
	message  message.Ability
	cdn      cdn.Ability
	caller   plugin.CallerAbility

	mu         sync.Mutex
	store      *StoreManager
	logEngine  *LogEngine
	chatEngine *ChatEngine
	configMgr  *ConfigManager
	aiService  *AIService
	httpServer *http.Server
	startTime  time.Time
}

func (p *DashboardPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "dashboard",
		Author:      "golem",
		Version:     "1.0.0",
		Description: "Golem Web 管理控制台：参数配置、会话专属人设策略、全模态聊天审计、AI 智能画像与运行日志检索中心",
		Priority:    100,
		Next:        true,
		AlwaysRun:   true,
	}
}

func (p *DashboardPlugin) OnLoad() error {
	return p.startServer()
}

func (p *DashboardPlugin) OnUnload() error {
	return p.stopServer()
}

func (p *DashboardPlugin) OnEnable() error {
	return p.startServer()
}

func (p *DashboardPlugin) OnDisable() error {
	return p.stopServer()
}

func (p *DashboardPlugin) startServer() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.httpServer != nil {
		return nil
	}

	p.startTime = time.Now()

	// 路径解析
	dataDir := "data"
	dashboardJSON := filepath.Join(dataDir, "dashboard.json")
	globalConfigTOML := filepath.Join(dataDir, "config.toml")
	pluginsConfigTOML := filepath.Join("plugins", "config.toml")
	golemLogFile := "golem.log"
	aiHistoryFile := filepath.Join(dataDir, "ai_history.json")
	statisticsDBFile := filepath.Join("plugins", "statistics", "statistics.db")
	sentStoreFile := filepath.Join(dataDir, "dashboard_sent.json")

	// 初始化各服务引擎
	store, err := NewStoreManager(dashboardJSON)
	if err != nil {
		return fmt.Errorf("初始化 dashboard store 失败: %w", err)
	}
	p.store = store

	p.logEngine = NewLogEngine(golemLogFile)
	p.chatEngine = NewChatEngine(aiHistoryFile, statisticsDBFile, sentStoreFile)
	p.configMgr = NewConfigManager(globalConfigTOML, pluginsConfigTOML)
	p.aiService = NewAIService(p.caller, pluginsConfigTOML)

	// 配置监听端口
	host := "0.0.0.0"
	if p.Config.Host != "" {
		host = p.Config.Host
	}
	port := 8899
	if p.Config.Port > 0 {
		port = p.Config.Port
	}

	mux := http.NewServeMux()

	// 注册公开认证接口
	mux.HandleFunc("/api/auth/login", p.handleLogin)

	// 注册受保护接口
	mux.HandleFunc("/api/auth/profile", p.AuthMiddleware(p.handleProfile))
	mux.HandleFunc("/api/auth/password", p.AuthMiddleware(p.handleChangePassword))
	mux.HandleFunc("/api/overview", p.AuthMiddleware(p.handleOverview))
	mux.HandleFunc("/api/config/global", p.AuthMiddleware(p.handleGlobalConfig))
	mux.HandleFunc("/api/config/plugins", p.AuthMiddleware(p.handlePluginsConfig))
	mux.HandleFunc("/api/config/plugins/toggle", p.AuthMiddleware(p.handlePluginToggle))
	mux.HandleFunc("/api/config/plugins/save", p.AuthMiddleware(p.handlePluginSave))
	mux.HandleFunc("/api/targets", p.AuthMiddleware(p.handleTargets))
	mux.HandleFunc("/api/targets/save", p.AuthMiddleware(p.handleTargetSave))
	mux.HandleFunc("/api/targets/delete", p.AuthMiddleware(p.handleTargetDelete))
	mux.HandleFunc("/api/chats/sessions", p.AuthMiddleware(p.handleChatSessions))
	mux.HandleFunc("/api/chats/messages", p.AuthMiddleware(p.handleChatMessages))
	mux.HandleFunc("/api/chats/send", p.AuthMiddleware(p.handleChatSend))
	mux.HandleFunc("/api/chats/send_image", p.AuthMiddleware(p.handleChatSendImage))
	mux.HandleFunc("/api/chats/send_voice", p.AuthMiddleware(p.handleChatSendVoice))
	mux.HandleFunc("/api/chats/tts_config", p.AuthMiddleware(p.handleGetTTSConfig))
	mux.HandleFunc("/api/chats/search", p.AuthMiddleware(p.handleChatSearch))
	mux.HandleFunc("/api/chats/clear_context", p.AuthMiddleware(p.handleChatClearContext))
	mux.HandleFunc("/api/chats/media", p.handleChatMedia)
	mux.HandleFunc("/api/ai/analyze", p.AuthMiddleware(p.handleAIAnalyze))
	mux.HandleFunc("/api/logs", p.AuthMiddleware(p.handleLogs))
	mux.HandleFunc("/api/logs/tags", p.AuthMiddleware(p.handleLogsTags))
	mux.HandleFunc("/api/logs/export", p.AuthMiddleware(p.handleLogsExport))

	// 静态 SPA 前端路由
	subFS, err := fs.Sub(webFS, "web")
	if err != nil {
		return fmt.Errorf("加载嵌入静态资源失败: %w", err)
	}
	fileServer := http.FileServer(http.FS(subFS))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		// 如果文件存在则直接提供，否则重定向到 index.html 实现 SPA
		f, err := subFS.Open(strings.TrimPrefix(r.URL.Path, "/"))
		if err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// 兜底返回 index.html
		indexContent, err := webFS.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexContent)
	})

	addr := fmt.Sprintf("%s:%d", host, port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
	}
	p.httpServer = server

	go func() {
		slog.Info("[dashboard] Golem 管理控制台启动成功", "url", fmt.Sprintf("http://127.0.0.1:%d", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("[dashboard] 控制台 HTTP 服务异常退出", "err", err)
		}
	}()

	return nil
}

func (p *DashboardPlugin) stopServer() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.httpServer == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := p.httpServer.Shutdown(ctx)
	p.httpServer = nil
	slog.Info("[dashboard] 控制台 HTTP 服务已关闭")
	return err
}
