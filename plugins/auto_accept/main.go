package main

import (
	"encoding/xml"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

// Config 自动通过好友与欢迎语配置
type Config struct {
	AutoVerify           bool   `toml:"auto_verify" comment:"是否自动通过好友申请"`
	GreetingEnabled      bool   `toml:"greeting_enabled" comment:"通过后是否自动发送欢迎打招呼"`
	GreetingDelaySeconds int    `toml:"greeting_delay_seconds" comment:"通过后延迟发送打招呼消息的秒数"`
	NotifyOwner          bool   `toml:"notify_owner" comment:"通过后是否向主人(管管)发送通知"`
	KeywordFilter        string `toml:"keyword_filter" comment:"好友申请附言过滤关键词，若非空则只通过包含该关键词的申请"`
	CustomGreeting       string `toml:"custom_greeting" comment:"自定义打招呼欢迎语，留空则使用肉丸专属欢迎语"`
}

// VerifyMsgXML 好友验证消息 XML 结构体
type VerifyMsgXML struct {
	XMLName         xml.Name `xml:"msg"`
	FromUsername    string   `xml:"fromusername,attr"`
	EncryptUsername string   `xml:"encryptusername,attr"`
	Ticket          string   `xml:"ticket,attr"`
	FromNickname    string   `xml:"fromnickname,attr"`
	Content         string   `xml:"content,attr"`
	Scene           int      `xml:"scene,attr"`
	SourceUsername  string   `xml:"sourceusername,attr"`
	SourceNickname  string   `xml:"sourcenickname,attr"`
}

// AutoAcceptPlugin 自动通过好友申请插件
type AutoAcceptPlugin struct {
	plugin.ConfigAbility[Config]
	message message.Ability
	contact contact.Ability

	dedupMu sync.Mutex
	dedup   map[string]time.Time
}

func (p *AutoAcceptPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "auto_accept",
		Author:      "Golem Team",
		Version:     "1.0.0",
		Description: "自动通过好友申请并发送肉丸专属欢迎语与功能指引",
		Priority:    0,
	}
}

func (p *AutoAcceptPlugin) GetSubscriptions() []string {
	return []string{
		message.TypeVerify.Topic,
	}
}

func (p *AutoAcceptPlugin) OnLoad() error {
	p.dedupMu.Lock()
	p.dedup = make(map[string]time.Time)
	p.dedupMu.Unlock()
	return nil
}

func (p *AutoAcceptPlugin) isDuplicate(ticket, encryptUsername string) bool {
	p.dedupMu.Lock()
	defer p.dedupMu.Unlock()

	now := time.Now()
	for k, t := range p.dedup {
		if now.Sub(t) > 10*time.Minute {
			delete(p.dedup, k)
		}
	}

	key := ticket
	if key == "" {
		key = encryptUsername
	}
	if key == "" {
		return false
	}

	if _, exists := p.dedup[key]; exists {
		return true
	}
	p.dedup[key] = now
	return false
}

func (p *AutoAcceptPlugin) OnEvent(e *plugin.Event) (bool, error) {
	if e == nil || e.Topic != message.TypeVerify.Topic {
		return false, nil
	}

	evtMsg, ok := e.Payload.(*plugin.Event_Message)
	if !ok || evtMsg == nil || evtMsg.Message == nil {
		return false, nil
	}

	rawContent := strings.TrimSpace(evtMsg.Message.GetContent())
	if rawContent == "" {
		return false, nil
	}

	vMsg, err := parseVerifyMsg(rawContent)
	if err != nil {
		slog.Error("[auto_accept] 解析好友验证消息XML失败", "err", err, "raw", rawContent)
		return false, nil
	}

	if vMsg.EncryptUsername == "" || vMsg.Ticket == "" {
		slog.Warn("[auto_accept] 好友验证消息缺少必要凭证", "from", vMsg.FromUsername, "nickname", vMsg.FromNickname)
		return false, nil
	}

	if p.isDuplicate(vMsg.Ticket, vMsg.EncryptUsername) {
		slog.Debug("[auto_accept] 忽略重复的好友验证事件", "ticket", vMsg.Ticket)
		return true, nil
	}

	slog.Info("[auto_accept] 收到好友申请",
		"from_username", vMsg.FromUsername,
		"nickname", vMsg.FromNickname,
		"content", vMsg.Content,
		"scene", vMsg.Scene,
		"scene_desc", sceneDesc(vMsg.Scene),
		"source_user", vMsg.SourceUsername,
		"source_nick", vMsg.SourceNickname,
	)

	// 检查关键词过滤
	if filter := strings.TrimSpace(p.Config.KeywordFilter); filter != "" {
		if !strings.Contains(vMsg.Content, filter) {
			slog.Info("[auto_accept] 好友申请附言未包含指定关键词，跳过自动通过",
				"filter", filter,
				"content", vMsg.Content,
				"from", vMsg.FromNickname,
			)
			return false, nil
		}
	}

	// 是否开启自动验证
	if !p.Config.AutoVerify {
		slog.Info("[auto_accept] 自动通过未开启 (auto_verify=false)，仅记录日志", "from", vMsg.FromNickname)
		return false, nil
	}

	if p.contact == nil {
		slog.Error("[auto_accept] contact ability 未注入，无法通过好友验证")
		return false, nil
	}

	// 执行好友验证通过
	err = p.contact.VerifyFriend(vMsg.EncryptUsername, vMsg.Ticket, vMsg.Scene)
	if err != nil {
		slog.Error("[auto_accept] 通过好友验证失败",
			"err", err,
			"username", vMsg.FromUsername,
			"nickname", vMsg.FromNickname,
		)
		return false, err
	}

	slog.Info("[auto_accept] 🎉 成功通过好友验证！",
		"username", vMsg.FromUsername,
		"nickname", vMsg.FromNickname,
	)

	// 通知主人 (管管)
	if p.Config.NotifyOwner {
		p.notifyOwnerNewFriend(vMsg)
	}

	// 发送欢迎打招呼
	if p.Config.GreetingEnabled {
		targetUser := vMsg.FromUsername
		if targetUser == "" {
			targetUser = vMsg.EncryptUsername
		}
		delaySec := p.Config.GreetingDelaySeconds
		if delaySec <= 0 {
			delaySec = 2
		}

		go func(username, nickname, sourceNickname string, delay time.Duration) {
			time.Sleep(delay)
			greeting := p.buildGreeting(nickname, sourceNickname)
			receiver := &contact.Contact{Username: username}
			if _, sendErr := p.sendText(receiver, greeting); sendErr != nil {
				slog.Warn("[auto_accept] 发送欢迎打招呼失败",
					"receiver", username,
					"nickname", nickname,
					"err", sendErr,
				)
			} else {
				slog.Info("[auto_accept] ✅ 已成功向新好友发送欢迎打招呼",
					"receiver", username,
					"nickname", nickname,
				)
			}
		}(targetUser, vMsg.FromNickname, vMsg.SourceNickname, time.Duration(delaySec)*time.Second)
	}

	return true, nil
}

func (p *AutoAcceptPlugin) notifyOwnerNewFriend(v *VerifyMsgXML) {
	if p.contact == nil || p.message == nil {
		return
	}
	owner := p.contact.GetOwner()
	if owner == nil || owner.Username == "" {
		slog.Debug("[auto_accept] 未找到机器人主人信息，跳过通知")
		return
	}

	sceneStr := sceneDesc(v.Scene)
	msgContent := v.Content
	if msgContent == "" {
		msgContent = "（无附言）"
	}

	text := fmt.Sprintf("📢【新好友添加通知】\n"+
		"已自动通过一位新好友的申请：\n"+
		"👤 昵称：%s\n"+
		"🆔 账号：%s\n"+
		"💬 附言：%s\n"+
		"📌 来源：%s",
		v.FromNickname, v.FromUsername, msgContent, sceneStr)

	if v.SourceNickname != "" {
		text += fmt.Sprintf("\n👥 推荐人：%s", v.SourceNickname)
	}

	_, err := p.sendText(owner, text)
	if err != nil {
		slog.Warn("[auto_accept] 发送主人通知失败", "err", err)
	} else {
		slog.Info("[auto_accept] 已向主人发送新好友添加通知", "owner", owner.Username)
	}
}

func timeGreeting() string {
	hour := time.Now().Hour()
	switch {
	case hour >= 5 && hour < 11:
		return "早上好"
	case hour >= 11 && hour < 13:
		return "中午好"
	case hour >= 13 && hour < 18:
		return "下午好"
	default:
		return "晚上好"
	}
}

func (p *AutoAcceptPlugin) getBotName() string {
	if p.contact != nil {
		if self := p.contact.GetSelf(); self != nil {
			if nick := strings.TrimSpace(self.GetNickname()); nick != "" {
				if strings.Contains(nick, "肉丸") {
					return "肉丸"
				}
				return nick
			}
		}
	}
	return "肉丸"
}

func (p *AutoAcceptPlugin) getAdminName(sourceNickname string) string {
	if s := strings.TrimSpace(sourceNickname); s != "" {
		return s
	}
	if p.contact != nil {
		if owner := p.contact.GetOwner(); owner != nil {
			if nick := strings.TrimSpace(owner.GetNickname()); nick != "" {
				return nick
			}
			if remark := strings.TrimSpace(owner.GetRemark()); remark != "" {
				return remark
			}
		}
	}
	return "管管"
}

func (p *AutoAcceptPlugin) buildGreeting(nickname, sourceNickname string) string {
	tg := timeGreeting()
	botName := p.getBotName()
	adminName := p.getAdminName(sourceNickname)

	if p.Config.CustomGreeting != "" {
		g := p.Config.CustomGreeting
		displayName := strings.TrimSpace(nickname)
		if displayName == "" {
			displayName = "朋友"
		}
		g = strings.ReplaceAll(g, "{nickname}", displayName)
		g = strings.ReplaceAll(g, "{time_greeting}", tg)
		g = strings.ReplaceAll(g, "{greeting}", tg)
		g = strings.ReplaceAll(g, "{bot_name}", botName)
		g = strings.ReplaceAll(g, "{bot}", botName)
		g = strings.ReplaceAll(g, "{admin_name}", adminName)
		g = strings.ReplaceAll(g, "{admin}", adminName)
		g = strings.ReplaceAll(g, "{owner}", adminName)
		return g
	}

	cleanNick := strings.TrimSpace(nickname)
	if cleanNick != "" {
		return fmt.Sprintf("%s%s，我是%s介绍的%s。", cleanNick, tg, adminName, botName)
	}
	return fmt.Sprintf("你好，我是%s介绍的%s。", adminName, botName)
}

func (p *AutoAcceptPlugin) sendText(receiver *contact.Contact, text string) (*message.Send_Response, error) {
	if p.message == nil {
		return nil, fmt.Errorf("message ability 未注入")
	}
	msg := &message.Message{
		Type:     message.TypeText,
		Receiver: receiver,
		Content:  text,
		Data:     &message.Message_Text{Text: &message.TextData{Content: text}},
	}
	return p.message.Send(msg)
}

func parseVerifyMsg(content string) (*VerifyMsgXML, error) {
	var v VerifyMsgXML
	err := xml.Unmarshal([]byte(content), &v)
	if err == nil && v.EncryptUsername != "" && v.Ticket != "" {
		return &v, nil
	}

	// 容错备用正则提取
	reExtract := func(attr string) string {
		re := regexp.MustCompile(attr + `="([^"]*)"`)
		m := re.FindStringSubmatch(content)
		if len(m) > 1 {
			return m[1]
		}
		return ""
	}

	v.FromUsername = reExtract("fromusername")
	v.EncryptUsername = reExtract("encryptusername")
	v.Ticket = reExtract("ticket")
	v.FromNickname = reExtract("fromnickname")
	v.Content = reExtract("content")
	v.SourceUsername = reExtract("sourceusername")
	v.SourceNickname = reExtract("sourcenickname")
	if sceneStr := reExtract("scene"); sceneStr != "" {
		v.Scene, _ = strconv.Atoi(sceneStr)
	}

	if v.EncryptUsername != "" && v.Ticket != "" {
		return &v, nil
	}

	if err != nil {
		return nil, err
	}
	return &v, nil
}

func sceneDesc(scene int) string {
	switch scene {
	case 1:
		return "QQ好友"
	case 2:
		return "搜索微信号/手机号"
	case 3:
		return "微信号搜索"
	case 6:
		return "手机联系人/通讯录"
	case 14:
		return "群聊添加"
	case 15:
		return "手机号搜索"
	case 17:
		return "名片分享推荐"
	case 18:
		return "附近的人"
	case 25:
		return "漂流瓶"
	case 29:
		return "摇一摇"
	case 30:
		return "二维码/扫一扫"
	default:
		return fmt.Sprintf("添加渠道(%d)", scene)
	}
}

func main() {
	p := &AutoAcceptPlugin{
		dedup: make(map[string]time.Time),
	}
	slog.Info("[auto_accept] 自动通过好友与欢迎语插件启动中...")
	plugin.Start(p)
}
