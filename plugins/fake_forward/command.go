package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/sbgayhub/golem/sdk/chatroom"
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

// ChatCommand 手动指定聊天内容。
// 格式：/fake chat 名字1:内容1|名字2:内容2
type ChatCommand struct {
	_        struct{} `cmd:"fake" help:"手动指定假聊天内容" usage:"/fake [chatroom] <名字:内容|名字:内容>" example:"/fake chat 小明:你好|小红:你好呀"`
	Chatroom string   `flag:"c,chatroom" help:"指定群聊名称"`
	Content  string   `arg:"content" help:"聊天内容，竖线分隔多条，冒号分隔名字与内容" required:"true" variadic:"true"`
	Command  *plugin.Command
}

// handle 处理 /fake 命令。
func (p *FakeForwardPlugin) handle(cmd ChatCommand) (string, error) {
	receiver := cmd.Command.Sender
	if cmd.Chatroom != "" {
		receiver = p.contact.Get("nickname::" + cmd.Chatroom)
	}

	records, err := p.parseRecords(receiver, cmd.Content)
	if err != nil || len(records) == 0 {
		return "", err
	}

	slog.Debug("处理伪转发", "records", len(records), "receiver", receiver.GetNickname())
	return p.sendChatRecord(cmd.Command.Sender, records)
}

// parseRecords 解析伪转发负载，支持竖线分隔多条记录。
// 格式：名字1:内容1|名字2:内容2|...
func (p *FakeForwardPlugin) parseRecords(sender *contact.Contact, payload string) ([]recordItem, error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil, fmt.Errorf("empty payload")
	}

	parts := strings.Split(payload, "|")
	var records []recordItem
	var members []*chatroom.Member
	if sender.Type == contact.ContactType_CONTACT_TYPE_CHATROOM {
		members = p.chatroom.ListMembers(sender.Username)
	}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// 支持全角「：」和半角「:」
		colonIdx := strings.Index(part, ":")
		colonLen := 1
		if colonIdx < 0 {
			colonIdx = strings.Index(part, "：")
			colonLen = len("：")
		}
		if colonIdx < 0 {
			continue
		}

		name := strings.TrimSpace(part[:colonIdx])
		content := strings.TrimSpace(part[colonIdx+colonLen:])
		if name == "" || content == "" {
			continue
		}

		var avatar string
		// 1. 支持名字带自定义头像 URL，例如 "马斯克(http://...):内容" 或 "马斯克[http...]:内容"
		if idx := strings.IndexAny(name, "([（【"); idx > 0 {
			endIdx := strings.IndexAny(name[idx+1:], ")]）】")
			if endIdx > 0 {
				customURL := strings.TrimSpace(name[idx+1 : idx+1+endIdx])
				if strings.HasPrefix(customURL, "http://") || strings.HasPrefix(customURL, "https://") {
					avatar = customURL
					name = strings.TrimSpace(name[:idx])
				}
			}
		}

		// 2. 如果是群聊，优先匹配群成员群名片、昵称、备注或 wxid
		if avatar == "" && sender.Type == contact.ContactType_CONTACT_TYPE_CHATROOM {
			slog.Debug("获取群成员信息", "name", name)
			for _, member := range members {
				if member.DisplayName == name || member.Nickname == name || member.Remark == name || member.Username == name {
					avatar = member.GetAvatar()
					slog.Debug("成功获取到成员头像", "name", name, "avatar", avatar)
					break
				}
			}
		}

		// 3. 回退匹配联系人列表
		if avatar == "" {
			if c := p.contact.Get("nickname::" + name); c != nil && c.GetAvatar() != "" {
				avatar = c.GetAvatar()
			} else if c := p.contact.Get("remark::" + name); c != nil && c.GetAvatar() != "" {
				avatar = c.GetAvatar()
			} else if c := p.contact.Get(name); c != nil && c.GetAvatar() != "" {
				avatar = c.GetAvatar()
			}
		}

		records = append(records, recordItem{Name: name, Content: content, AvatarURL: avatar})
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("no valid records found")
	}
	return records, nil
}

// sendChatRecord 构建 XML 并发送聊天记录。
func (p *FakeForwardPlugin) sendChatRecord(receiver *contact.Contact, records []recordItem) (string, error) {
	title := fmt.Sprintf("与%s的聊天记录", records[0].Name)
	desc := fmt.Sprintf("共%d条消息", len(records))

	xmlContent := buildChatRecordXML(title, desc, records)
	_, err := p.message.Send(&message.Message{
		Type:     message.TypeApplication,
		Receiver: receiver,
		Content:  title,
		Data: &message.Message_App{App: &message.AppData{
			SubType: 19,
			Title:   title,
			Desc:    desc,
			Xml:     xmlContent,
		}},
	})
	if err != nil {
		slog.Error("发送聊天记录失败", "err", err)
		return "", fmt.Errorf("发送聊天记录失败: %w", err)
	}
	return "", nil
}

// recordItem 聊天记录中的单条消息。
type recordItem struct {
	Name      string // 发信人昵称
	Content   string // 消息内容
	AvatarURL string // 头像URL
}

func buildChatRecordXML(title, desc string, records []recordItem) string {
	return fmt.Sprintf(`<appmsg>`+
		`<title>%s</title>`+
		`<des>%s</des>`+
		`<action>view</action>`+
		`<type>19</type>`+
		`<url>https://support.weixin.qq.com/cgi-bin/mmsupport-bin/readtemplate?t=page/favorite_record__w_unsupport&from=singlemessage&isappinstalled=0</url>`+
		`<recorditem>%s</recorditem>`+
		`</appmsg>`, escapeXML(title), escapeXML(desc), buildRecordItemXML(title, desc, records))
}

func buildRecordItemXML(title, desc string, records []recordItem) string {
	var builder strings.Builder
	builder.WriteString("<![CDATA[<recordinfo>\n")
	builder.WriteString(fmt.Sprintf("<title>%s</title>\n", escapeXML(title)))
	builder.WriteString(fmt.Sprintf("<desc>%s</desc>\n", escapeXML(desc)))
	builder.WriteString(fmt.Sprintf("<datalist count=\"%d\">\n", len(records)))

	base := time.Now().Add(-time.Duration(60+rand.IntN(600)) * time.Second) // 1-11分钟前开始
	for i, record := range records {
		gap := time.Duration(30+rand.IntN(271)) * time.Second // 30~300秒随机间隔
		t := base.Add(time.Duration(i) * gap)
		timeStr := t.Format("2006-01-02 15:04:05")

		headURL := ""
		if record.AvatarURL != "" {
			u := record.AvatarURL
			if strings.Contains(u, "?") {
				headURL = fmt.Sprintf("%s&ff=%d", u, i)
			} else {
				headURL = fmt.Sprintf("%s?ff=%d", u, i)
			}
		}

		builder.WriteString(fmt.Sprintf(
			"<dataitem datatype=\"1\">\n"+
				"\t<datadesc>%s</datadesc>\n"+
				"\t<sourcename>%s</sourcename>\n"+
				"\t<sourceheadurl>%s</sourceheadurl>\n"+
				"\t<sourcetime>%s</sourcetime>\n"+
				"\t<srcMsgCreateTime>%d</srcMsgCreateTime>\n"+
				"\t<fromnewmsgid>%d</fromnewmsgid>\n"+
				"</dataitem>",
			escapeXML(record.Content),
			escapeXML(record.Name),
			escapeXML(headURL),
			escapeXML(timeStr),
			t.Unix(),
			t.UnixNano(),
		))
		builder.WriteString("\n")
	}

	builder.WriteString("</datalist></recordinfo>]]>")
	return builder.String()
}

func escapeXML(value string) string {
	var buffer bytes.Buffer
	if err := xml.EscapeText(&buffer, []byte(value)); err != nil {
		return value
	}
	return buffer.String()
}
