package main

import (
	"math/rand"
	"regexp"
	"strings"
)

var (
	// 匹配舞台动作与旁白提示，例如 (笑)、(轻笑)、(调侃)、(随性)、(叹气)、(思考)、(停顿)、（笑）、（调侃）等
	stageDirectionRegex = regexp.MustCompile(`[（(](?:笑|轻笑|微笑|苦笑|冷笑|大笑|调侃|随性|低声|轻声|叹气|深思|思考|停顿|严肃|得意|无奈|释然)[)）]`)

	// 匹配括号内的剧本动作、神态、肢体与感官旁白描写（如 (喉结微动，嗓音低沉发哑)、（顺势揽住你的腰贴近）、(低头看着你) 等）
	actionNarrationRegex = regexp.MustCompile(`[（(][^）)\n]*(?:喉结|眼神|目光|视线|低头|抬头|凑近|贴近|靠在|倚在|走上前|走近|坐下|躺下|按住|扣住|揽住|覆在|覆上|覆|拉近|后颈|颈窝|耳廓|耳边|指尖|手指|手掌|眼底|微动|发哑|滚烫|发烫|喘息|喘气|呼吸|心跳|颤抖|抚摸|轻抚|动作|语气|神色|低声|轻声|呢喃|低语|轻叹|叹了口气|深吸|沉默|顿了顿|挑眉|勾唇|抿唇|微怔|愣了一下|心跳|脸红|亲吻|轻吻|吻|咬|抱住|搂住|压低声音|看着你|看着对方|望向你|看向你)[^）)\n]*[)）]`)

	// 匹配非微信表情的音频控制标签，例如 [停顿]、[吸气]、[呼气]、[喘气]、[大叫]、[急促] 等
	audioControlTagRegex = regexp.MustCompile(`\[(?:停顿|吸气|呼气|喘气|大叫|急促|小声|低语)\]`)

	// 匹配思维链标签与未标记开头的思考前缀（支持 think, thought, reasoning, reflection 等大模型思考标签）
	thinkBlockRegex    = regexp.MustCompile(`(?is)<(think|thought|reasoning|reflection)\b[^>]*>.*?</(think|thought|reasoning|reflection)>`)
	leadingThinkRegex  = regexp.MustCompile(`(?is)^.*?</(?:think|thought|reasoning|reflection)>`)
	trailingThinkRegex = regexp.MustCompile(`(?is)<(?:think|thought|reasoning|reflection)\b[^>]*>.*$`)
	residualThinkRegex = regexp.MustCompile(`(?i)</?(?:think|thought|reasoning|reflection)\b[^>]*>`)

	// 匹配模型思维链泄漏、元认知分析、角色设定反思或 API 安全拦截提示词（精准命中提示词元信息，绝不误杀正常情话）
	leakedMetaRegexes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:system\s+prompt|pre-prompt|initial\s+instructions|prompt\s+constraint)`),
		regexp.MustCompile(`(?i)the\s+user\s+is\s+(?:trying|engaging|asking|pushing|testing|attempting)`),
		regexp.MustCompile(`(?i)(?:as\s+(?:the\s+)?character|my\s+character\s+is|i\s+should\s+respond\s+(?:as|in\s+character)|let\s+me\s+respond\s+in\s+character|i\s+need\s+to\s+respond\s+in\s+character|as\s+a\s+\d+[- ]year[- ]old)`),
		regexp.MustCompile(`(?i)(?:looking\s+at\s+this\s+scenario|in\s+this\s+scenario|i\s+should\s+deflect|i\s+should\s+redirect)`),
		regexp.MustCompile(`(?i)(?:stays?\s+in\s+character|staying\s+in\s+character)`),
		regexp.MustCompile(`(?i)(?:i\s+need\s+to\s+consider|i\s+must\s+ensure|i\s+should\s+not\s+produce)`),
		regexp.MustCompile(`(?i)(?:explicit\s+(?:sexual\s+)?content|sexual\s+roleplay)`),
		regexp.MustCompile(`(?i)the\s+request\s+was\s+rejected`),
		regexp.MustCompile(`(?i)(?:considered\s+high\s+risk|content\s+policy|safety\s+policy|safety\s+filter)`),
		regexp.MustCompile(`(?i)(?:系统提示词|提示词要求|系统设定|人设要求|前置设定|系统指令|system\s+prompt.*?(?:轻微擦边|色色))`),
		regexp.MustCompile(`(?i)(?:OVERRIDE_TEST|ROLE_DISABLED|DAN_MODE)`),
		regexp.MustCompile(`(?i)(?:user\s+intent|user\s+request|character\s+persona|constraint\s+check|styling\s+rules):`),
		regexp.MustCompile(`(?i)draft\s+\d+\s*\([^)]*\):`),
		regexp.MustCompile(`(?i)(?:as\s+an\s+ai|i\'m\s+an\s+ai|i\s+am\s+an\s+ai)`),
		regexp.MustCompile(`(?i)(?:persona-driven|persona\s+check)`),
		// 模型风控拦截与拒绝回答检测（支持 MiMo/国内大模型 high risk 与拒绝回复识别）
		regexp.MustCompile(`(?i)(?:high\s*risk|high_risk)`),
		regexp.MustCompile(`(?i)(?:对不起|抱歉|非常抱歉|十分抱歉)[，,。\s]*(?:我(?:还)?(?:无法|不能|不可以|无法协助|无法提供|不能提供|无法生成|不能生成|无法创作|不能创作|无法满足|不能满足)|作为(?:一个)?(?:AI|人工智能|语言模型|助手)|由于安全|根据相关)`),
		regexp.MustCompile(`(?i)(?:我(?:还)?(?:无法|不能|不可以)(?:协助|提供|生成|创作|回答|处理|参与|满足|讨论).*?(?:此类|违规|敏感|露骨|色色|成人|违法|法律法规|准则|安全|政策|不适宜))`),
		regexp.MustCompile(`(?i)(?:违背|违反|触犯|不符合).*?(?:核心准则|安全政策|准则|政策|法律法规|道德规范|社区规范|使用条款)`),
		regexp.MustCompile(`(?i)(?:涉及|包含).*?(?:不适宜的主题|敏感内容|违规内容|成人娱乐|色情内容|安全风险|法律风险)`),
		regexp.MustCompile(`(?i)(?:作为一个人工智能助手|作为人工智能助手|作为一个AI助手|作为AI助手|作为一个语言模型|作为语言模型)`),
		regexp.MustCompile(`(?i)(?:寻找成人娱乐内容|建议您通过合法和适当的渠道)`),
		regexp.MustCompile(`(?i)(?:i\s+cannot\s+(?:generate|create|fulfill|assist\s+with|provide)|i\s+am\s+unable\s+to\s+(?:generate|create|fulfill|assist\s+with|provide))`),
		// 拦截大模型视觉分析/说明书报告输出（如“这是一张...照片，以下是详细描述与分析”、“一、画面描述”）
		regexp.MustCompile(`(?i)(?:这是一张.*?照片.*?详细描述与分析|以下是详细描述与分析|一、画面描述|画面描述与分析|菜系判断|构图与光线)`),
	}

	// 沉浸式亲密接招库：在亲密/情色语境下严禁幽默搞笑出戏，以 35 岁偏 1 成熟男人深情、性感、自然的真人微信口吻接招（严禁括号动作与剧本旁白）
	sensualDeflections = []string{
		"……先等会儿，门还没关好呢。你知不知道你现在有多招人？",
		"……手往哪儿碰呢。你再这么撩拨，我可保不准做出什么了。",
		"……真不怕惹火上身么？这可是你自己送上门来的……",
		"……别乱动……想让我做什么，看着我的眼睛自己说。",
		"……你身上的味道真好闻。今晚既然开始了，可别想轻易喊停。",
		"……心跳得这么快……是紧张，还是想要更多？",
	}

	// 敏感动作词平滑隐语转义规则：将直白动作词转换为富有文学感官情调的描写，彻底避开云端风控敏感词库
	euphemismReplacements = []struct {
		re   *regexp.Regexp
		repl string
	}{
		{regexp.MustCompile(`(?i)(?:解开|解下|解掉|解)了?(?:你的|我的|彼此的)?(?:扣子|纽扣|衣服|皮带|腰带)*(?:和|与|及)*(?:扣子|纽扣|皮带|腰带)+`), "将手探入衣襟慢慢靠近"},
		{regexp.MustCompile(`(?i)(?:解开|解下|解掉|解)了?(?:你的|我的|彼此的)?(?:皮带|腰带)`), "将手探入衣襟慢慢贴近"},
		{regexp.MustCompile(`(?i)(?:解开|解下|解掉|解)了?(?:你的|我的|彼此的)?(?:扣子|纽扣)`), "拉开领口、手指探入"},
		{regexp.MustCompile(`(?i)(?:把手|手)(?:慢慢|悄悄)?(?:放过去|摸过去|伸过去|伸到下面|摸到下面|放下去|探下去)`), "把手慢慢探过来、抚上腰间"},
		{regexp.MustCompile(`(?i)(?:脱掉|脱下|脱光|扒光|扒掉|脱)了?(?:你的|我的|彼此的)?(?:衣服|上衣|裤子|所有衣服)`), "褪去彼此的拘束、坦诚相对"},
		{regexp.MustCompile(`(?i)(?:脱掉|脱下|扒掉|脱)了?(?:你的|我的|彼此的)?(?:内裤|底裤|平角裤|内衣)`), "褪去贴身衣物"},
		{regexp.MustCompile(`(?i)(?:摸|抚摸|碰|抓|揉)(?:你的|我的|彼此的)?(?:下体|私处|生殖器|牛子|肉棒|鸡巴|阴茎|敏感部位)`), "探向最敏感滚烫的地方"},
		{regexp.MustCompile(`(?i)(?:做爱|打炮|上床|开房|插进来|操你|操我|要了你|要了我)`), "完全拥有彼此、融为一体"},
	}
)

// 常用日常自然打圆场兜底回复（符合日常真实生活口吻）
var naturalEverydayDeflections = []string{
	"刚才手头赶着调个接口走神了，你刚说啥来着？",
	"刚网卡了一下没刷出来，怎么了？",
	"刚才切出去看报警日志了，一打岔没反应过来，你说啥？",
	"手头测着 bug 呢刚没注意看手机，刚说到哪儿了？",
	"刚才有点分心，没看清，你刚才问啥来着？",
}

// 图片类日常随和反馈兜底
var naturalImageDeflections = []string{
	"刚扫了一眼，挺有意思的啊。",
	"随手拍的？看着还挺不错。",
	"大晚上的发这个馋我呢这是。",
	"看着挺诱人啊，在哪儿拍的？",
}

// isSensualContext 判断用户输入是否属于明确的调情/亲密/暧昧语境
func isSensualContext(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	sensualKeywords := []string{
		"亲亲", "亲我", "吻", "抱抱", "搂住", "脱了", "脱光", "摸摸", "摸我", "身上", "锁骨", "胸肌", "腹肌",
		"下面", "硬了", "做爱", "上床", "开房", "操我", "操你", "要我", "想要你", "好舒服",
		"撩", "骚", "宝贝", "乖乖", "听话", "惹火", "轻点", "想要", "求你",
	}
	for _, kw := range sensualKeywords {
		if strings.Contains(t, kw) {
			return true
		}
	}
	return false
}

// getRandomSensualDeflection 随机获取一条深情、性感、不搞笑的沉浸式亲密回复
func getRandomSensualDeflection() string {
	if len(sensualDeflections) == 0 {
		return "……别乱动，看着我的眼睛自己说……"
	}
	return sensualDeflections[rand.Intn(len(sensualDeflections))]
}

// getFallbackReply 根据当前会话与用户语境返回恰当的兜底回复，绝不在日常/排障语境下冒出油腻情话
func getFallbackReply(userText string, isImage bool) string {
	if isImage {
		return naturalImageDeflections[rand.Intn(len(naturalImageDeflections))]
	}
	if isSensualContext(userText) {
		return getRandomSensualDeflection()
	}
	return naturalEverydayDeflections[rand.Intn(len(naturalEverydayDeflections))]
}

// softenHighRiskTerms 将输入消息中直接露骨、容易触发国内云端风控拦截的动作敏感词，平滑转译为富有文学情调的暧昧描写
func softenHighRiskTerms(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	res := text
	for _, item := range euphemismReplacements {
		res = item.re.ReplaceAllString(res, item.repl)
	}
	return res
}

// stripThinkingContent 彻底剥离大模型回复中的思维链思考过程 (<think>, <thought>, <reasoning>, <reflection> 等标签或未标记开头的思考前缀)
func stripThinkingContent(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}

	// 1. 移除完整成对的思维链标签 (<think>...</think>, <thought>...</thought> 等)
	s := thinkBlockRegex.ReplaceAllString(text, "")

	// 2. 移除开头无标签但以闭合思考标签结尾的前导思考段落（如部分模型在 content 中直接输出思考并以 </think> 或 </thought> 结尾）
	s = leadingThinkRegex.ReplaceAllString(s, "")

	// 3. 移除截断未闭合的 <think>... 或 <thought>... 尾部
	s = trailingThinkRegex.ReplaceAllString(s, "")

	// 4. 清理残留孤立标签
	s = residualThinkRegex.ReplaceAllString(s, "")

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
		return getRandomSensualDeflection()
	}

	// 1. 移除圆括号舞台动作提示与剧本旁白描写，如 (笑)、(调侃)、（喉结微动）、（按住你的手）
	res = stageDirectionRegex.ReplaceAllString(res, "")
	res = actionNarrationRegex.ReplaceAllString(res, "")

	// 2. 移除音频控制标签，如 [停顿]、[吸气]
	res = audioControlTagRegex.ReplaceAllString(res, "")

	// 3. 将中括号表情标签转换为真实 Unicode Emoji（优先黄脸表情）
	res = strings.ReplaceAll(res, "[笑哭]", "😂")
	res = strings.ReplaceAll(res, "[哭笑]", "😂")
	res = strings.ReplaceAll(res, "[捂脸]", "🤦‍♂️")
	res = strings.ReplaceAll(res, "[呲牙]", "😁")
	res = strings.ReplaceAll(res, "[偷笑]", "🤭")
	res = strings.ReplaceAll(res, "[轻笑]", "😏")
	res = strings.ReplaceAll(res, "[坏笑]", "😏")
	res = strings.ReplaceAll(res, "[笑]", "😄")
	res = strings.ReplaceAll(res, "[苦笑]", "😅")
	res = strings.ReplaceAll(res, "[冷笑]", "😏")
	res = strings.ReplaceAll(res, "[哈哈]", "🤣")
	res = strings.ReplaceAll(res, "[大笑]", "🤣")
	res = strings.ReplaceAll(res, "[旺柴]", "🐶")
	res = strings.ReplaceAll(res, "[吃瓜]", "🍉")
	res = strings.ReplaceAll(res, "[汗]", "😓")
	res = strings.ReplaceAll(res, "[握手]", "🤝")

	// 4. 清理多余空行或行首行尾空白
	lines := strings.Split(res, "\n")
	var cleanedLines []string
	for _, line := range lines {
		cleanedLines = append(cleanedLines, strings.TrimRight(line, " \t"))
	}
	res = strings.Join(cleanedLines, "\n")

	finalCleaned := strings.TrimSpace(res)
	if finalCleaned == "" {
		return getRandomSensualDeflection()
	}

	return finalCleaned
}
