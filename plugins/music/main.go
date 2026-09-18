package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

type MusicPlugin struct {
	message message.Ability
}

var xmlTemplate = `<appmsg appid="%s" sdkver="0">
    <title>%s</title>
    <des>%s</des>
    <action>view</action>
    <type>3</type>
    <dataurl>%s</dataurl>
    <songalbumurl>%s</songalbumurl>
    <songlyric>%s</songlyric>
</appmsg>
`

var prefixes = []string{"音乐 ", "点歌 ", "music "}

func (m *MusicPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "music",
		Author:      "ovo",
		Version:     "1.0.0",
		Description: "音乐点播，发送 '[音乐|点歌|music] music_name' 点歌",
		Priority:    10,
		Next:        false,
		AlwaysRun:   false,
	}
}

func (m *MusicPlugin) GetSubscriptions() []string {
	return []string{message.TypeText.Topic}
}

func extractSongName(content string) string {
	text := strings.TrimSpace(content)
	// 1. 剥离可能存在的 @前缀（如 "@肉丸叔叔\u2005" 或 "@机器人 "）
	if strings.HasPrefix(text, "@") {
		if idx := strings.IndexAny(text, " \t\r\n\u2005\u00a0"); idx > 0 {
			text = strings.TrimSpace(text[idx:])
		}
	}

	// 2. 规则前缀列表（按长度降序优先匹配）
	patterns := []string{
		"发给我一首歌", "给我发一首歌", "给我放一首歌", "给我点一首歌", "给我来一首歌",
		"发给我一首", "给我发一首", "给我放一首", "给我点一首", "给我来一首",
		"发给我首歌", "给我发首歌", "发给我首", "给我发首", "发给我个歌", "给我发个歌",
		"发我一首歌", "来一首歌", "放一首歌", "点一首歌", "听一首歌", "搜一首歌",
		"发我一首", "发我首", "给我一首", "给我来首", "发一首", "来一首",
		"点一首", "放一首", "听一首", "搜一首", "发首歌", "来首歌",
		"放首歌", "点首歌", "听首歌", "搜首歌", "发个歌", "来个歌",
		"放个歌", "点个歌", "听个歌", "搜个歌",
		"发首", "来首", "点首", "放首", "听首", "搜首",
		"我要听", "我想听", "要听", "想听", "听听", "听下", "听一下",
		"发给我", "给我发", "发我", "给我",
		"点歌", "放歌", "搜歌", "播放", "点播",
		"音乐", "music",
	}

	var matched bool
	var query string
	for _, p := range patterns {
		if strings.HasPrefix(text, p) {
			query = strings.TrimPrefix(text, p)
			matched = true
			break
		}
	}

	if !matched {
		return ""
	}

	query = strings.TrimSpace(query)
	// 去除常见后缀，如 "孙燕姿的歌" -> "孙燕姿"
	query = strings.TrimSuffix(query, "的歌")
	query = strings.TrimSuffix(query, "这首歌")
	query = strings.TrimSuffix(query, "这歌")
	query = strings.TrimSuffix(query, "歌曲")
	query = strings.TrimSuffix(query, "音乐")
	query = strings.TrimSuffix(query, "听听")
	query = strings.TrimSuffix(query, "听下")
	query = strings.TrimSuffix(query, "一下")
	query = strings.TrimSpace(query)

	return query
}

func (m *MusicPlugin) OnEvent(event *plugin.Event) (bool, error) {
	payload, ok := event.Payload.(*plugin.Event_Message)
	if !ok || payload.Message == nil {
		return false, nil
	}
	msg := payload.Message

	name := extractSongName(msg.Content)
	if name == "" {
		return false, nil
	}

	slog.Info("[music] 收到点歌请求", "query", name, "sender", msg.Sender.GetUsername())

	resp, err := http.DefaultClient.Get("https://109a.cn/API/qqyy/api.php?msg=" + url.PathEscape(name))
	if err != nil {
		slog.Warn("[music] 请求失败", "err", err)
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()

	all, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Warn("[music] 读取响应失败", "err", err)
		return false, err
	}

	result, err := parseMusicResponse(all)
	if err != nil {
		slog.Warn("[music] 解析响应失败", "err", err, "status", resp.StatusCode)
		return false, err
	}

	xmlContent := fmt.Sprintf(xmlTemplate,
		getProvider(),
		result.Song,
		result.Singer,
		result.URL,
		result.Cover,
		result.Lyric,
	)

	_, err = m.message.Send(&message.Message{
		Receiver: msg.Sender,
		Type:     message.TypeAppMusic,
		Content:  fmt.Sprintf("[音乐] %s - %s", result.Song, result.Singer),
		Data: &message.Message_App{App: &message.AppData{
			SubType: 76, // 音乐子类型
			Title:   result.Song,
			Desc:    result.Singer,
			Xml:     xmlContent,
		}},
	})
	if err != nil {
		slog.Warn("[music] 发送消息失败", "err", err)
		return false, err
	}
	return true, nil
}

type musicResult struct {
	Song   string `json:"song,omitempty"`
	Singer string `json:"singer,omitempty"`
	URL    string `json:"url,omitempty"`
	Cover  string `json:"cover,omitempty"`
	Lyric  string `json:"lyric,omitempty"`
}

type musicAPIResponse struct {
	Code int           `json:"code"`
	Data []musicResult `json:"data"`
}

func parseMusicResponse(body []byte) (*musicResult, error) {
	var resp musicAPIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("json 解析失败：%w", err)
	}
	if resp.Code != 200 || len(resp.Data) == 0 {
		return nil, fmt.Errorf("接口返回错误：code=%d, 结果数=%d", resp.Code, len(resp.Data))
	}
	return &resp.Data[0], nil
}

func main() {
	plugin.Start(&MusicPlugin{})
}
