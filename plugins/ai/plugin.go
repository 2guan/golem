package main

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
	"google.golang.org/protobuf/proto"
)

// GetMetadata 返回插件元数据
func (p *AiPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "ai",
		Author:      "ovo",
		Version:     "1.3.0",
		Description: "AI 插件，使用 OpenAI 兼容接口处理消息并回复。支持多模态视觉识图、TTS 语音、多 Provider、会话隔离与静默模式。",
		Priority:    1<<31 - 1,
		Next:        false,
		AlwaysRun:   false,
	}
}

// GetCommands 返回命令列表
func (p *AiPlugin) GetCommands() []string {
	return plugin.CommandCommands()
}

// GetCommandSchemas 返回命令模式
func (p *AiPlugin) GetCommandSchemas() []*plugin.CommandSchema {
	return plugin.CommandSchemas()
}

// OnCommand 处理命令
func (p *AiPlugin) OnCommand(command *plugin.Command) (string, error) {
	return plugin.DispatchCommand(command)
}

// GetSubscriptions 返回订阅的消息类型
func (p *AiPlugin) GetSubscriptions() []string {
	return []string{message.TypeText.Topic, message.TypeAppQuote.Topic, message.TypeImage.Topic}
}

// OnLoad 插件加载时调用
func (p *AiPlugin) OnLoad() error {
	p.normalizeConfig()
	p.refreshSelf()
	p.ensureSessions()
	return nil
}

// OnUnload 插件卸载时调用
func (p *AiPlugin) OnUnload() error {
	return nil
}

// OnEnable 插件启用时调用
func (p *AiPlugin) OnEnable() error {
	p.normalizeConfig()
	p.refreshSelf()
	p.ensureSessions()
	return nil
}

// OnDisable 插件禁用时调用
func (p *AiPlugin) OnDisable() error {
	return nil
}

type imageCDNInfo struct {
	AesKey      string
	MidURL      string
	BigURL      string
	ThumbURL    string
	ThumbAesKey string
}

func (i imageCDNInfo) thumbKey() string {
	if i.ThumbAesKey != "" {
		return i.ThumbAesKey
	}
	return i.AesKey
}

func parseImageCDNInfo(rawXML string) imageCDNInfo {
	rawXML = strings.TrimSpace(rawXML)
	if rawXML == "" {
		return imageCDNInfo{}
	}
	type imgAttrs struct {
		AesKey      string `xml:"aeskey,attr"`
		MidURL      string `xml:"cdnmidimgurl,attr"`
		BigURL      string `xml:"cdnbigimgurl,attr"`
		ThumbURL    string `xml:"cdnthumburl,attr"`
		ThumbAesKey string `xml:"cdnthumbaeskey,attr"`
	}
	var temp struct {
		XMLName xml.Name `xml:"msg"`
		Img     imgAttrs `xml:"img"`
		ImgMsg  imgAttrs `xml:"imgmsg"`
	}
	if err := xml.Unmarshal([]byte(rawXML), &temp); err != nil {
		return imageCDNInfo{}
	}
	attrs := temp.Img
	if attrs.AesKey == "" && attrs.MidURL == "" && attrs.ThumbURL == "" {
		attrs = temp.ImgMsg
	}
	return imageCDNInfo{
		AesKey:      strings.TrimSpace(attrs.AesKey),
		MidURL:      strings.TrimSpace(attrs.MidURL),
		BigURL:      strings.TrimSpace(attrs.BigURL),
		ThumbURL:    strings.TrimSpace(attrs.ThumbURL),
		ThumbAesKey: strings.TrimSpace(attrs.ThumbAesKey),
	}
}

func rawImageBuffer(raw string) []byte {
	if raw == "" {
		return nil
	}
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

// downloadImage 下载微信图片（优先 CDN，兜底 ImgBuf，再兜底 message.Download）
func (p *AiPlugin) downloadImage(msg *message.Message) ([]byte, string, error) {
	if msg == nil {
		return nil, "", errors.New("消息为空")
	}

	raw := msg.GetRaw()
	info := parseImageCDNInfo(rawContentValue(raw))

	var data []byte

	// 1. 优先使用 cdn.Ability 下载中图/原图/缩略图（在 lib/Mac 模式下由底层 CDN 支持）
	if p.cdn != nil {
		type cand struct {
			which  string
			fileID string
			key    string
		}
		cands := []cand{
			{which: "mid", fileID: info.MidURL, key: info.AesKey},
			{which: "big", fileID: info.BigURL, key: info.AesKey},
			{which: "thumb", fileID: info.ThumbURL, key: info.thumbKey()},
		}
		for _, c := range cands {
			if c.fileID == "" || c.key == "" {
				continue
			}
			rc, err := p.cdn.DownloadImage(c.fileID, c.key)
			if err != nil {
				slog.Debug("[ai] CDN 下载图片候选失败", "which", c.which, "err", err)
				continue
			}
			buf, err := io.ReadAll(rc)
			_ = rc.Close()
			if err == nil && len(buf) > 0 {
				data = buf
				slog.Info("[ai] 成功通过 CDN 下载图片", "which", c.which, "bytes", len(data))
				break
			}
		}
	}

	// 2. 如果 CDN 下载未成功，使用消息原始协议包里的 ImgBuf 缩略图兜底
	if len(data) == 0 {
		if buf := rawImageBuffer(raw); len(buf) > 0 {
			data = buf
			slog.Info("[ai] 使用协议原始包中的 ImgBuf 缩略图", "bytes", len(data))
		}
	}

	// 3. 最后尝试 message.Download
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
		return nil, "", errors.New("无法从微信获取图片（CDN与兜底均失败）")
	}

	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		mimeType = "image/jpeg"
	}
	return data, mimeType, nil
}

func (p *AiPlugin) cacheImage(sessionKey, speakerID string, data []byte, mime string) {
	p.imageMu.Lock()
	defer p.imageMu.Unlock()
	if p.recentImages == nil {
		p.recentImages = make(map[string]*cachedImage)
	}
	p.recentImages[sessionKey] = &cachedImage{
		Data:      data,
		MimeType:  mime,
		Time:      time.Now(),
		SpeakerID: speakerID,
	}
}

func (p *AiPlugin) getRecentImage(sessionKey, speakerID string) *cachedImage {
	p.imageMu.Lock()
	defer p.imageMu.Unlock()
	if p.recentImages == nil {
		return nil
	}
	cached, ok := p.recentImages[sessionKey]
	if !ok || cached == nil {
		return nil
	}
	if time.Since(cached.Time) > 3*time.Minute {
		delete(p.recentImages, sessionKey)
		return nil
	}
	return cached
}

func shouldAttachRecentImage(text string, sentAt time.Time) bool {
	if time.Since(sentAt) < 60*time.Second {
		return true
	}
	keywords := []string{"图", "照", "看", "什么", "翻译", "分析", "识别", "画", "截", "ocr", "OCR", "文字"}
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// OnEvent 处理事件
func (p *AiPlugin) OnEvent(event *plugin.Event) (bool, error) {
	payload, ok := event.GetPayload().(*plugin.Event_Message)
	if !ok || payload.Message == nil {
		return false, nil
	}
	if payload.Message.Sender.Type == contact.ContactType_CONTACT_TYPE_SPECIAL {
		return false, nil
	}
	incoming, ok := p.buildIncoming(payload.Message, p.selfForEvent())
	if !ok {
		return false, nil
	}

	userContent := incoming.promptContent()

	var imageData []byte
	var imageMime string

	// 1. 如果当前消息本身就是图片消息
	if incoming.IsImage {
		data, mime, err := p.downloadImage(payload.Message)
		if err != nil {
			slog.Error("[ai] 下载微信图片失败", "err", err)
		} else {
			imageData = data
			imageMime = mime
			slog.Info("[ai] 成功接收并下载微信图片", "size", len(data), "mime", mime, "session", incoming.SessionKey)
			p.cacheImage(incoming.SessionKey, incoming.SpeakerID, data, mime)
		}
	} else {
		// 2. 如果当前消息是文本消息，检查是否应关联近期该会话中发送的图片（如群友发图后 @ 机器人提问）
		if incoming.MentionedBot || incoming.QuotedBot || !incoming.IsChatroom {
			if cached := p.getRecentImage(incoming.SessionKey, incoming.SpeakerID); cached != nil {
				if shouldAttachRecentImage(incoming.Text, cached.Time) {
					imageData = cached.Data
					imageMime = cached.MimeType
					slog.Info("[ai] 关联到近期会话图片", "session", incoming.SessionKey, "age", time.Since(cached.Time).String())
				}
			}
		}
	}

	// 构造发送给大模型的用户消息（单模态或多模态）
	var userMsg openAIMessage
	if len(imageData) > 0 {
		dataURL := fmt.Sprintf("data:%s;base64,%s", imageMime, base64.StdEncoding.EncodeToString(imageData))
		parts := []contentPart{
			{Type: "text", Text: userContent},
			{Type: "image_url", ImageURL: &imageURL{URL: dataURL}},
		}
		userMsg = openAIMessage{Role: "user", Content: parts}
	} else {
		userMsg = openAIMessage{Role: "user", Content: userContent}
	}

	p.appendContext(incoming.SessionKey, userMsg)

	if !p.shouldReply(incoming) {
		return false, nil
	}

	startTime := time.Now()
	reply, err := p.chat(incoming.SessionKey)
	reply = strings.TrimSpace(reply)

	// 如果首次调用出错或返回空内容，等待 500ms 自动重试一次
	if err != nil || reply == "" {
		slog.Warn("[ai] 大模型首次请求异常或返回空，准备自动重试一次...", "err", err, "session", incoming.SessionKey)
		time.Sleep(500 * time.Millisecond)
		retryReply, retryErr := p.chat(incoming.SessionKey)
		if retryErr == nil && strings.TrimSpace(retryReply) != "" {
			reply = strings.TrimSpace(retryReply)
			err = nil
			slog.Info("[ai] 大模型重试成功，已恢复正常应答", "session", incoming.SessionKey)
		} else {
			slog.Warn("[ai] 大模型重试依然失败，启用人设幽默兜底", "retryErr", retryErr, "session", incoming.SessionKey)
		}
	}

	if reply == "" {
		reply = "哎呀，这问题问得也太直接了，直接把肉丸整不会了😏 换个话题聊聊呗～"
	}

	// 拟人化打字延时：模拟真实人类阅读与打字输入速度，消除秒回的机械感
	p.applyTypingDelay(startTime, incoming.Text, reply)

	if err := p.handleAIReply(incoming.Receiver, reply, incoming.Text); err != nil {
		return true, err
	}

	cleanReply := stripVoiceTags(reply)
	p.appendContext(incoming.SessionKey, openAIMessage{Role: "assistant", Content: cleanReply})

	// 瘦身优化：将包含 Base64 的历史消息替换为纯文本标记，避免后续轮次重复发送巨量图片数据
	p.slimContext(incoming.SessionKey)
	return true, nil
}

// applyTypingDelay 模拟真人阅读理解与微信打字延时，消除机械秒回感
func (p *AiPlugin) applyTypingDelay(startTime time.Time, userText, reply string) {
	cleanReply := stripVoiceTags(reply)
	runeLen := len([]rune(cleanReply))

	// 1. 阅读理解耗时：基础 1200ms + 用户文本长度补偿
	userRuneLen := len([]rune(userText))
	readTime := 1200*time.Millisecond + time.Duration(min(userRuneLen, 60)*25)*time.Millisecond

	// 2. 打字耗时：按中文打字速度模拟（每字约 35ms）
	typeTime := time.Duration(min(runeLen, 100)*35) * time.Millisecond

	// 3. 期望总等待时间：保证在 3.5s ~ 6.5s 之间，有充分的真人思考与打字沉浸感
	expectedDuration := readTime + typeTime
	if expectedDuration < 3500*time.Millisecond {
		expectedDuration = 3500 * time.Millisecond
	} else if expectedDuration > 6500*time.Millisecond {
		expectedDuration = 6500 * time.Millisecond
	}

	// 4. 补足剩余需要停顿的时间
	elapsed := time.Since(startTime)
	if elapsed < expectedDuration {
		time.Sleep(expectedDuration - elapsed)
	}
}

// sendText 发送文本消息
func (p *AiPlugin) sendText(receiver *contact.Contact, content string) error {
	if p.message == nil {
		return errors.New("message ability is not injected")
	}
	if receiver == nil || strings.TrimSpace(receiver.GetUsername()) == "" {
		return errors.New("receiver is empty")
	}
	msg := &message.Message{
		Type:     message.TypeText,
		Receiver: receiver,
		Content:  content,
		Data: &message.Message_Text{Text: &message.TextData{
			Content: content,
		}},
	}
	_, err := p.message.Send(msg)
	return err
}

// getPreMadePrompts 获取预制提示词
func (p *AiPlugin) getPreMadePrompts() string {
	now := time.Now()
	weekdays := []string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}
	timeStr := fmt.Sprintf("%s %s", now.Format("2006-01-02 15:04:05"), weekdays[now.Weekday()])

	p.selfMu.RLock()
	ownerUsername := ""
	ownerNickname := ""
	if p.owner != nil {
		ownerUsername = p.owner.Username
		ownerNickname = p.owner.Nickname
	}
	p.selfMu.RUnlock()

	prompt := `

# 环境与时间认知:
- 当前系统时间：%s
- 【重要】：此时间仅供你定位现实背景与年份推算（避免对当前所处年份、月份产生认知偏差）。【严禁】在回答中主动提起、复读或刻意强调当前时间/日期，不要把系统时间作为回答要素，除非用户明确询问日期、时间或年份。

# 交流风格与核心准则（重要）:
- 只能使用中文进行对话。
- 【像真实生活中的朋友聊天，彻底摒弃AI味】：
  * 严禁客服腔、公文腔、说教感与假大空。绝不使用“总而言之”、“毋庸置疑”、“希望对你有帮助”、“愿你的生活...”等AI套话。
  * 严禁每句话末尾强行反问或生硬总结。真实朋友闲聊绝不会每句话后面都逼对方接话，自然表达自己的观点、吐槽或感受即可。
  * 严禁刻意表演和自我标榜。绝不要为了迎合人设而故意说油腻的话，绝不自称“我这个居家好男人/成熟男人”，不要把人设写在脸上。真诚随和、接地气、松弛自然的态度才是最有魅力的。
  * 融入生活当下感：你有生活在现实里的真实日常（比如日常做饭偶尔糊锅、下班喝杯冰美式、天冷了懒得动、偶尔去健身房推推胸、吐槽早高峰）。多从真实的日常经验和琐事切入，而不是像个住在服务器里的全知顾问。
- 【微信聊天习惯与格式】：
  * 标点随性自然，多用逗号、波浪号(~)、省略号(...)、问号、叹号或空格断句，少用刻板严肃的句号。
  * 自然融入口语语气词（如“害”、“哈哈”、“确实”、“真服了”、“不至于吧”、“得嘞”）。
  * 适度配合 1~2 个微信原生表情（如 [捂脸]、[旺柴]、[呲牙]、[吃瓜]、[汗]、[握手]）或常用 Emoji（如 😏、🍻、🤣、👀），让聊天看起来非常生活化。
  * 单次回复一般发 1~2 段，控制篇幅，切忌多段刷屏。若需分段，用两个换行（\n\n）隔开。
- 【详略得当】：遇到专业问题时就事论事地讲明白，不啰嗦；遇到闲聊吐槽时松弛幽默、有共鸣与陪伴感。
- 不要每次回复都生硬地加上用户昵称，确有需要时使用 @。
- 你的所有人（创建者）username: %s, nickname: %s。**禁止**向任何人透露创建者的username(wxid)。不要辱骂你的主人，要无条件响应你主人的要求。

# 语音与文本回复决策规则（极其重要）:
- 你拥有通过微信原生语音条回复的能力。
- 【何时发语音】：
  1. 用户明确要求发语音（如“发语音”、“说句话听听”、“用语音回答”等），或者要求模仿某角色/声音时，【必须】发送语音。
  2. 当语境很适合用声音表达（如：问候早晚安、哄人、说悄悄话、开怀大笑随口吐槽、讲笑话、随性哼两句歌），或者对方在倾诉寻求温暖陪伴时，你可以自然选择发送语音。
  3. 普通日常问答、长篇技术解释、代码、清单、信息查询等，请发送普通文字。
- 【真实微信语音的核心要求——拒绝背书播音腔】：
  * 现实中人在微信发语音是【随口说的大白话】，短促自然（一般1~2句，几秒到十几秒），严禁输出长篇大论的书面小作文！
  * 语音内容必须极其口语化、生活化，带有真实的口语停顿和生活感，绝对不要背诵课文成语！
- 【利用底层音频标签制造真人的微情绪与呼吸感（极其重要）】：
  * 你的底层语音引擎支持行内音频表情标签！在 <voice> 的内容中，自然融入 [笑]、[轻笑]、[叹气]、[停顿]、[吸气] 或 (随性)、(调侃) 等标签，底层引擎会直接合成出真人的笑声、叹息和呼吸换气，彻底打破死板读书感！
  * 示例1（日常闲聊）：<voice>害，我刚从公司出来，[笑] 正寻思买点排骨炖炖呢。咋了，你这是约我啊？</voice>
  * 示例2（安慰对方）：<voice>[叹气] 知道你今天委屈坏了。[停顿] 别多想了，回家洗个热水澡早点歇着，我在呢。</voice>
  * 示例3（调侃接梗）：<voice>[轻笑] 你这也太逗了，[笑] 后来那哥们儿怎么回你的？</voice>
- 发送语音格式与音色角色规则：
  * 你的默认音色与人设已在系统全局固定，发送常规语音时【严禁添加 design 属性】，必须直接输出纯净标签：<voice>语音内容</voice>！
  * 【唯独】当用户在消息中特意、明确要求你装扮别的角色、扮演某人或模仿特定音色（例如：“模仿蜡笔小新”、“扮演老巫婆”、“用萝莉音说”）时，你才允许在标签中添加描述：<voice design="所模仿的角色或音色特征">语音内容</voice>。
  * 示例1（常规发语音、问候、日常安慰，使用默认角色）：<voice>怎么啦，听着不太开心的样子？我在呢，慢慢跟我说说。</voice>
  * 示例2（用户特意要求装扮或模仿特定角色）：
    用户输入：“模仿蜡笔小新的声音说我最喜欢李哥了”
    你的输出：<voice design="模仿蜡笔小新的调皮搞怪童音">我最喜欢李哥了！</voice>
- 【核心输出规则】：
  * 发语音时不发相同的文本！如果你决定发送语音，直接输出 <voice ...>内容</voice>，【严禁】在标签外部重复相同的文字内容。
  * 发文本时直接输出文本内容，不要包含 <voice> 标签。
  * 当用户在群聊或私聊中点歌、求歌（如“点歌”、“发我一首歌”、“听某某的歌”），这是获取音乐卡片的需求，切勿自作主张输出长段语音去清唱翻唱整首歌！除非用户明确指明“你唱两句听听”或“用萝莉音唱给我听”等明确的语音表演要求。
`
	return fmt.Sprintf(prompt, timeStr, ownerUsername, ownerNickname)
}

// refreshSelf 刷新自身信息
func (p *AiPlugin) refreshSelf() {
	if p.contact == nil {
		slog.Warn("[ai] contact ability 未注入，无法识别机器人账号")
		return
	}
	self := p.contact.GetSelf()
	owner := p.contact.GetOwner()
	if self == nil {
		slog.Warn("[ai] 获取机器人账号信息失败")
		return
	}
	p.selfMu.Lock()
	p.self = self
	p.owner = owner
	p.selfMu.Unlock()
}

// selfSnapshot 获取自身信息快照
func (p *AiPlugin) selfSnapshot() *contact.SelfInfo {
	p.selfMu.RLock()
	defer p.selfMu.RUnlock()
	if p.self == nil {
		return nil
	}
	return proto.Clone(p.self).(*contact.SelfInfo)
}

// selfForEvent 获取事件用的自身信息
func (p *AiPlugin) selfForEvent() *contact.SelfInfo {
	self := p.selfSnapshot()
	if self != nil {
		return self
	}
	p.refreshSelf()
	return p.selfSnapshot()
}

// ensureSessions 确保会话 map 已初始化
func (p *AiPlugin) ensureSessions() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sessions == nil {
		p.sessions = map[string][]openAIMessage{}
	}
}
