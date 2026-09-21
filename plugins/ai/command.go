package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sbgayhub/golem/sdk/plugin"
)

type aiGetCommand struct {
	_ struct{} `cmd:"ai get" help:"查看 AI 配置" usage:"/ai get" example:"/ai get"`
}

type aiSetCommand struct {
	_                struct{} `cmd:"ai set" help:"更新 AI 全局配置" usage:"/ai set [-p provider] [--fallback-provider provider] [-r reply_rate] [--max-context n] [--timeout n] [-s silence]" example:"/ai set -p deepseek -r 0.2\n/ai set -s true"`
	Provider         *string  `flag:"p,provider" help:"切换全局活动 provider 名称"`
	FallbackProvider *string  `flag:"fallback-provider" help:"切换全局备用 fallback provider 名称"`
	ReplyRate        *float64 `flag:"r,reply-rate" help:"普通消息回复概率，取值 0~1"`
	MaxContext       *int     `flag:"max-context" help:"每个会话最多保留的上下文消息数"`
	Timeout          *int     `flag:"timeout" help:"大模型请求超时缺省值，单位秒"`
	Silence          *bool    `flag:"s,silence" help:"全局静默模式，true 开启 / false 关闭"`
}

type aiPromptCommand struct {
	_       struct{} `cmd:"ai prompt" help:"切换或设置提示词" usage:"/ai prompt <name> [-p prompt]" example:"/ai prompt default\n/ai prompt roleplay -p \"你是一个微信聊天机器人\""`
	Name    string   `arg:"name" help:"提示词名称" required:"true"`
	Prompt  *string  `flag:"p,prompt" help:"提示词内容；提供后会新增或更新该提示词并切换到它"`
	Command *plugin.Command
}

type aiClearContextCommand struct {
	_       struct{} `cmd:"ai clear-context" help:"清理上下文" usage:"/ai clear-context [-t target]" example:"/ai clear-context\n/ai clear-context -t chatroom::123@chatroom"`
	Target  string   `flag:"t,target" help:"会话 key；不传则清理当前命令来源会话"`
	Command *plugin.Command
}

type aiSessionGetCommand struct {
	_       struct{} `cmd:"ai session-get" help:"查看会话配置" usage:"/ai session-get [-t target]" example:"/ai session-get\n/ai session-get -t chatroom:123@chatroom"`
	Target  string   `flag:"t,target" help:"会话 key；不传则查看当前会话"`
	Command *plugin.Command
}

type aiSessionSetCommand struct {
	_          struct{} `cmd:"ai session-set" help:"设置会话配置" usage:"/ai session-set [-t target] [-p provider] [--prompt name] [-r reply_rate] [-c max_context] [-s silence]" example:"/ai session-set -p deepseek -r 0.8\n/ai session-set -t chatroom:123@chatroom -s true"`
	Target     string   `flag:"t,target" help:"会话 key；不传则设置当前会话"`
	Provider   *string  `flag:"p,provider" help:"会话级 provider 名称"`
	Prompt     *string  `flag:",prompt" help:"会话级提示词名称"`
	ReplyRate  *float64 `flag:"r,reply-rate" help:"回复概率，取值 0~1"`
	MaxContext *int     `flag:"c,max-context" help:"上下文消息数"`
	Silence    *bool    `flag:"s,silence" help:"会话级静默模式，true 开启 / false 关闭"`
	Command    *plugin.Command
}

type aiSessionResetCommand struct {
	_       struct{} `cmd:"ai session-reset" help:"重置会话配置为全局默认" usage:"/ai session-reset [-t target]" example:"/ai session-reset\n/ai session-reset -t chatroom:123@chatroom"`
	Target  string   `flag:"t,target" help:"会话 key；不传则重置当前会话"`
	Command *plugin.Command
}

type aiSessionListCommand struct {
	_ struct{} `cmd:"ai session-list" help:"列出所有自定义会话配置" usage:"/ai session-list" example:"/ai session-list"`
}

type aiHelpCommand struct {
	_ struct{} `cmd:"ai help" help:"显示 AI 插件帮助" usage:"/ai help" example:"/ai help"`
}

type aiProviderAddCommand struct {
	_       struct{} `cmd:"ai provider-add" help:"新增或更新 Provider 预设" usage:"/ai provider-add <name> [-u url] [-k api_key] [-m model] [--timeout n]" example:"/ai provider-add deepseek -u https://api.deepseek.com/v1 -k sk-xxx -m deepseek-chat"`
	Name    string   `arg:"name" help:"provider 名称" required:"true"`
	BaseURL *string  `flag:"u,url" help:"OpenAI 兼容接口地址，例如 https://api.openai.com/v1"`
	APIKey  *string  `flag:"k,api-key" help:"接口密钥"`
	Model   *string  `flag:"m,model" help:"模型名称"`
	Timeout *int     `flag:"timeout" help:"请求超时秒数，0 表示使用全局缺省"`
}

type aiProviderRemoveCommand struct {
	_    struct{} `cmd:"ai provider-remove" help:"删除 Provider 预设" usage:"/ai provider-remove <name>" example:"/ai provider-remove deepseek"`
	Name string   `arg:"name" help:"provider 名称" required:"true"`
}

type aiProviderListCommand struct {
	_ struct{} `cmd:"ai provider-list" help:"列出所有 Provider 预设" usage:"/ai provider-list" example:"/ai provider-list"`
}

func registerCommands(p *AiPlugin) error {
	handlers := []func() error{
		func() error { return plugin.RegisterCommand(p.handleGet) },
		func() error { return plugin.RegisterCommand(p.handleSet) },
		func() error { return plugin.RegisterCommand(p.handlePrompt) },
		func() error { return plugin.RegisterCommand(p.handleClearContext) },
		func() error { return plugin.RegisterCommand(p.handleSessionGet) },
		func() error { return plugin.RegisterCommand(p.handleSessionSet) },
		func() error { return plugin.RegisterCommand(p.handleSessionReset) },
		func() error { return plugin.RegisterCommand(p.handleSessionList) },
		func() error { return plugin.RegisterCommand(p.handleProviderAdd) },
		func() error { return plugin.RegisterCommand(p.handleProviderRemove) },
		func() error { return plugin.RegisterCommand(p.handleProviderList) },
		//func() error { return plugin.RegisterCommand(p.handleHelp) },
	}
	for _, register := range handlers {
		if err := register(); err != nil {
			return err
		}
	}
	return nil
}

func (p *AiPlugin) handleGet(aiGetCommand) (string, error) {
	config := p.configSnapshot()
	providerLine := emptyDash(config.ActiveProvider)
	if config.ActiveProvider != "" {
		if prov, ok := config.Providers[config.ActiveProvider]; ok && prov != nil {
			providerLine = fmt.Sprintf("%s | model=%s url=%s", config.ActiveProvider, emptyDash(prov.Model), emptyDash(prov.BaseURL))
		}
	}
	return strings.Join([]string{
		"AI 配置：",
		fmt.Sprintf("active_provider=%s", providerLine),
		fmt.Sprintf("providers=%s", strings.Join(providerNames(config.Providers), ",")),
		fmt.Sprintf("active_prompt=%s", config.ActivePrompt),
		fmt.Sprintf("prompts=%s", strings.Join(promptNames(config.Prompts), ",")),
		fmt.Sprintf("reply_rate=%.4g", config.ReplyRate),
		fmt.Sprintf("max_context=%d", config.MaxContextMessages),
		fmt.Sprintf("timeout=%d", config.HTTPTimeoutSeconds),
		fmt.Sprintf("silence=%v", config.Silence),
		fmt.Sprintf("prompt=%s", emptyDash(activePromptContent(config))),
	}, "\n"), nil
}

func (p *AiPlugin) handleSet(cmd aiSetCommand) (string, error) {
	p.configMu.Lock()
	defer p.configMu.Unlock()

	if err := p.applySetCommandLocked(cmd); err != nil {
		return "", err
	}
	if err := p.SaveConfig(p); err != nil {
		return "", fmt.Errorf("保存 AI 配置失败: %w", err)
	}
	return "AI 配置已更新", nil
}

func (p *AiPlugin) applySetCommandLocked(cmd aiSetCommand) error {
	changed := false
	if cmd.Provider != nil {
		name := strings.TrimSpace(*cmd.Provider)
		if name == "" {
			return fmt.Errorf("provider 名称不能为空")
		}
		p.Config = normalizeConfigValue(p.Config)
		if _, ok := p.Config.Providers[name]; !ok {
			return fmt.Errorf("provider 不存在：%s（请先 /ai provider-add %s）", name, name)
		}
		p.Config.ActiveProvider = name
		changed = true
	}
	if cmd.FallbackProvider != nil {
		fb := strings.TrimSpace(*cmd.FallbackProvider)
		if fb != "" {
			p.Config = normalizeConfigValue(p.Config)
			if _, ok := p.Config.Providers[fb]; !ok {
				return fmt.Errorf("fallback provider 不存在：%s（请先 /ai provider-add %s）", fb, fb)
			}
		}
		p.Config.FallbackProvider = fb
		changed = true
	}
	if cmd.ReplyRate != nil {
		if *cmd.ReplyRate < 0 || *cmd.ReplyRate > 1 {
			return fmt.Errorf("reply_rate 必须在 0~1 之间")
		}
		p.Config.ReplyRate = *cmd.ReplyRate
		changed = true
	}
	if cmd.MaxContext != nil {
		if *cmd.MaxContext <= 0 {
			return fmt.Errorf("max_context 必须大于 0")
		}
		p.Config.MaxContextMessages = *cmd.MaxContext
		changed = true
	}
	if cmd.Timeout != nil {
		if *cmd.Timeout <= 0 {
			return fmt.Errorf("timeout 必须大于 0")
		}
		p.Config.HTTPTimeoutSeconds = *cmd.Timeout
		changed = true
	}
	if cmd.Silence != nil {
		p.Config.Silence = *cmd.Silence
		changed = true
	}
	if !changed {
		return fmt.Errorf("未提供要更新的配置")
	}
	p.Config = normalizeConfigValue(p.Config)
	return nil
}

func (p *AiPlugin) handlePrompt(cmd aiPromptCommand) (string, error) {
	p.configMu.Lock()
	defer p.configMu.Unlock()

	updated, err := p.applyPromptCommandLocked(cmd)
	if err != nil {
		return "", err
	}

	p.clearContext(commandSessionKey(cmd.Command))

	if err := p.SaveConfig(p); err != nil {
		return "", fmt.Errorf("保存 AI 配置失败: %w", err)
	}
	if updated {
		return fmt.Sprintf("提示词已保存并切换：%s", p.Config.ActivePrompt), nil
	}
	return fmt.Sprintf("已切换提示词：%s", p.Config.ActivePrompt), nil
}

func (p *AiPlugin) applyPromptCommandLocked(cmd aiPromptCommand) (bool, error) {
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		return false, fmt.Errorf("提示词名称不能为空")
	}
	p.Config = normalizeConfigValue(p.Config)
	if cmd.Prompt == nil {
		if _, ok := p.Config.Prompts[name]; !ok {
			return false, fmt.Errorf("提示词不存在：%s", name)
		}
		p.Config.ActivePrompt = name
		return false, nil
	}
	p.Config.Prompts[name] = strings.TrimSpace(*cmd.Prompt)
	p.Config.ActivePrompt = name
	p.Config = normalizeConfigValue(p.Config)
	return true, nil
}

func (p *AiPlugin) handleClearContext(cmd aiClearContextCommand) (string, error) {
	key := strings.TrimSpace(cmd.Target)
	if key == "" {
		key = commandSessionKey(cmd.Command)
	}
	if key == "" {
		return "", fmt.Errorf("无法确定要清理的会话")
	}
	p.clearContext(key)
	return fmt.Sprintf("上下文已清理：%s", key), nil
}

func (p *AiPlugin) handleSessionGet(cmd aiSessionGetCommand) (string, error) {
	key := strings.TrimSpace(cmd.Target)
	if key == "" {
		key = commandSessionKey(cmd.Command)
	}
	if key == "" {
		return "", fmt.Errorf("无法确定要查看的会话")
	}

	config := p.configSnapshot()
	sessionCfg := p.getSessionConfig(key)

	lines := []string{fmt.Sprintf("会话：%s", key)}

	// provider
	if sessionCfg != nil && sessionCfg.ActiveProvider != nil {
		lines = append(lines, fmt.Sprintf("active_provider=%s (会话)", *sessionCfg.ActiveProvider))
	} else {
		lines = append(lines, fmt.Sprintf("active_provider=%s (全局)", emptyDash(config.ActiveProvider)))
	}

	// 提示词
	if sessionCfg != nil && sessionCfg.ActivePrompt != nil {
		lines = append(lines, fmt.Sprintf("active_prompt=%s (会话)", *sessionCfg.ActivePrompt))
	} else {
		lines = append(lines, fmt.Sprintf("active_prompt=%s (全局)", config.ActivePrompt))
	}

	// 回复概率
	if sessionCfg != nil && sessionCfg.ReplyRate != nil {
		lines = append(lines, fmt.Sprintf("reply_rate=%.4g (会话)", *sessionCfg.ReplyRate))
	} else {
		lines = append(lines, fmt.Sprintf("reply_rate=%.4g (全局)", config.ReplyRate))
	}

	// 上下文长度
	if sessionCfg != nil && sessionCfg.MaxContextMessages != nil {
		lines = append(lines, fmt.Sprintf("max_context=%d (会话)", *sessionCfg.MaxContextMessages))
	} else {
		lines = append(lines, fmt.Sprintf("max_context=%d (全局)", config.MaxContextMessages))
	}

	// 静默
	if sessionCfg != nil && sessionCfg.Silence != nil {
		lines = append(lines, fmt.Sprintf("silence=%v (会话)", *sessionCfg.Silence))
	} else {
		lines = append(lines, fmt.Sprintf("silence=%v (全局)", config.Silence))
	}

	// 显示全局配置
	lines = append(lines, "---")
	lines = append(lines, fmt.Sprintf("全局 active_provider=%s", emptyDash(config.ActiveProvider)))
	lines = append(lines, fmt.Sprintf("全局 active_prompt=%s", config.ActivePrompt))
	lines = append(lines, fmt.Sprintf("全局 reply_rate=%.4g", config.ReplyRate))
	lines = append(lines, fmt.Sprintf("全局 max_context=%d", config.MaxContextMessages))
	lines = append(lines, fmt.Sprintf("全局 silence=%v", config.Silence))

	return strings.Join(lines, "\n"), nil
}

func (p *AiPlugin) handleSessionSet(cmd aiSessionSetCommand) (string, error) {
	key := strings.TrimSpace(cmd.Target)
	if key == "" {
		key = commandSessionKey(cmd.Command)
	}
	if key == "" {
		return "", fmt.Errorf("无法确定要设置的会话")
	}

	if cmd.Provider == nil && cmd.Prompt == nil && cmd.ReplyRate == nil && cmd.MaxContext == nil && cmd.Silence == nil {
		return "", fmt.Errorf("未提供要更新的配置")
	}

	// 验证参数
	if cmd.ReplyRate != nil && (*cmd.ReplyRate < 0 || *cmd.ReplyRate > 1) {
		return "", fmt.Errorf("reply_rate 必须在 0~1 之间")
	}
	if cmd.MaxContext != nil && *cmd.MaxContext <= 0 {
		return "", fmt.Errorf("max_context 必须大于 0")
	}
	config := p.configSnapshot()
	if cmd.Prompt != nil {
		if _, ok := config.Prompts[*cmd.Prompt]; !ok {
			return "", fmt.Errorf("提示词不存在：%s", *cmd.Prompt)
		}
	}
	if cmd.Provider != nil {
		name := strings.TrimSpace(*cmd.Provider)
		if name == "" {
			return "", fmt.Errorf("provider 名称不能为空")
		}
		if _, ok := config.Providers[name]; !ok {
			return "", fmt.Errorf("provider 不存在：%s", name)
		}
	}

	// 获取或创建会话配置
	p.configMu.Lock()
	defer p.configMu.Unlock()

	sessionCfg := p.getSessionConfig(key)
	if sessionCfg == nil {
		sessionCfg = &SessionConfig{}
	}

	// 更新字段
	if cmd.Provider != nil {
		name := strings.TrimSpace(*cmd.Provider)
		sessionCfg.ActiveProvider = &name
	}
	if cmd.Prompt != nil {
		prompt := strings.TrimSpace(*cmd.Prompt)
		sessionCfg.ActivePrompt = &prompt
	}
	if cmd.ReplyRate != nil {
		rate := *cmd.ReplyRate
		sessionCfg.ReplyRate = &rate
	}
	if cmd.MaxContext != nil {
		ctx := *cmd.MaxContext
		sessionCfg.MaxContextMessages = &ctx
	}
	if cmd.Silence != nil {
		silence := *cmd.Silence
		sessionCfg.Silence = &silence
	}

	p.setSessionConfig(key, sessionCfg)

	if err := p.SaveConfig(p); err != nil {
		return "", fmt.Errorf("保存配置失败: %w", err)
	}

	return fmt.Sprintf("会话配置已更新：%s", key), nil
}

func (p *AiPlugin) handleSessionReset(cmd aiSessionResetCommand) (string, error) {
	key := strings.TrimSpace(cmd.Target)
	if key == "" {
		key = commandSessionKey(cmd.Command)
	}
	if key == "" {
		return "", fmt.Errorf("无法确定要重置的会话")
	}

	p.configMu.Lock()
	defer p.configMu.Unlock()

	p.deleteSessionConfig(key)

	if err := p.SaveConfig(p); err != nil {
		return "", fmt.Errorf("保存配置失败: %w", err)
	}

	return fmt.Sprintf("会话配置已重置为全局默认：%s", key), nil
}

func (p *AiPlugin) handleSessionList(aiSessionListCommand) (string, error) {
	keys := p.listSessionKeys()
	if len(keys) == 0 {
		return "暂无自定义会话配置", nil
	}

	sort.Strings(keys)
	lines := []string{fmt.Sprintf("共 %d 个自定义会话配置：", len(keys))}

	for _, key := range keys {
		cfg := p.getSessionConfig(key)
		if cfg == nil {
			continue
		}

		parts := []string{key}
		if cfg.ActiveProvider != nil {
			parts = append(parts, fmt.Sprintf("provider=%s", *cfg.ActiveProvider))
		}
		if cfg.ActivePrompt != nil {
			parts = append(parts, fmt.Sprintf("prompt=%s", *cfg.ActivePrompt))
		}
		if cfg.ReplyRate != nil {
			parts = append(parts, fmt.Sprintf("rate=%.4g", *cfg.ReplyRate))
		}
		if cfg.MaxContextMessages != nil {
			parts = append(parts, fmt.Sprintf("ctx=%d", *cfg.MaxContextMessages))
		}
		if cfg.Silence != nil {
			parts = append(parts, fmt.Sprintf("silence=%v", *cfg.Silence))
		}

		lines = append(lines, strings.Join(parts, ", "))
	}

	return strings.Join(lines, "\n"), nil
}

func (p *AiPlugin) handleProviderAdd(cmd aiProviderAddCommand) (string, error) {
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		return "", fmt.Errorf("provider 名称不能为空")
	}
	if cmd.Timeout != nil && *cmd.Timeout < 0 {
		return "", fmt.Errorf("timeout 不能为负数")
	}

	p.configMu.Lock()
	defer p.configMu.Unlock()

	p.Config = normalizeConfigValue(p.Config)
	prov, ok := p.Config.Providers[name]
	if !ok || prov == nil {
		prov = &Provider{}
	}
	if cmd.BaseURL != nil {
		prov.BaseURL = strings.TrimSpace(*cmd.BaseURL)
	}
	if cmd.APIKey != nil {
		prov.APIKey = strings.TrimSpace(*cmd.APIKey)
	}
	if cmd.Model != nil {
		prov.Model = strings.TrimSpace(*cmd.Model)
	}
	if cmd.Timeout != nil {
		prov.HTTPTimeoutSeconds = *cmd.Timeout
	}
	p.Config.Providers[name] = prov

	setActive := false
	if strings.TrimSpace(p.Config.ActiveProvider) == "" {
		p.Config.ActiveProvider = name
		setActive = true
	}

	if err := p.SaveConfig(p); err != nil {
		return "", fmt.Errorf("保存配置失败: %w", err)
	}
	if setActive {
		return fmt.Sprintf("Provider 已新增并设为活动：%s", name), nil
	}
	return fmt.Sprintf("Provider 已新增/更新：%s", name), nil
}

func (p *AiPlugin) handleProviderRemove(cmd aiProviderRemoveCommand) (string, error) {
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		return "", fmt.Errorf("provider 名称不能为空")
	}

	p.configMu.Lock()
	defer p.configMu.Unlock()

	p.Config = normalizeConfigValue(p.Config)
	if _, ok := p.Config.Providers[name]; !ok {
		return "", fmt.Errorf("provider 不存在：%s", name)
	}
	delete(p.Config.Providers, name)
	clearedActive := false
	if p.Config.ActiveProvider == name {
		p.Config.ActiveProvider = ""
		clearedActive = true
	}

	if err := p.SaveConfig(p); err != nil {
		return "", fmt.Errorf("保存配置失败: %w", err)
	}
	if clearedActive {
		return fmt.Sprintf("Provider 已删除：%s（原为活动 provider，已清空，请 /ai set -p 重新指定）", name), nil
	}
	return fmt.Sprintf("Provider 已删除：%s", name), nil
}

func (p *AiPlugin) handleProviderList(aiProviderListCommand) (string, error) {
	config := p.configSnapshot()
	names := providerNames(config.Providers)
	if len(names) == 0 {
		return "暂无 Provider 预设，请使用 /ai provider-add <name> 新增", nil
	}
	lines := []string{fmt.Sprintf("共 %d 个 Provider（活动：%s，备用：%s）：", len(names), emptyDash(config.ActiveProvider), emptyDash(config.FallbackProvider))}
	for _, name := range names {
		prov := config.Providers[name]
		marker := ""
		if name == config.ActiveProvider {
			marker = " *"
		}
		fbInfo := ""
		if len(prov.FallbackModels) > 0 {
			fbInfo = fmt.Sprintf(" fallbacks=[%s]", strings.Join(prov.FallbackModels, ","))
		}
		lines = append(lines, fmt.Sprintf("%s%s | model=%s%s url=%s key=%s", name, marker, emptyDash(prov.Model), fbInfo, emptyDash(prov.BaseURL), maskSecret(prov.APIKey)))
	}
	return strings.Join(lines, "\n"), nil
}

func commandSessionKey(cmd *plugin.Command) string {
	if cmd == nil || cmd.GetSender() == nil {
		return ""
	}
	sender := cmd.GetSender()
	if sender.GetUsername() == "" {
		return ""
	}
	if sender.GetType() == contactTypeChatroom {
		return "chatroom:" + sender.GetUsername()
	}
	return "private:" + sender.GetUsername()
}

func maskSecret(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "-"
	}
	if len(secret) <= 8 {
		return "****"
	}
	return secret[:4] + "****" + secret[len(secret)-4:]
}

func emptyDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func promptNames(prompts map[string]string) []string {
	names := make([]string, 0, len(prompts))
	for name := range prompts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func providerNames(providers map[string]*Provider) []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
