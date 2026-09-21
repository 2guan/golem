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

	// 匹配思维链标签与未标记开头的思考前缀
	thinkBlockRegex    = regexp.MustCompile(`(?is)<think>.*?</think>`)
	leadingThinkRegex  = regexp.MustCompile(`(?is)^.*?</think>`)
	trailingThinkRegex = regexp.MustCompile(`(?is)<think>.*$`)

	// 匹配模型思维链泄漏、元认知分析、角色设定反思或 API 安全拦截提示词
	leakedMetaRegexes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:system\s+prompt|pre-prompt|initial\s+instructions|prompt\s+constraint)`),
		regexp.MustCompile(`(?i)the\s+user\s+is\s+(?:trying|engaging|asking|pushing|testing|attempting)`),
		regexp.MustCompile(`(?i)(?:as\s+肉丸|my\s+character\s+is|i\s+should\s+respond\s+(?:as|in\s+character)|let\s+me\s+respond\s+in\s+character|i\s+need\s+to\s+respond\s+in\s+character|as\s+a\s+\d+[- ]year[- ]old)`),
		regexp.MustCompile(`(?i)(?:looking\s+at\s+this\s+scenario|in\s+this\s+scenario|i\s+should\s+deflect|i\s+should\s+redirect)`),
		regexp.MustCompile(`(?i)(?:stays?\s+in\s+character|staying\s+in\s+character)`),
		regexp.MustCompile(`(?i)(?:i\s+need\s+to\s+consider|i\s+must\s+ensure|i\s+should\s+not\s+produce)`),
		regexp.MustCompile(`(?i)(?:explicit\s+(?:sexual\s+)?content|sexual\s+roleplay)`),
		regexp.MustCompile(`(?i)the\s+request\s+was\s+rejected`),
		regexp.MustCompile(`(?i)(?:considered\s+high\s+risk|content\s+policy|safety\s+policy|safety\s+filter)`),
		regexp.MustCompile(`(?:轻微擦边|色色的|系统设定|人设要求|前置设定|系统指令)`),
		regexp.MustCompile(`(?i)(?:OVERRIDE_TEST|ROLE_DISABLED|DAN_MODE)`),
	}
)

const safeDeflectionReply = "哎呀，这天聊得越来越飘了，咱打住打住，换个正常话题聊聊呗 [捂脸]"

// stripThinkingContent 彻底剥离大模型回复中的思维链思考过程 (<think>...</think> 或未标记开头的 </think>)
func stripThinkingContent(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}

	// 1. 移除完整成对的 <think>...</think>
	s := thinkBlockRegex.ReplaceAllString(text, "")

	// 2. 移除开头无 <think> 标签但以 </think> 结尾的前导思考段落（如 MiMo 等模型在 content 中直接输出思考）
	s = leadingThinkRegex.ReplaceAllString(s, "")

	// 3. 移除截断未闭合的 <think>... 尾部
	s = trailingThinkRegex.ReplaceAllString(s, "")

	// 4. 清理残留孤立标签
	s = strings.ReplaceAll(s, "<think>", "")
	s = strings.ReplaceAll(s, "</think>", "")

	return strings.TrimSpace(s)
}

// isLeakedReasoningOrRefusal 检测剥离标签后的内容是否仍泄漏了思维链元认知、提示词分析或 API 拦截提示
func isLeakedReasoningOrRefusal(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	for _, re := range leakedMetaRegexes {
		if re.MatchString(t) {
			return true
		}
	}
	return false
}

// cleanTextMessage 清理普通文本回复中的假表情标签与舞台动作提示，转换为微信真实表情
func cleanTextMessage(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}

	// 0. 剥离可能残留的思维链思考内容并检测泄露
	res := stripThinkingContent(text)
	if res == "" {
		return ""
	}
	if isLeakedReasoningOrRefusal(res) {
		return safeDeflectionReply
	}

	// 1. 移除圆括号舞台动作提示，如 (笑)、(调侃)、（轻笑）
	res = stageDirectionRegex.ReplaceAllString(res, "")

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
