package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sbgayhub/golem/sdk/cdn"
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

// SetuPlugin 色图插件
type SetuPlugin struct {
	plugin.ConfigAbility[Config]
	message message.Ability
	contact contact.Ability
	cdn     cdn.Ability
	client  *http.Client
}

// newHTTPClient 创建带自定义重定向处理的 HTTP 客户端
func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("过多重定向")
			}
			// 处理 Location 头中可能存在的引号
			loc := req.Response.Header.Get("Location")
			if loc != "" {
				// 去除引号
				loc = strings.Trim(loc, "'\"")
				if parsed, err := url.Parse(loc); err == nil {
					req.URL = req.URL.ResolveReference(parsed)
				}
			}
			return nil
		},
	}
}

// GetMetadata 返回插件元数据
func (p *SetuPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "setu",
		Author:      "Golem Team",
		Version:     "1.0.1",
		Description: "色图插件 - 提供各种图片和视频",
		Priority:    -100,
	}
}

func (p *SetuPlugin) OnLoad() error {
	slog.Info("[setu] 色图插件加载成功",
		"img_url", p.Config.ImgURL,
		"video_rate", p.Config.VideoRate,
	)
	return nil
}

func (p *SetuPlugin) OnUnload() error {
	slog.Info("[setu] 色图插件已卸载")
	return nil
}

// GetSubscriptions 订阅事件
func (p *SetuPlugin) GetSubscriptions() []string {
	return []string{
		message.TypeText.Topic,
	}
}

// OnEvent 处理事件
func (p *SetuPlugin) OnEvent(e *plugin.Event) (bool, error) {
	msg := e.Payload.(*plugin.Event_Message).Message
	if msg == nil {
		return false, nil
	}

	text := strings.TrimSpace(msg.GetContent())
	if text == "" {
		if td := msg.GetText(); td != nil {
			text = strings.TrimSpace(td.Content)
		}
	}
	if text == "" {
		return false, nil
	}

	// 剥离可能存在的 @前缀或后缀（如 "@肉丸叔叔\u2005来点帅哥"）
	text = cleanCommandText(text)
	if text == "" {
		return false, nil
	}

	// 获取接收者（支持群聊与私聊兜底）
	receiver := p.contact.Get(e.GetSender())
	if receiver == nil {
		if msg.Sender != nil && msg.Sender.GetUsername() == e.GetSender() {
			receiver = msg.Sender
		} else if msg.Receiver != nil && msg.Receiver.GetUsername() == e.GetSender() {
			receiver = msg.Receiver
		} else {
			receiver = &contact.Contact{
				Username: e.GetSender(),
			}
		}
	}

	// 匹配关键词
	switch text {
	case "setu帮助", "色图帮助":
		return p.handleHelp(receiver)
	case "plmm", "漂亮妹妹", "来点美女", "来点小姐姐", "来个美女", "来个小姐姐", "看美女", "看妹子":
		return p.handlePlmm(receiver)
	case "美女视频", "小姐姐视频", "来点美女视频", "来点小姐姐视频", "来个美女视频", "来个小姐姐视频", "看美女视频":
		return p.handleVideo(receiver, "美女", p.Config.ImgVideoURL, p.Config.ImgURL)
	case "来点黑丝", "来个黑丝", "看黑丝":
		return p.handleSiImage(receiver, "黑丝", p.Config.HeisiVideoURL, p.Config.HeisiURL)
	case "黑丝视频", "来点黑丝视频", "来个黑丝视频":
		return p.handleVideo(receiver, "黑丝", p.Config.HeisiVideoURL, p.Config.HeisiURL)
	case "来点白丝", "来个白丝", "看白丝":
		return p.handleSiImage(receiver, "白丝", p.Config.BaisiVideoURL, p.Config.BaisiURL)
	case "白丝视频", "来点白丝视频", "来个白丝视频":
		return p.handleVideo(receiver, "白丝", p.Config.BaisiVideoURL, p.Config.BaisiURL)
	case "看看腿", "来点腿", "看腿", "来个腿":
		return p.handleKkt(receiver)
	case "来点帅哥", "来个帅哥", "看帅哥", "发个帅哥":
		return p.handleBoy(receiver)
	case "帅哥视频", "来点帅哥视频", "来个帅哥视频", "看帅哥视频":
		return p.handleVideo(receiver, "帅哥", p.Config.BoyVideoURL, p.Config.BoyURL)
	}

	// 诊断：setu测cdn [url]，不填 url 则用默认猫图
	if text == "setu测cdn" || strings.HasPrefix(text, "setu测cdn ") {
		return p.handleDiagCdn(receiver, strings.TrimSpace(strings.TrimPrefix(text, "setu测cdn")))
	}

	// 前缀匹配：来点XX（搜索）
	if strings.HasPrefix(text, "来点") && len([]rune(text)) > 2 {
		keyword := string([]rune(text)[2:])
		return p.handleSearch(receiver, keyword)
	}

	return false, nil
}

// cleanCommandText 剥离群聊中可能存在的 @机器人的昵称 前缀与后缀
func cleanCommandText(content string) string {
	text := strings.TrimSpace(content)
	// 剥离开头的 @xxx（如 @肉丸叔叔\u2005）
	for strings.HasPrefix(text, "@") {
		idx := strings.IndexAny(text, " \t\r\n\u2005\u00a0:：,，")
		if idx > 0 {
			text = strings.TrimSpace(text[idx:])
			text = strings.TrimLeft(text, " :：,，\t\r\n\u2005\u00a0")
		} else {
			break
		}
	}
	// 剥离结尾紧跟的 @xxx
	if idx := strings.LastIndex(text, "@"); idx > 0 {
		text = strings.TrimSpace(text[:idx])
	}
	return strings.TrimSpace(text)
}

// handleDiagCdn 诊断用：走与 sendImage 相同的 p.cdn.UploadImage 路径，显式回执成/败（不降级吞错）。
// 用途：和 hermes 切到 message.Send 之后的发图做对照，确认 cdn 这条路是否自愈。
// imgURL 为空时用默认猫图，便于直接发“setu测cdn”快速复测。
func (p *SetuPlugin) handleDiagCdn(receiver *contact.Contact, imgURL string) (bool, error) {
	const defaultURL = "https://cdn2.thecatapi.com/images/42r.jpg"
	if strings.TrimSpace(imgURL) == "" {
		imgURL = defaultURL
	}
	data, err := p.downloadMedia(imgURL)
	if err != nil {
		p.sendText(receiver, "setu测cdn：下载失败 "+err.Error()+" url="+imgURL)
		return true, nil
	}
	if _, err := p.cdn.UploadImage(receiver.GetUsername(), bytes.NewReader(data)); err != nil {
		slog.Error("[setu] 诊断发图 cdn.UploadImage 失败", "url", imgURL, "err", err)
		p.sendText(receiver, "setu测cdn：cdn 上传失败 "+err.Error()+" url="+imgURL)
		return true, nil
	}
	p.sendText(receiver, fmt.Sprintf("setu测cdn：成功直发图（%d 字节，走 cdn.UploadImage）url=%s", len(data), imgURL))
	return true, nil
}
