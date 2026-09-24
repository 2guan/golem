package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

type ReviewResult struct {
	Score       string `json:"score"`
	Tag         string `json:"tag"`
	Comment     string `json:"comment"`
	Advice      string `json:"advice"`
}

func (p *AvatarReviewPlugin) reviewAvatar(avatarURL, nickname string) string {
	slog.Debug("[avatar_review] 开始评测头像", "nickname", nickname, "url", avatarURL)

	// 1. 尝试下载图片并转 Base64
	var imgBase64 string
	if avatarURL != "" {
		if data, err := p.downloadImage(avatarURL); err == nil && len(data) > 0 {
			imgBase64 = base64.StdEncoding.EncodeToString(data)
		} else {
			slog.Warn("[avatar_review] 下载头像失败，走备选评测", "err", err)
		}
	}

	// 2. 尝试调用视觉大模型评测
	if imgBase64 != "" {
		if res, err := p.callVisionLLM(imgBase64, nickname); err == nil && strings.TrimSpace(res) != "" {
			return res
		}
	}

	// 3. 降级：趣味老将风格生成
	return p.generateFunReview(nickname)
}

func (p *AvatarReviewPlugin) downloadImage(urlStr string) ([]byte, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (p *AvatarReviewPlugin) callVisionLLM(imageBase64, nickname string) (string, error) {
	botName := p.getBotName()
	prompt := fmt.Sprintf(`你现在是“%s”（幽默嘴碎、毒舌但有分寸感、极其接地气）。
你的名字叫“%s”，平时自称“%s”或“我”，只有在明确遇到年轻人、学生或小孩时才可以自称“叔叔”，其它任何正常情况下绝不要自称叔叔。
请观察用户的微信头像，对该头像进行一次趣味锐评打分。

必须严格按以下格式输出（直接输出文字，不要带任何 markdown 代码块或 json 格式）：
🎨【%s · 头像锐评报告】
👤 评测对象：%s
💯 综合评分：（0-100分，带一位小数，如 89.5 分）
🏷️ 专属标签：（如：【微醺老干部】、【疑似网图搬运工】、【纯欲天花板】等极具网感搞笑的4字标签）

🎙️ %s 毒舌锐评：
（40-70字，从构图、色调、人物神态、或者如果是动物/二次元/风景的角度，用老将嘴碎风格幽默吐槽或夸奖，自称%s或我，不要自称叔叔）

💡 优化/换头像建议：
（20-40字，给出一条无厘头或实用的穿搭/拍摄/头像升级建议）`, botName, botName, botName, botName, nickname, botName, botName)

	type imgURL struct {
		URL string `json:"url"`
	}
	type part struct {
		Type     string  `json:"type"`
		Text     string  `json:"text,omitempty"`
		ImageURL *imgURL `json:"image_url,omitempty"`
	}
	type openAIMsg struct {
		Role    string `json:"role"`
		Content []part `json:"content"`
	}
	type reqBody struct {
		Model    string      `json:"model"`
		Messages []openAIMsg `json:"messages"`
	}

	model := p.Config.Model
	if model == "" {
		model = "mimo-v2.6-flash"
	}

	reqPayload := reqBody{
		Model: model,
		Messages: []openAIMsg{
			{
				Role: "user",
				Content: []part{
					{Type: "text", Text: prompt},
					{Type: "image_url", ImageURL: &imgURL{URL: "data:image/jpeg;base64," + imageBase64}},
				},
			},
		},
	}

	jsonBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	httpReq, err := http.NewRequest("POST", p.Config.BaseURL+"/chat/completions", bytes.NewReader(jsonBytes))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.Config.APIKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var respObj struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &respObj); err != nil {
		return "", err
	}

	if len(respObj.Choices) > 0 {
		return strings.TrimSpace(respObj.Choices[0].Message.Content), nil
	}
	return "", fmt.Errorf("无有效回答")
}

func (p *AvatarReviewPlugin) generateFunReview(nickname string) string {
	scores := []string{"92.4", "88.8", "79.5", "95.0", "84.2", "99.9"}
	score := scores[rand.IntN(len(scores))]

	tags := []string{
		"【赛博养生达人】",
		"【微醺氛围感天花板】",
		"【深藏不露老干部】",
		"【精神状态极度超前】",
		"【高冷电竞少年感】",
		"【都市丽人绝不加班】",
	}
	tag := tags[rand.IntN(len(tags))]

	botName := p.getBotName()
	comments := []string{
		fmt.Sprintf("这头像是真的有点东西，色调稳重中透着一丝看透红尘的疲倦，眼神里写满了‘今天谁也别想让我加班’的淡然。%s看了都得给你点个赞！", botName),
		"好家伙，光线抓得很刁钻，下颌线比我当年的操作思路还要清晰。不过这角度略显矜持，建议多点眼神互动，杀伤力翻倍！",
		"气场拉满，构图非常有杂志大片内味儿。微表情里带着三分薄凉四分漫不经心，一看就是群里闷声干大事的主儿！",
		"这头像散发着一种‘我很贵、我很酷、但我也很想喝奶茶’的奇妙反差萌。很有审美，微胖界的彭于晏甘拜下风！",
	}
	comment := comments[rand.IntN(len(comments))]

	advices := []string{
		"建议保持当前发型，下回拍照稍微侧脸15度，顺便把反光板往上抬一寸，直接出道！",
		"头像已经无可挑剔，唯一的缺点就是太低调了，建议换成无边框纯享模式闪瞎全群！",
		"下次回合可以试着在背景里加点暖光或者霓虹灯带，赛博朋克氛围感直接拉满！",
	}
	advice := advices[rand.IntN(len(advices))]

	return fmt.Sprintf("🎨【%s · 头像锐评报告】\n"+
		"👤 评测对象：%s\n"+
		"💯 综合评分：%s 分\n"+
		"🏷️ 专属标签：%s\n\n"+
		"🎙️ %s 毒舌锐评：\n%s\n\n"+
		"💡 升级优化建议：\n%s", botName, nickname, score, tag, botName, comment, advice)
}
