package main

import (
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/sbgayhub/golem/sdk/cdn"
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

// DemosPlugin 娱乐功能插件
type DemosPlugin struct {
	plugin.ConfigAbility[Config]
	message message.Ability
	contact contact.Ability
	cdn     cdn.Ability
	client  *http.Client

	handlers map[string]handlerFunc
}

type handlerFunc func(receiver *contact.Contact, arg string) (bool, error)

func (p *DemosPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "demos",
		Author:      "Golem Team",
		Version:     "1.1.0",
		Description: "Demos 演示功能合集插件（包含各种第三方公共 API 实验性多媒体能力）",
		Priority:    0,
		Next:        false,
		AlwaysRun:   false,
	}
}

func (p *DemosPlugin) OnLoad() error {
	slog.Info("[demos] 插件加载成功", "video_native", p.Config.VideoNative, "max_list", p.Config.MaxList)
	return nil
}

func (p *DemosPlugin) OnUnload() error {
	slog.Info("[demos] 插件已卸载")
	return nil
}

func (p *DemosPlugin) GetSubscriptions() []string {
	return []string{message.TypeText.Topic}
}

func (p *DemosPlugin) sortedKeys() []string {
	keys := make([]string, 0, len(p.handlers))
	for k := range p.handlers {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] > keys[j]
	})
	return keys
}

func (p *DemosPlugin) OnEvent(e *plugin.Event) (bool, error) {
	msg := e.Payload.(*plugin.Event_Message).Message
	if msg == nil {
		return false, nil
	}

	text := strings.TrimSpace(msg.GetContent())
	if text == "" {
		return false, nil
	}

	receiver := p.contact.Get(e.GetSender())
	if receiver == nil {
		slog.Warn("[demos] 未找到接收者", "sender", e.GetSender())
		return false, nil
	}

	handled, err := p.dispatch(receiver, text)
	if err != nil {
		slog.Error("[demos] 处理命令失败", "text", text, "err", err)
		p.sendText(receiver, "哎呀，翻车了！这个功能暂时罢工了，请稍后再试试吧~")
		return true, nil
	}
	return handled, nil
}
