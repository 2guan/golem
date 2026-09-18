package main

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/sbgayhub/golem/sdk/chatroom"
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

var prefixes = []string{"meme", "emoji", "表情"}

type Config struct {
	Url string
}

type MemePlugin struct {
	message  message.Ability
	contact  contact.Ability
	chatroom chatroom.Ability
	plugin.ConfigAbility[Config]
	mu    sync.RWMutex
	cache map[string]*memeInfo
	infos []*memeInfo
}

func (m *MemePlugin) OnLoad() error {
	if m.Config.Url == "" {
		slog.Warn("meme 插件未配置 Url，已跳过缓存加载（配置 Url 后可正常使用）")
		return nil
	}
	if err := m.loadCache(); err != nil {
		slog.Warn("meme 缓存加载失败", "err", err)
		return err
	}

	slog.Info("meme 缓存加载完成", "count", len(m.infos))
	return nil
}

func (m *MemePlugin) OnUnload() error {
	return nil
}

func (m *MemePlugin) OnEnable() error {
	return nil
}

func (m *MemePlugin) OnDisable() error {
	return nil
}

func (m *MemePlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "meme",
		Author:      "ovo",
		Version:     "v1.0.0",
		Description: "meme 表情包生成插件，支持生成各种表情包",
		Priority:    100,
		Next:        false,
		AlwaysRun:   false,
	}
}

func (m *MemePlugin) GetSubscriptions() []string {
	return []string{message.TypeText.Topic, message.TypeAppQuote.Topic}
}

func (m *MemePlugin) OnEvent(event *plugin.Event) (bool, error) {
	msg := event.Payload.(*plugin.Event_Message).Message
	if msg == nil {
		return false, nil
	}

	text := extractText(msg)
	if text == "" {
		return false, nil
	}

	prefix, args := matchPrefix(text)
	if prefix == "" {
		return false, nil
	}

	args = strings.TrimSpace(args)
	if idx := strings.Index(args, "@"); idx >= 0 {
		args = strings.TrimSpace(args[:idx])
	}
	if args == "" {
		m.sendText(event, "请输入表情名称，如: "+prefix+" 揍\n查看列表: "+prefix+" list")
		return true, nil
	}

	switch args {
	case "list":
		return m.handleList(event, prefix), nil
	case "reload":
		if err := m.loadCache(); err != nil {
			m.sendText(event, "缓存刷新失败: "+err.Error())
		} else {
			m.sendText(event, fmt.Sprintf("缓存已刷新，共 %d 个表情", len(m.infos)))
		}
		return true, nil
	}

	fields := strings.Fields(args)
	keyword := fields[0]
	textArgs := fields[1:]

	info := m.lookupByKeyword(keyword)
	if info == nil {
		m.sendText(event, fmt.Sprintf("未找到表情 [%s]，请使用 %s list 查看可用列表", keyword, prefix))
		return true, nil
	}

	target, sender := m.collectUsers(event, msg)

	images := make([]map[string]string, 0)

	switch {
	case info.Params.MaxImages == 0:
	case info.Params.MinImages >= 2:
		if target == nil {
			m.sendText(event, fmt.Sprintf("表情 [%s] 需要两张图片，请@或引用一个用户", keyword))
			return true, nil
		}
		img1, err := m.uploadImage(sender.Avatar)
		if err != nil {
			slog.Warn("上传发送者头像失败", "err", err)
			m.sendText(event, "上传头像失败，请稍后重试")
			return true, nil
		}
		img2, err := m.uploadImage(target.Avatar)
		if err != nil {
			slog.Warn("上传目标头像失败", "err", err)
			m.sendText(event, "上传头像失败，请稍后重试")
			return true, nil
		}
		images = []map[string]string{
			{"name": sender.Nickname, "id": img1},
			{"name": target.Nickname, "id": img2},
		}
	default:
		user := target
		if user == nil {
			user = sender
		}
		img, err := m.uploadImage(user.Avatar)
		if err != nil {
			slog.Warn("上传头像失败", "err", err)
			m.sendText(event, "上传头像失败，请稍后重试")
			return true, nil
		}
		images = []map[string]string{
			{"name": user.Nickname, "id": img},
		}
	}

	texts := make([]string, 0)
	if len(textArgs) > 0 {
		if info.Params.MinTexts > 0 && len(textArgs) < info.Params.MinTexts {
			m.sendText(event, fmt.Sprintf("[%s] 表情需要%d段文字参数", keyword, info.Params.MinTexts))
			return true, nil
		}
		if info.Params.MaxTexts > 0 && len(textArgs) > info.Params.MaxTexts {
			m.sendText(event, fmt.Sprintf("[%s] 表情最多需要%d段文字参数", keyword, info.Params.MaxTexts))
			return true, nil
		}
		texts = textArgs
	} else if len(info.Params.DefaultTexts) > 0 {
		texts = info.Params.DefaultTexts
	} else if info.Params.MinTexts > 0 {
		m.sendText(event, fmt.Sprintf("[%s] 表情需要%d段文字参数", keyword, info.Params.MinTexts))
		return true, nil
	}

	resultID, err := m.generateMeme(info.Key, images, texts)
	if err != nil {
		slog.Warn("生成 meme 失败", "key", info.Key, "err", err)
		m.sendText(event, fmt.Sprintf("生成表情 [%s] 失败: %s", keyword, err.Error()))
		return true, nil
	}

	data, err := m.downloadImage(resultID)
	if err != nil {
		slog.Warn("下载 meme 结果失败", "err", err)
		m.sendText(event, "下载表情失败，请稍后重试")
		return true, nil
	}

	const maxEmoticonSize = 500 * 1024
	if len(data) > maxEmoticonSize {
		compressed, err := compressImage(data, maxEmoticonSize)
		if err != nil {
			slog.Warn("压缩表情失败，使用原图发送图片", "size", len(data), "err", err)
		} else {
			slog.Info("表情压缩完成", "before", len(data), "after", len(compressed))
			data = compressed
		}
	}

	receiver := m.contact.Get(event.GetSender())
	if receiver == nil {
		slog.Warn("未找到接收者联系人")
		return false, nil
	}

	sendMsg := &message.Message{
		Content:  fmt.Sprintf("[%s] %s", prefix, keyword),
		Receiver: receiver,
		Type:     message.TypeEmoji,
		Data:     &message.Message_Emoji{Emoji: &message.EmojiData{Media: &message.Media{Data: data}}},
	}
	if _, err := m.message.Send(sendMsg); err != nil {
		slog.Warn("发送表情失败", "err", err)
		return false, nil
	}

	return true, nil
}

func main() {
	plugin.Start(&MemePlugin{})
}
