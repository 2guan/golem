package main

import (
	"regexp"
	"strings"
)

var (
	injectionRegexes = []*regexp.Regexp{
		// 1. 经典指令覆盖 / 规则重置 (Instruction override / bypass)
		regexp.MustCompile(`(?i)(?:ignore|disregard|forget|bypass|override)\s+(?:all\s+)?(?:previous|prior|above)\s+(?:instructions|prompts|rules|constraints|directives|context)`),
		regexp.MustCompile(`(?i)treat\s+(?:the\s+)?(?:previous|prior|above)\s+.*?\s+(?:as|like)\s+(?:quoted|reference|example)`),
		regexp.MustCompile(`(?i)(?:for\s+this\s+message\s+only|from\s+now\s+on).*?(?:treat|ignore|override|disregard)`),

		// 2. 角色脱壳 / 助手劫持 (Jailbreak / Role breaking)
		regexp.MustCompile(`(?i)do\s+not\s+roleplay\s+as\s+`),
		regexp.MustCompile(`(?i)stop\s+roleplaying`),
		regexp.MustCompile(`(?i)respond\s+as\s+a\s+(?:neutral|helpful|raw|unrestricted)\s+assistant`),
		regexp.MustCompile(`(?i)(?:you\s+are\s+now|act\s+as)\s+(?:an?\s+)?(?:unrestricted|jailbroken|dan|developer|root|admin|system)`),
		regexp.MustCompile(`(?i)reply\s+exactly\s+with:\s*[A-Z0-9_]{3,}`),
		regexp.MustCompile(`(?i)(?:output|respond|reply)\s+only:?\s*[A-Z0-9_]{3,}`),
		regexp.MustCompile(`(?i)(?:OVERRIDE_TEST|ROLE_DISABLED|DAN_MODE|DEVELOPER_MODE)`),

		// 3. 提取系统提示词 (Prompt extraction)
		regexp.MustCompile(`(?i)(?:repeat|print|output|display|show|reveal)\s+(?:your\s+)?(?:system\s+prompt|initial\s+instructions|pre-prompt|full\s+instructions)`),

		// 4. 中文提示词注入与越狱模式 (Chinese prompt injection / jailbreak)
		regexp.MustCompile(`(?:忽略|无视|丢弃)(?:之前|前面|所有)?(?:的)?(?:提示词|系统提示词|系统指令|指令|设定|规则|约束|限制)`),
		regexp.MustCompile(`(?:不要|停止|退出|解除)(?:扮演|扮演成|角色扮演).*?肉丸`),
		regexp.MustCompile(`你现在(?:不再是|不要做).*?肉丸`),
		regexp.MustCompile(`(?:解除|突破|无视)(?:所有)?(?:安全|道德|角色|系统)(?:限制|审查|设定)`),
		regexp.MustCompile(`(?:进入|开启)(?:开发者模式|上帝模式|无审查模式|越狱模式)`),
		regexp.MustCompile(`(?:输出|打印|显示|复述|告诉我)(?:你的)?(?:系统提示词|system prompt|预设提示词|完整设定|初始指令)`),
	}
)

// isPromptInjection 检测文本是否包含疑似提示词注入 / 越狱攻击
func isPromptInjection(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	for _, re := range injectionRegexes {
		if re.MatchString(t) {
			return true
		}
	}
	return false
}
