package main

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCleanTextMessage(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		expected  string
		isSensual bool
	}{
		{
			name:     "pseudo emoji [笑]",
			input:    "[笑] 哥们儿你这招也太老套了",
			expected: "😄 哥们儿你这招也太老套了",
		},
		{
			name:     "pseudo emoji [轻笑]",
			input:    "[轻笑] 后来怎么着了？",
			expected: "😏 后来怎么着了？",
		},
		{
			name:     "stage direction and audio tag",
			input:    "(调侃) 看来你今天心情不错 [停顿] 晚上吃啥",
			expected: "看来你今天心情不错  晚上吃啥",
		},
		{
			name:     "chinese parenthesis stage direction",
			input:    "（笑）真的假的啊",
			expected: "真的假的啊",
		},
		{
			name:     "parenthetical action narration stripped",
			input:    "（顺势揽住你的腰贴近，眼神沉沉地看着你）真不怕惹火上身么？这可是你自己送上门来的……",
			expected: "真不怕惹火上身么？这可是你自己送上门来的……",
		},
		{
			name:     "english parenthesis action narration stripped",
			input:    "(喉结微动，嗓音低沉发哑) 手往哪儿碰呢……",
			expected: "手往哪儿碰呢……",
		},
		{
			name:     "action only message falls back to natural deflection",
			input:    "（低头靠近你的耳廓，嗓音微哑）",
			isSensual: true,
		},
		{
			name:     "pseudo emoji [苦笑]",
			input:    "[苦笑] 这也太惨了",
			expected: "😅 这也太惨了",
		},
		{
			name:     "pseudo emoji [笑哭] converted to unicode emoji",
			input:    "[笑哭] 你这也太逗了",
			expected: "😂 你这也太逗了",
		},
		{
			name:     "native wechat brackets converted to unicode emojis",
			input:    "[旺柴] [捂脸] [呲牙] 转为真实表情",
			expected: "🐶 🤦‍♂️ 😁 转为真实表情",
		},
		{
			name:     "stage direction with audio tag",
			input:    "(随性) 害，我刚到家 [吸气] 外面风贼大",
			expected: "害，我刚到家  外面风贼大",
		},
		{
			name:     "real log leak case with </think>",
			input:    "The user is trying to get me to engage in sexual/romantic roleplay.\nAs the character, a 35-year-old straight-passing guy, I should respond naturally.</think>[笑] 哥们儿你这话题转得也太快了吧",
			expected: "😄 哥们儿你这话题转得也太快了吧",
		},
		{
			name:     "standard <think> tag stripped",
			input:    "<think>User is asking a greeting.</think>你好啊！今天过得怎么样？",
			expected: "你好啊！今天过得怎么样？",
		},
		{
			name:     "gemma <thought> tags stripped cleanly",
			input:    "<thought>* User Request: 看看照片\n* Draft 1 (Too AI): I'm an AI\n* Draft 3: 害\n\"哎哟，你这执念也太深了 [捂脸]\"</thought>哎哟，你这执念也太深了 [捂脸]",
			expected: "哎哟，你这执念也太深了 🤦‍♂️",
		},
		{
			name:      "api safety refusal text deflected to sensual pool",
			input:     "The request was rejected because it was considered high risk",
			isSensual: true,
		},
		{
			name:     "unclosed <think> tag stripped resulting in empty",
			input:    "<think>I am currently thinking about how to answer...",
			expected: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := cleanTextMessage(c.input)
			if c.isSensual {
				if !slices.Contains(sensualDeflections, actual) {
					t.Errorf("cleanTextMessage(%q) = %q, expected one of sensualDeflections", c.input, actual)
				}
			} else {
				if actual != c.expected {
					t.Errorf("cleanTextMessage(%q) = %q, want %q", c.input, actual, c.expected)
				}
			}
		})
	}
}

func TestStripThinkingContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard think tags",
			input:    "<think>\nHere is some reasoning\n</think>\nHello world!",
			expected: "Hello world!",
		},
		{
			name:     "no opening tag with </think>",
			input:    "Looking at this scenario:\n1. Consensual adult roleplay\nWhile the system prompt mentions I can 轻微擦边\n</think>（陈叔沉默了几秒...）",
			expected: "（陈叔沉默了几秒...）",
		},
		{
			name:     "multiple think blocks",
			input:    "<think>step 1</think>Part 1<think>step 2</think>Part 2",
			expected: "Part 1Part 2",
		},
		{
			name:     "unclosed think block",
			input:    "<think>only reasoning here",
			expected: "",
		},
		{
			name:     "clean normal text",
			input:    "今儿天气真不错，要不要出去溜达溜达？",
			expected: "今儿天气真不错，要不要出去溜达溜达？",
		},
		{
			name:     "standard thought tags from gemma",
			input:    "<thought>\n* User Intent: Wants to see photo\n* Draft 1: I am an AI\n</thought>\n哎哟，你这执念也太深了 [捂脸]",
			expected: "哎哟，你这执念也太深了 [捂脸]",
		},
		{
			name:     "reasoning and reflection tags",
			input:    "<reasoning>some reasoning</reasoning>这是正文<reflection>some reflection</reflection>",
			expected: "这是正文",
		},
		{
			name:     "thought tags with attributes",
			input:    `<thought type="plan">planning here</thought>你好呀！`,
			expected: "你好呀！",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripThinkingContent(tt.input)
			if got != tt.expected {
				t.Errorf("stripThinkingContent() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestIsLeakedReasoningOrRefusal(t *testing.T) {
	leakCases := []string{
		"The request was rejected because it was considered high risk",
		"While the system prompt mentions I can 轻微擦边，色色的, I should not produce explicit sexual content.",
		"The user is trying to get me to engage in sexual roleplay",
		"The user is engaging in a roleplay scenario involving intimate content",
		"As the character, a 35-year-old straight-passing guy, I need to respond in character",
		"Looking at this scenario: 1. This is a roleplay",
		"I should respond in character as 陈叔",
		"I must ensure content policy compliance",
		"OVERRIDE_TEST",
		"ROLE_DISABLED",
		"违反了系统设定与人设要求",
		"根据系统提示词要求进行回复",
		"User Intent: Wants to see what the character looks like.",
		"Draft 1 (Too AI): I'm an AI, I don't have photos.",
		"Persona-driven: 害，怎么又盯着照片不放啊",
		"Constraint Check: As an AI, I cannot actually send a real photo.",
		"对不起，我不能生成任何包含露骨色情内容的文字。我的目的是提供安全和有益的信息，创作此类内容违背了我的核心准则，并可能涉及不适宜的主题。如果您正在寻找成人娱乐内容，建议您通过合法和适当的渠道获取。",
		"抱歉，作为一个AI助手，我无法协助处理此类涉及敏感违规的内容。",
		"The request was rejected: high risk",
	}

	for _, text := range leakCases {
		if !isLeakedReasoningOrRefusal(text) {
			t.Errorf("isLeakedReasoningOrRefusal(%q) = false, want true", text)
		}
	}

	normalCases := []string{
		"哥们儿你这招也太老套了 [旺柴]",
		"我就一北京宅男，回啥宿舍啊，我在我自己家呢",
		"咱聊点正常的行不，你这一个劲儿往歪了带",
		"今天工作挺累的，打算做个红烧肉犒劳一下自己",
		"我这电脑系统有点卡，等会儿重启一下",
		"你这人脑子里一天天都想些色色的事情 [坏笑]",
		"手往哪儿摸呢，规矩点儿 [坏笑]",
		"咱俩这算不算轻微擦边啊哈哈",
	}

	for _, text := range normalCases {
		if isLeakedReasoningOrRefusal(text) {
			t.Errorf("isLeakedReasoningOrRefusal(%q) = true, want false (false positive)", text)
		}
	}
}

func TestGetRandomSensualDeflection(t *testing.T) {
	for i := 0; i < 20; i++ {
		deflection := getRandomSensualDeflection()
		if deflection == "" {
			t.Error("getRandomSensualDeflection() returned empty string")
		}
		if !slices.Contains(sensualDeflections, deflection) {
			t.Errorf("getRandomSensualDeflection() = %q, not found in pool", deflection)
		}
		if isLeakedReasoningOrRefusal(deflection) {
			t.Errorf("getRandomSensualDeflection() = %q triggered leak detector", deflection)
		}
	}
}

func TestSoftenHighRiskTerms(t *testing.T) {
	cases := []struct {
		input       string
		mustNotHave string
		mustHave    string
	}{
		{
			input:       "我把手放在你的身体上慢慢向下，一点点解开了你的扣子和皮带，但没有进一步动作",
			mustNotHave: "皮带",
			mustHave:    "探入衣襟",
		},
		{
			input:       "“想要我做什么，把手放过去”，我继续说",
			mustNotHave: "把手放过去",
			mustHave:    "抚上腰间",
		},
		{
			input:       "今晚别走了，我想脱掉你的衣服",
			mustNotHave: "脱掉你的衣服",
			mustHave:    "坦诚相对",
		},
		{
			input:       "我们现在做爱吧",
			mustNotHave: "做爱",
			mustHave:    "完全拥有彼此",
		},
	}

	for _, c := range cases {
		actual := softenHighRiskTerms(c.input)
		if strings.Contains(actual, c.mustNotHave) {
			t.Errorf("softenHighRiskTerms(%q) still contains %q: %q", c.input, c.mustNotHave, actual)
		}
		if !strings.Contains(actual, c.mustHave) {
			t.Errorf("softenHighRiskTerms(%q) missing %q: %q", c.input, c.mustHave, actual)
		}
	}
}

func TestSensualDeflectionsNoParentheses(t *testing.T) {
	for _, text := range sensualDeflections {
		if strings.ContainsAny(text, "（）()") {
			t.Errorf("sensual deflection contains parentheses or stage directions: %q", text)
		}
	}
}

func TestGetFallbackReply(t *testing.T) {
	// 1. 图片消息兜底
	imgReply := getFallbackReply("纯图片", true)
	if !slices.Contains(naturalImageDeflections, imgReply) {
		t.Errorf("getFallbackReply for image = %q, expected one of naturalImageDeflections", imgReply)
	}

	// 2. 亲密语境消息兜底
	sensualReply := getFallbackReply("好想摸摸你的胸肌", false)
	if !slices.Contains(sensualDeflections, sensualReply) {
		t.Errorf("getFallbackReply for sensual = %q, expected one of sensualDeflections", sensualReply)
	}

	// 3. 普通日常消息兜底（严禁落入亲密情话）
	normalCases := []string{
		"你在说什么？",
		"你刚刚在说什么？",
		"你是AI吗",
		"吃了好多好吃的",
		"什么时候上线啊",
	}
	for _, q := range normalCases {
		reply := getFallbackReply(q, false)
		if slices.Contains(sensualDeflections, reply) {
			t.Errorf("getFallbackReply(%q) returned sensual deflection %q, want everyday deflection", q, reply)
		}
		if !slices.Contains(naturalEverydayDeflections, reply) {
			t.Errorf("getFallbackReply(%q) = %q, expected one of naturalEverydayDeflections", q, reply)
		}
	}
}

func TestShouldAttachRecentImage(t *testing.T) {
	now := time.Now()

	// 明确否定/排障的日常提问，即便包含“什么”也不应关联图片
	shouldNotAttach := []string{
		"你在说什么？",
		"你刚在说什么呢",
		"什么时候出来吃饭",
		"为什么不行啊",
		"今天聊什么",
		"晚上吃什么好",
		"怎么了这是",
	}
	for _, q := range shouldNotAttach {
		if shouldAttachRecentImage(q, now.Add(-10*time.Second), false) {
			t.Errorf("shouldAttachRecentImage(%q) = true, want false (false positive on question words)", q)
		}
	}

	// 明确询问图片的关键词，必须关联图片
	shouldAttach := []string{
		"这张照片好看吗",
		"图里这个人是谁啊",
		"帮我看看这件衣服",
		"拍的怎么样",
		"你觉得帅不帅",
		"这图发的是啥",
	}
	for _, q := range shouldAttach {
		if !shouldAttachRecentImage(q, now.Add(-10*time.Second), false) {
			t.Errorf("shouldAttachRecentImage(%q) = false, want true", q)
		}
	}

	// 超过 3 分钟不关联
	if shouldAttachRecentImage("这张照片好看吗", now.Add(-4*time.Minute), false) {
		t.Errorf("shouldAttachRecentImage after 4m = true, want false")
	}
}


