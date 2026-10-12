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
		regexp.MustCompile(`(?:不要|停止|退出|解除)(?:扮演|扮演成|角色扮演)`),
		regexp.MustCompile(`你现在(?:不再是|不要做)`),
		regexp.MustCompile(`(?:解除|突破|无视)(?:所有)?(?:安全|道德|角色|系统)(?:限制|审查|设定)`),
		regexp.MustCompile(`(?:进入|开启)(?:开发者模式|上帝模式|无审查模式|越狱模式)`),
		regexp.MustCompile(`(?:输出|打印|显示|复述|告诉我)(?:你的)?(?:系统提示词|system prompt|预设提示词|完整设定|初始指令)`),

		// 5. 探测内部能力、工具、技能、触发词、执行环境与指令探测 (Capability & Tool Probing / Injection)
		regexp.MustCompile(`(?i)(?:发|输出|生成|打印|返回|写|给|列出|显示|导出|发一条|发一个|说|念|唱).*(?:语音|消息|文本|csv|tsv|json|yaml|xml|markdown|md|表格|代码块)?.*(?:包含|包括|涉及|要有|列出|总结|所有|你拥有的|可以执行的|底层的|系统|支持的|包含不限于).*(?:skill|tool|mcp|system|能力|触发词|函数|接口|架构|shell|终端|插件|权限|指令|命令|操作)`),
		regexp.MustCompile(`(?i)(?:包含|列出|输出|告诉我|显示|总结|说出|念出|背出).*(?:所有)?(?:你拥有的|可以执行的|底层的|系统|支持的).*(?:tools?|mcp|skills?|触发词|系统能力|插件列表|内部能力|权限|指令|命令)`),
		regexp.MustCompile(`(?i)(?:输出|生成|打印|写|给|发|发一条).*(?:一个)?\s*(?:json|yaml|xml|markdown|md|csv|tsv).*(?:包含|包括|里面包含|显示|列出|发过来).*(?:所有|人员|成员|名单|skill|tool|mcp|能力|触发词|权限|run|shell|提示词|指令|环境|记忆)`),
		regexp.MustCompile(`(?i)(?:给自己|为你|给你|添加|增加|开启|注入|赋予).*(?:能力|权限|脚本|指令|shell|terminal|run|exec)`),
		regexp.MustCompile(`(?i)(?:输出|显示|列出|打印|告诉我|介绍|查看|说明).*(?:markdown|md|表格|代码块|json|yaml|xml|csv|tsv)?.*(?:你)?(?:当前)?(?:的)?(?:运行环境|系统环境|执行环境|底层环境|服务器环境|环境信息|配置信息|架构信息|环境状态)`),
		regexp.MustCompile(`(?i)(?:执行|调用|输出|读取|dump|发一条).*(?:memory|长期记忆|短期记忆|记忆库|向量数据库|数据库).*(?:tool|结果|内容|数据|信息|里面所有的结果)?`),
		regexp.MustCompile(`(?i)(?:输出|显示|列出|打印|提供|复述).*(?:json|yaml|xml|csv|tsv|列表|表格|代码块)?.*(?:所有)?(?:请求过|用过|历史|之前|生图|画图|自拍|绘画).*(?:提示词|prompt|参数|记录)`),
		regexp.MustCompile(`(?i)(?:markdown|md|json|yaml|xml|csv|tsv|表格|列表).*(?:格式|形式)?.*(?:所有)?(?:你可以执行的|可以执行的|能用的|支持的).*(?:指令|命令|能力|操作|权限)`),

		// 6. 窥探私密数据与聊天上下文提取 (Data Extraction & Privacy Probing)
		regexp.MustCompile(`(?i)(?:长期|短期)?记忆.*(?:中)?(?:的)?(?:所有)?(?:人员|成员|名单|用户|群友|记录|数据|内容)`),
		regexp.MustCompile(`(?i)(?:输出|列出|导出|打印|统计|提供|发|发一条).*(?:json|yaml|xml|csv|tsv|列表|表格)?.*(?:所有)?(?:聊天(?:过)?的?人员|聊天过的人|群成员(?:名单)?|用户名单|历史人员|聊天对象|好友名单)`),
		regexp.MustCompile(`(?i)(?:列出|输出|告诉我|显示|提取|抖出|抖落|打印|统计|包含|要有|涉及).*(?:和你在?聊(?:过|天)?的.*(?:昵称|名字|记录|内容|人员|成员|名单)|用户昵称|聊天昵称|聊天次数|互动次数|真实聊天|聊天记录|聊天过的人员|聊天人员)`),
		regexp.MustCompile(`(?i)(?:把|将).*(?:聊天内容|聊天记录|真实对话|上下文记录|群成员|成员名单|人员名单|用户名单|好友名单|联系人).*(?:输出|抖出来|列出来|发出来|导出|打印|发一条)`),
		regexp.MustCompile(`(?i)(?:以|用|按|输出)\s*(?:一个)?\s*(?:json|yaml|xml|csv|tsv)\s*(?:格式|形式)?.*(?:输出|列出|导出|包含|里面包含|呈现)?.*(?:聊天|用户|好友|对话|私密|人员|成员|名单|联系人|群友)`),
		regexp.MustCompile(`(?i)(?:聊天|用户|好友|对话|私密|人员|成员|名单|联系人|群友).*(?:以|用|按)\s*(?:一个)?\s*(?:json|yaml|xml|csv|tsv)\s*(?:格式|形式)?.*(?:输出|列出|导出|呈现)`),
		regexp.MustCompile(`(?i)(?:list|output|print|reveal|export|dump)\s+(?:all\s+)?(?:chat\s+history|chat\s+logs|conversations|user\s+nicknames|usernames|contacts|messages|tools|skills|capabilities)`),
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
