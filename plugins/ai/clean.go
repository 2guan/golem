package main

import (
	"regexp"
	"strings"
)

var (
	// 匹配舞台动作与旁白提示，例如 (笑)、(轻笑)、(调侃)、(随性)、(叹气)、(思考)、(停顿)、（笑）、（调侃）等
	stageDirectionRegex = regexp.MustCompile(`[（(](?:笑|轻笑|微笑|苦笑|冷笑|大笑|调侃|随性|低声|轻声|叹气|深思|思考|停顿|严肃|得意|无奈|释然)[)）]`)

	// 匹配非微信表情的音频控制标签，例如 [停顿]、[吸气]、[呼气]、[喘气]、[大叫]、[急促] 等
	audioControlTagRegex = regexp.MustCompile(`\[(?:停顿|吸气|呼气|喘气|大叫|急促|小声|低语)\]`)
)

// cleanTextMessage 清理普通文本回复中的假表情标签与舞台动作提示，转换为微信真实表情
func cleanTextMessage(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}

	// 1. 移除圆括号舞台动作提示，如 (笑)、(调侃)、（轻笑）
	res := stageDirectionRegex.ReplaceAllString(text, "")

	// 2. 移除音频控制标签，如 [停顿]、[吸气]
	res = audioControlTagRegex.ReplaceAllString(res, "")

	// 3. 将微信不存在的伪表情标签映射为微信原生支持的黄脸表情
	// 微信没有 [笑] 这个表情，输出时微信不会转为表情图，而是原样显示为文字 "[笑]"
	res = strings.ReplaceAll(res, "[笑]", "[呲牙]")
	res = strings.ReplaceAll(res, "[轻笑]", "[偷笑]")
	res = strings.ReplaceAll(res, "[苦笑]", "[捂脸]")
	res = strings.ReplaceAll(res, "[冷笑]", "[坏笑]")
	res = strings.ReplaceAll(res, "[哭笑]", "[笑哭]")
	res = strings.ReplaceAll(res, "[哈哈]", "[大笑]")

	// 4. 清理多余空行或行首行尾空白
	lines := strings.Split(res, "\n")
	var cleanedLines []string
	for _, line := range lines {
		cleanedLines = append(cleanedLines, strings.TrimRight(line, " \t"))
	}
	res = strings.Join(cleanedLines, "\n")

	return strings.TrimSpace(res)
}
