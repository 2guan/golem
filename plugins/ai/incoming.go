package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"unicode"

	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
)

func (p *AiPlugin) buildIncoming(msg *message.Message, self *contact.SelfInfo) (incomingMessage, bool) {
	sender := msg.GetSender()
	if sender == nil || sender.GetUsername() == "" {
		return incomingMessage{}, false
	}

	isImage := false
	isVoice := false
	if msg.GetType() != nil {
		switch msg.GetType().GetCode() {
		case message.TypeImage.Code:
			isImage = true
		case message.TypeVoice.Code:
			isVoice = true
		}
	}

	isChatroom := sender.GetType() == contactTypeChatroom
	speakerName := ""
	speakerID := ""
	chatroomName := ""
	if isChatroom {
		speakerName = displayMember(msg.GetMember())
		speakerID = msg.GetMember().GetUsername()
		chatroomName = displayContact(sender)
	} else {
		speakerName = displayContact(sender)
		speakerID = sender.GetUsername()
	}

	rawText := messageContent(msg)
	text := strings.TrimSpace(rawText)

	if isVoice {
		if isChatroom {
			text = fmt.Sprintf("[群成员 %s 发送了一条语音消息，请倾听音频并根据群聊情境自然交流]", speakerName)
		} else {
			text = "对方发来了一条语音消息，请倾听音频并根据你的人设自然回复。"
		}
	} else if isImage {
		// 如果文本为空，或者只包含图片分辨率/体积（如 [157 x 210] 68.38KB），统一使用明确的图片描述
		if text == "" || (strings.HasPrefix(text, "[") && strings.Contains(text, "KB")) {
			if isChatroom {
				text = fmt.Sprintf("[群成员 %s 发送了一张图片]", speakerName)
			} else {
				text = "[对方发来了一张照片/图片，请像微信好友一样随口自然聊两句，严禁输出任何图片分析报告或清单]"
			}
		}
	} else if text == "" {
		return incomingMessage{}, false
	}

	in := incomingMessage{
		Receiver:     sender,
		Text:         text,
		IsChatroom:   isChatroom,
		Quote:        extractQuote(msg),
		RawMsg:       msg,
		IsImage:      isImage,
		IsVoice:      isVoice,
		SessionKey:   "private:" + sender.GetUsername(),
		SpeakerName:  speakerName,
		SpeakerID:    speakerID,
		ChatroomName: chatroomName,
	}
	var extraIdentities []string
	if in.IsChatroom {
		in.SessionKey = "chatroom:" + sender.GetUsername()
		if p.chatroom != nil && self != nil {
			if member := p.chatroom.GetMember(sender.GetUsername(), self.GetUsername()); member != nil {
				if member.DisplayName != "" {
					extraIdentities = append(extraIdentities, member.DisplayName)
				}
				if member.Nickname != "" {
					extraIdentities = append(extraIdentities, member.Nickname)
				}
			}
		}
	}
	in.MentionedBot = isMentionedBot(msg, self, extraIdentities...)
	in.QuotedBot = isQuotedBot(in.Quote, self, extraIdentities...)
	return in, true
}

func (in incomingMessage) promptContent() string {
	quotePrefix := ""
	if in.Quote.Content != "" {
		if in.Quote.DisplayName != "" {
			quotePrefix = fmt.Sprintf("[引用 %s: %s] ", in.Quote.DisplayName, in.Quote.Content)
		} else {
			quotePrefix = fmt.Sprintf("[引用: %s] ", in.Quote.Content)
		}
	}

	if in.IsChatroom {
		speaker := in.SpeakerName
		if speaker == "" {
			speaker = "群友"
		}
		return fmt.Sprintf("%s: %s%s", speaker, quotePrefix, in.Text)
	}

	return quotePrefix + in.Text
}

func messageContent(msg *message.Message) string {
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
	}
	return msg.GetContent()
}

func extractRemindsFromRaw(msg *message.Message) []string {
	if msg == nil || msg.GetRaw() == "" {
		return nil
	}
	var rawData struct {
		Source string `json:"source"`
	}
	if err := json.Unmarshal([]byte(msg.GetRaw()), &rawData); err != nil || rawData.Source == "" {
		return nil
	}
	var t struct {
		XmlName xml.Name `xml:"msgsource"`
		Reminds string   `xml:"atuserlist"`
	}
	if err := xml.Unmarshal([]byte(rawData.Source), &t); err != nil || t.Reminds == "" {
		return nil
	}
	var res []string
	for _, s := range strings.Split(t.Reminds, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			res = append(res, s)
		}
	}
	return res
}

func isMentionedBot(msg *message.Message, self *contact.SelfInfo, extraIdentities ...string) bool {
	identities := selfIdentities(self, extraIdentities...)
	if len(identities) == 0 {
		return false
	}
	// 1. 检查普通文本消息原生 Reminds 字段
	if text := msg.GetText(); text != nil {
		for _, remind := range text.GetReminds() {
			if reminderMentionsIdentity(remind, identities) {
				return true
			}
		}
	}
	// 2. 检查 Raw JSON 中 msgsource.atuserlist（对引用消息 TypeAppQuote 等非 Text 消息尤为关键）
	for _, remind := range extractRemindsFromRaw(msg) {
		if reminderMentionsIdentity(remind, identities) {
			return true
		}
	}
	// 3. 检查消息内容是否包含 @身份
	content := messageContent(msg)
	for _, identity := range identities {
		if strings.Contains(content, "@"+identity) {
			return true
		}
	}
	return false
}

func isQuotedBot(quote quoteInfo, self *contact.SelfInfo, extraIdentities ...string) bool {
	identities := selfIdentities(self, extraIdentities...)
	if len(identities) == 0 {
		return false
	}
	for _, value := range []string{quote.FromUser, quote.ChatUser} {
		if containsIdentity(value, identities) {
			return true
		}
	}
	displayName := strings.TrimSpace(quote.DisplayName)
	for _, identity := range identities {
		if displayName == identity || strings.Contains(displayName, identity) {
			return true
		}
	}
	return false
}

func selfIdentities(self *contact.SelfInfo, extraIdentities ...string) []string {
	seen := map[string]struct{}{}
	var identities []string

	add := func(val string) {
		val = strings.TrimSpace(val)
		if val == "" {
			return
		}
		if _, ok := seen[val]; !ok {
			seen[val] = struct{}{}
			identities = append(identities, val)
		}
	}

	if self != nil {
		add(self.GetUsername())
		add(self.GetNickname())
		add(self.GetAlias())
	}
	for _, extra := range extraIdentities {
		add(extra)
	}

	return identities
}

func containsIdentity(value string, identities []string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, identity := range identities {
		if value == identity {
			return true
		}
	}
	return false
}

func reminderMentionsIdentity(remind string, identities []string) bool {
	for _, part := range strings.FieldsFunc(remind, isReminderSeparator) {
		part = strings.TrimPrefix(strings.TrimSpace(part), "@")
		if containsIdentity(part, identities) {
			return true
		}
	}
	return false
}

func isReminderSeparator(r rune) bool {
	return unicode.IsSpace(r) || r == ',' || r == '，' || r == ';' || r == '；'
}

func extractQuote(msg *message.Message) quoteInfo {
	if msg == nil {
		return quoteInfo{}
	}
	if app := msg.GetApp(); app != nil {
		if quote := parseQuoteXML(app.GetXml()); quote.hasValue() {
			return quote
		}
	}
	if raw := msg.GetRaw(); raw != "" {
		if content := rawContentValue(raw); content != "" {
			if quote := parseQuoteXML(content); quote.hasValue() {
				return quote
			}
		}
	}
	return quoteInfo{}
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

func parseQuoteXML(raw string) quoteInfo {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return quoteInfo{}
	}

	var data struct {
		AppMsg struct {
			Refer quoteRefer `xml:"refermsg"`
		} `xml:"appmsg"`
		Refer quoteRefer `xml:"refermsg"`
	}
	if err := xml.Unmarshal([]byte(raw), &data); err != nil {
		return quoteInfo{}
	}
	refer := data.AppMsg.Refer
	if !refer.hasValue() {
		refer = data.Refer
	}
	return quoteInfo{
		FromUser:    strings.TrimSpace(refer.FromUser),
		ChatUser:    strings.TrimSpace(refer.ChatUser),
		DisplayName: strings.TrimSpace(refer.DisplayName),
		Content:     strings.TrimSpace(refer.Content),
	}
}

type quoteRefer struct {
	DisplayName string `xml:"displayname"`
	FromUser    string `xml:"fromusr"`
	ChatUser    string `xml:"chatusr"`
	Content     string `xml:"content"`
}

func (q quoteRefer) hasValue() bool {
	return q.DisplayName != "" || q.FromUser != "" || q.ChatUser != "" || q.Content != ""
}

func (q quoteInfo) hasValue() bool {
	return q.DisplayName != "" || q.FromUser != "" || q.ChatUser != "" || q.Content != ""
}

func displayContact(c *contact.Contact) string {
	if c == nil {
		return ""
	}
	for _, value := range []string{c.GetRemark(), c.GetNickname(), c.GetAlias(), c.GetUsername()} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func displayMember(member interface {
	GetDisplayName() string
	GetRemark() string
	GetNickname() string
	GetAlias() string
	GetUsername() string
}) string {
	if member == nil {
		return ""
	}
	for _, value := range []string{
		member.GetDisplayName(),
		member.GetRemark(),
		member.GetNickname(),
		member.GetAlias(),
		member.GetUsername(),
	} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
