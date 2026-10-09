package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sbgayhub/golem/sdk/cdn"
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

type recentImageInfo struct {
	Data      []byte
	MimeType  string
	Time      time.Time
	SenderID  string
}

type ImageGenPlugin struct {
	plugin.ConfigAbility[Config]
	contact contact.Ability
	message message.Ability
	cdn     cdn.Ability
	caller  plugin.CallerAbility

	genMu     sync.Mutex
	generator *ImageGenerator

	imgMu        sync.Mutex
	recentImages map[string]*recentImageInfo
}

func (p *ImageGenPlugin) getGenerator() *ImageGenerator {
	p.genMu.Lock()
	defer p.genMu.Unlock()
	if p.generator == nil {
		cfg := p.Config
		if cfg.APIKey == "" {
			cfg = defaultConfig()
		}
		p.generator = newImageGenerator(cfg)
	}
	return p.generator
}

func (p *ImageGenPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "image_gen",
		Author:      "Golem Team",
		Version:     "1.0.0",
		Description: "AI 图像生成与自拍插件 - 基于 Seedream 模型，支持文字生图、图生图与写实生活自拍",
		Priority:    10,
	}
}

func (p *ImageGenPlugin) OnLoad() error {
	p.genMu.Lock()
	p.generator = newImageGenerator(p.Config)
	if p.recentImages == nil {
		p.recentImages = make(map[string]*recentImageInfo)
	}
	p.genMu.Unlock()
	slog.Info("[image_gen] 插件加载完成", "model", p.Config.Model, "base_url", p.Config.BaseURL)
	return nil
}

func (p *ImageGenPlugin) OnUnload() error {
	slog.Info("[image_gen] 插件已卸载")
	return nil
}

func (p *ImageGenPlugin) OnEnable() error {
	return nil
}

func (p *ImageGenPlugin) OnDisable() error {
	return nil
}

func (p *ImageGenPlugin) GetSubscriptions() []string {
	return []string{
		message.TypeText.Topic,
		message.TypeAppQuote.Topic,
		message.TypeImage.Topic,
	}
}

func (p *ImageGenPlugin) GetCapabilities() []string {
	return []string{
		"image.generate",
		"image.selfie",
	}
}

// OnCall 供其他插件（如 ai 插件）调用
func (p *ImageGenPlugin) OnCall(capability string, args map[string]string) (string, []byte, error) {
	gen := p.getGenerator()
	receiver := strings.TrimSpace(args["receiver"])

	switch capability {
	case "image.generate":
		prompt := strings.TrimSpace(args["prompt"])
		if prompt == "" {
			return "", nil, errors.New("缺少 prompt 参数")
		}
		refImage := strings.TrimSpace(args["image"])
		// 如果指定了 ref="selfie"，使用肉丸头像作为参考图
		if refImage == "selfie" {
			refImage = p.getAvatarURL()
		}

		jpegData, mime, err := gen.GenerateImage(prompt, refImage)
		if err != nil {
			return "", nil, err
		}

		mediaURL, _ := p.saveGeneratedImage(jpegData)

		if receiver != "" {
			if err := p.sendImageBytes(receiver, jpegData); err != nil {
				return "", nil, fmt.Errorf("发送图片到 %s 失败: %w", receiver, err)
			}
			return "text", []byte(mediaURL), nil
		}
		return mime, jpegData, nil

	case "image.selfie":
		scene := strings.TrimSpace(args["scene"])
		avatarURL := strings.TrimSpace(args["avatar_url"])
		if avatarURL == "" {
			avatarURL = p.getAvatarURL()
		}

		jpegData, mime, _, err := gen.GenerateSelfie(scene, avatarURL)
		if err != nil {
			return "", nil, err
		}

		mediaURL, _ := p.saveGeneratedImage(jpegData)

		if receiver != "" {
			if err := p.sendImageBytes(receiver, jpegData); err != nil {
				return "", nil, fmt.Errorf("发送自拍到 %s 失败: %w", receiver, err)
			}
			return "text", []byte(mediaURL), nil
		}
		return mime, jpegData, nil

	default:
		return "", nil, fmt.Errorf("不支持的能力: %s", capability)
	}
}

// OnEvent 处理微信事件
func (p *ImageGenPlugin) OnEvent(e *plugin.Event) (bool, error) {
	payload, ok := e.Payload.(*plugin.Event_Message)
	if !ok || payload.Message == nil {
		return false, nil
	}
	msg := payload.Message

	senderID := e.GetSender()
	sessionKey := senderID

	// 1. 如果是图片消息，缓存图片数据供后续“生图/画图”参考
	if msg.GetType() != nil && msg.GetType().GetCode() == message.TypeImage.Code {
		data, mime, err := p.downloadWeChatImage(msg)
		if err == nil && len(data) > 0 {
			p.cacheRecentImage(sessionKey, msg.GetSender().GetUsername(), data, mime)
			slog.Debug("[image_gen] 缓存会话最近图片", "session", sessionKey, "size", len(data))
		}
		return false, nil
	}

	// 2. 处理文本或引用消息
	text := cleanText(p.extractMessageText(msg))
	if text == "" {
		return false, nil
	}

	receiver := p.resolveReceiver(e, msg)
	if receiver == nil {
		return false, nil
	}

	// 帮助命令
	if text == "生图帮助" || text == "画图帮助" || text == "自拍帮助" {
		helpText := "🎨 【AI 生图与自拍使用说明】\n\n" +
			"1. 📷 发张自拍：\n" +
			"   • 发送「自拍」、「来张自拍」、「拍张照」\n" +
			"   • 指定地点：如「来张在故宫的自拍」、「发张在海边的自拍」\n\n" +
			"2. 🖌️ 文字生图：\n" +
			"   • 发送「生图 <画面描述>」或「画图 <描述>」\n" +
			"   • 例如：「画图 一只在阳光下伸懒腰的可爱三花猫，写实摄影」\n\n" +
			"3. 🖼️ 参考图生图 / 改图：\n" +
			"   • 引用一张图片并发送「生图/画图 <要求>」\n" +
			"   • 或直接发图后，接着发送「画图 把它变成赛博朋克风格」"
		p.sendText(receiver, helpText)
		return true, nil
	}

	speakerName := ""
	if msg.Member != nil {
		speakerName = msg.Member.DisplayName
		if speakerName == "" {
			speakerName = msg.Member.Nickname
		}
	}
	if speakerName == "" && msg.Sender != nil {
		speakerName = msg.Sender.Nickname
	}

	// 自拍触发判断
	if isSelfie, scene := p.matchSelfieTrigger(text); isSelfie {
		go p.handleSelfieCommand(receiver, scene, text, speakerName)
		return true, nil
	}

	// 生图 / 画图触发判断
	if isGen, prompt := p.matchImageGenTrigger(text); isGen {
		// 检查是否有参考图（引用消息中的图片，或会话最近 5 分钟内发送的图片）
		refImageB64 := p.findReferenceImage(msg, sessionKey)
		go p.handleImageGenCommand(receiver, prompt, refImageB64, text, speakerName)
		return true, nil
	}

	return false, nil
}

func (p *ImageGenPlugin) matchSelfieTrigger(text string) (bool, string) {
	t := strings.TrimSpace(text)

	// 精确匹配
	exacts := []string{
		"自拍", "来张自拍", "发张自拍", "看自拍", "看下自拍", "看看自拍",
		"拍张照", "拍个照", "拍个照片", "发个自拍", "来个自拍", "发一张自拍",
		"给我发张自拍", "发你的照片", "看你的照片", "看看你的照片", "发张你的照片",
	}
	for _, ex := range exacts {
		if t == ex {
			return true, ""
		}
	}

	// 包含自拍关键词且带有场景描述（如："来张在故宫的自拍"、"发张海边的自拍"、"在天安门拍张照"）
	if strings.Contains(t, "自拍") || strings.Contains(t, "拍张照") || strings.Contains(t, "拍个照") {
		// 排除长句或反问句中非触发的情景（如 "你觉得自拍好看吗"）
		if strings.HasPrefix(t, "来张") || strings.HasPrefix(t, "发张") || strings.HasPrefix(t, "拍张") ||
			strings.HasPrefix(t, "来个") || strings.HasPrefix(t, "发个") || strings.HasPrefix(t, "拍个") ||
			strings.HasPrefix(t, "发一张") || strings.HasPrefix(t, "给我来张") {
			// 提取场景
			scene := t
			scene = strings.TrimPrefix(scene, "给我来张")
			scene = strings.TrimPrefix(scene, "发一张")
			scene = strings.TrimPrefix(scene, "来张")
			scene = strings.TrimPrefix(scene, "发张")
			scene = strings.TrimPrefix(scene, "拍张")
			scene = strings.TrimPrefix(scene, "来个")
			scene = strings.TrimPrefix(scene, "发个")
			scene = strings.TrimPrefix(scene, "拍个")
			scene = strings.TrimSuffix(scene, "的自拍")
			scene = strings.TrimSuffix(scene, "自拍")
			scene = strings.TrimSuffix(scene, "的照片")
			scene = strings.TrimSuffix(scene, "照")
			scene = strings.TrimSpace(strings.TrimPrefix(scene, "在"))
			return true, scene
		}
	}

	return false, ""
}

func (p *ImageGenPlugin) matchImageGenTrigger(text string) (bool, string) {
	prefixes := []string{
		"生图 ", "生图：", "生图:", "画图 ", "画图：", "画图:",
		"画一个 ", "画一张 ", "画张 ", "画个 ", "生成图片 ", "帮我画 ",
	}
	for _, pref := range prefixes {
		if strings.HasPrefix(text, pref) {
			prompt := strings.TrimSpace(strings.TrimPrefix(text, pref))
			if prompt != "" {
				return true, prompt
			}
		}
	}
	return false, ""
}

func (p *ImageGenPlugin) saveGeneratedImage(data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("图片数据为空")
	}
	cacheDir := filepath.Join("data", "cache", "media")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", err
	}
	fileName := fmt.Sprintf("generated_%d.jpg", time.Now().UnixNano())
	filePath := filepath.Join(cacheDir, fileName)
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", err
	}
	mediaURL := fmt.Sprintf("/api/chats/media?file=%s", fileName)
	return mediaURL, nil
}

func (p *ImageGenPlugin) handleSelfieCommand(receiver *contact.Contact, customScene, userText, speakerName string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("[image_gen] 生成自拍任务崩溃", "err", r)
		}
	}()

	gen := p.getGenerator()
	avatarURL := p.getAvatarURL()
	data, _, scene, err := gen.GenerateSelfie(customScene, avatarURL)
	if err != nil {
		slog.Error("[image_gen] 生成自拍失败", "err", err, "scene", scene)
		return
	}

	mediaURL, _ := p.saveGeneratedImage(data)

	if err := p.sendImageBytes(receiver.GetUsername(), data); err != nil {
		slog.Error("[image_gen] 发送自拍图片失败", "err", err)
		return
	}

	// 同步记忆到 ai 插件
	p.recordToAI(receiver.GetUsername(), userText, speakerName, fmt.Sprintf("[我发送了一张生活自拍照，场景在：%s]", scene), mediaURL)
}

func (p *ImageGenPlugin) handleImageGenCommand(receiver *contact.Contact, prompt, refImage, userText, speakerName string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("[image_gen] 生成图片任务崩溃", "err", r)
		}
	}()

	gen := p.getGenerator()
	data, _, err := gen.GenerateImage(prompt, refImage)
	if err != nil {
		slog.Error("[image_gen] 生成图片失败", "err", err, "prompt", prompt)
		return
	}

	mediaURL, _ := p.saveGeneratedImage(data)

	if err := p.sendImageBytes(receiver.GetUsername(), data); err != nil {
		slog.Error("[image_gen] 发送图片失败", "err", err)
		return
	}

	// 同步记忆到 ai 插件
	p.recordToAI(receiver.GetUsername(), userText, speakerName, fmt.Sprintf("[我发送了一张绘制的图片，画面内容是：%s]", prompt), mediaURL)
}

func (p *ImageGenPlugin) recordToAI(session, userText, speakerName, assistantMemory, mediaURL string) {
	if p.caller == nil || strings.TrimSpace(session) == "" {
		return
	}
	args := map[string]string{
		"session":        session,
		"user_text":      userText,
		"speaker_name":   speakerName,
		"assistant_text": assistantMemory,
		"media_url":      mediaURL,
	}
	_, _, err := p.caller.CallPlugin("ai.record_interaction", args)
	if err != nil {
		slog.Warn("[image_gen] 同步记忆到 ai 插件失败", "err", err, "session", session)
	} else {
		slog.Info("[image_gen] 成功同步生成图片记忆到 ai 插件", "session", session, "memory", assistantMemory, "media_url", mediaURL)
	}
}

func (p *ImageGenPlugin) findReferenceImage(msg *message.Message, sessionKey string) string {
	// 1. 优先从引用消息解析图片
	if msg.GetType() != nil && msg.GetType().GetCode() == message.TypeAppQuote.Code {
		data, mime, err := p.downloadWeChatImage(msg)
		if err == nil && len(data) > 0 {
			return fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data))
		}
	}

	// 2. 从会话最近缓存的图片中提取
	p.imgMu.Lock()
	cached, ok := p.recentImages[sessionKey]
	p.imgMu.Unlock()
	if ok && cached != nil && time.Since(cached.Time) <= 5*time.Minute {
		mime := cached.MimeType
		if mime == "" {
			mime = "image/jpeg"
		}
		return fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(cached.Data))
	}

	return ""
}

func (p *ImageGenPlugin) cacheRecentImage(sessionKey, senderID string, data []byte, mime string) {
	p.imgMu.Lock()
	defer p.imgMu.Unlock()
	if p.recentImages == nil {
		p.recentImages = make(map[string]*recentImageInfo)
	}
	p.recentImages[sessionKey] = &recentImageInfo{
		Data:     data,
		MimeType: mime,
		Time:     time.Now(),
		SenderID: senderID,
	}
}

func (p *ImageGenPlugin) getAvatarURL() string {
	if p.contact != nil {
		if self := p.contact.GetSelf(); self != nil && strings.TrimSpace(self.GetAvatar()) != "" {
			return strings.TrimSpace(self.GetAvatar())
		}
	}
	url := strings.TrimSpace(p.Config.DefaultAvatarURL)
	if url == "" {
		url = defaultConfig().DefaultAvatarURL
	}
	return url
}

func (p *ImageGenPlugin) sendImageBytes(receiverUsername string, data []byte) error {
	if p.cdn == nil {
		return errors.New("cdn ability is not injected")
	}
	_, err := p.cdn.UploadImage(receiverUsername, bytes.NewReader(data))
	return err
}

func (p *ImageGenPlugin) sendText(receiver *contact.Contact, text string) {
	if p.message == nil || receiver == nil {
		return
	}
	_, _ = p.message.Send(&message.Message{
		Type:     message.TypeText,
		Receiver: receiver,
		Content:  text,
		Data:     &message.Message_Text{Text: &message.TextData{Content: text}},
	})
}

func (p *ImageGenPlugin) resolveReceiver(e *plugin.Event, msg *message.Message) *contact.Contact {
	if p.contact != nil {
		if c := p.contact.Get(e.GetSender()); c != nil {
			return c
		}
	}
	if msg.Sender != nil && msg.Sender.GetUsername() == e.GetSender() {
		return msg.Sender
	}
	if msg.Receiver != nil && msg.Receiver.GetUsername() == e.GetSender() {
		return msg.Receiver
	}
	return &contact.Contact{Username: e.GetSender()}
}

func (p *ImageGenPlugin) extractMessageText(msg *message.Message) string {
	if msg == nil {
		return ""
	}
	if text := msg.GetText(); text != nil && text.GetContent() != "" {
		return text.GetContent()
	}
	if app := msg.GetApp(); app != nil {
		if app.GetTitle() != "" {
			return app.GetTitle()
		}
		if app.GetDesc() != "" {
			return app.GetDesc()
		}
		// 引用消息 XML
		var data struct {
			Title string `xml:"title"`
		}
		if err := xml.Unmarshal([]byte(app.GetXml()), &data); err == nil && data.Title != "" {
			return data.Title
		}
	}
	return msg.GetContent()
}

func cleanText(content string) string {
	text := strings.TrimSpace(content)
	for strings.HasPrefix(text, "@") {
		idx := strings.IndexAny(text, " \t\r\n\u2005\u00a0:：,，")
		if idx > 0 {
			text = strings.TrimSpace(text[idx:])
			text = strings.TrimLeft(text, " :：,，\t\r\n\u2005\u00a0")
		} else {
			break
		}
	}
	if idx := strings.LastIndex(text, "@"); idx > 0 {
		text = strings.TrimSpace(text[:idx])
	}
	return strings.TrimSpace(text)
}

func (p *ImageGenPlugin) downloadWeChatImage(msg *message.Message) ([]byte, string, error) {
	if msg == nil {
		return nil, "", errors.New("消息为空")
	}

	raw := msg.GetRaw()
	info := parseImageCDNInfo(rawContentValue(raw))

	var data []byte

	// 1. 优先使用 cdn.Ability 下载
	if p.cdn != nil {
		type cand struct {
			fileID string
			key    string
		}
		cands := []cand{
			{fileID: info.MidURL, key: info.AesKey},
			{fileID: info.BigURL, key: info.AesKey},
			{fileID: info.ThumbURL, key: info.thumbKey()},
		}
		for _, c := range cands {
			if c.fileID == "" || c.key == "" {
				continue
			}
			rc, err := p.cdn.DownloadImage(c.fileID, c.key)
			if err != nil {
				continue
			}
			buf, err := io.ReadAll(rc)
			_ = rc.Close()
			if err == nil && len(buf) > 0 {
				data = buf
				break
			}
		}
	}

	// 2. 尝试协议包 ImgBuf 缩略图
	if len(data) == 0 {
		if buf := rawImageBuffer(raw); len(buf) > 0 {
			data = buf
		}
	}

	// 3. 尝试 message.Download
	if len(data) == 0 && p.message != nil {
		if rc, err := p.message.Download(msg); err == nil {
			buf, err := io.ReadAll(rc)
			_ = rc.Close()
			if err == nil && len(buf) > 0 {
				data = buf
			}
		}
	}

	if len(data) == 0 {
		return nil, "", errors.New("获取微信图片失败")
	}

	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		mimeType = "image/jpeg"
	}
	return data, mimeType, nil
}

type imageCDNInfo struct {
	AesKey   string
	ThumbKey string
	MidURL   string
	BigURL   string
	ThumbURL string
}

func (info imageCDNInfo) thumbKey() string {
	if info.ThumbKey != "" {
		return info.ThumbKey
	}
	return info.AesKey
}

func parseImageCDNInfo(content string) imageCDNInfo {
	if strings.TrimSpace(content) == "" {
		return imageCDNInfo{}
	}
	var node struct {
		XMLName  xml.Name `xml:"msg"`
		Img      struct {
			AesKey   string `xml:"aeskey,attr"`
			ThumbKey string `xml:"cdnthumbaeskey,attr"`
			MidURL   string `xml:"cdnmidimgurl,attr"`
			BigURL   string `xml:"cdnbigimgurl,attr"`
			ThumbURL string `xml:"cdnthumburl,attr"`
		} `xml:"img"`
	}
	if err := xml.Unmarshal([]byte(content), &node); err == nil {
		return imageCDNInfo{
			AesKey:   node.Img.AesKey,
			ThumbKey: node.Img.ThumbKey,
			MidURL:   node.Img.MidURL,
			BigURL:   node.Img.BigURL,
			ThumbURL: node.Img.ThumbURL,
		}
	}
	return imageCDNInfo{}
}

func rawContentValue(raw string) string {
	var data struct {
		Content struct {
			Value string `json:"value"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return ""
	}
	return data.Content.Value
}

func rawImageBuffer(raw string) []byte {
	var data struct {
		ImageBuffer struct {
			Data []byte `json:"data"`
		} `json:"image_buffer"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil
	}
	return data.ImageBuffer.Data
}
