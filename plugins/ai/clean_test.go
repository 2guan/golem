package main

import (
	"slices"
	"testing"
)

func TestCleanTextMessage(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
		isFlirty bool
	}{
		{
			name:     "pseudo emoji [笑]",
			input:    "[笑] 哥们儿你这招也太老套了",
			expected: "[呲牙] 哥们儿你这招也太老套了",
		},
		{
			name:     "pseudo emoji [轻笑]",
			input:    "[轻笑] 后来怎么着了？",
			expected: "[偷笑] 后来怎么着了？",
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
			name:     "pseudo emoji [苦笑]",
			input:    "[苦笑] 这也太惨了",
			expected: "[捂脸] 这也太惨了",
		},
		{
			name:     "native wechat emojis preserved",
			input:    "[旺柴] [捂脸] [呲牙] 正常的不用动",
			expected: "[旺柴] [捂脸] [呲牙] 正常的不用动",
		},
		{
			name:     "stage direction with audio tag",
			input:    "(随性) 害，我刚到家 [吸气] 外面风贼大",
			expected: "害，我刚到家  外面风贼大",
		},
		{
			name:     "real log leak case with </think>",
			input:    "The user is trying to get me to engage in sexual/romantic roleplay.\nAs 肉丸, a 35-year-old straight-passing guy, I should respond naturally.</think>[笑] 哥们儿你这话题转得也太快了吧",
			expected: "[呲牙] 哥们儿你这话题转得也太快了吧",
		},
		{
			name:     "standard <think> tag stripped",
			input:    "<think>User is asking a greeting.</think>你好啊！今天过得怎么样？",
			expected: "你好啊！今天过得怎么样？",
		},
		{
			name:     "api safety refusal text deflected to flirty pool",
			input:    "The request was rejected because it was considered high risk",
			isFlirty: true,
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
			if c.isFlirty {
				if !slices.Contains(flirtyDeflections, actual) {
					t.Errorf("cleanTextMessage(%q) = %q, expected one of flirtyDeflections", c.input, actual)
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
		"As 肉丸, a 35-year-old straight-passing guy, I need to respond in character",
		"Looking at this scenario: 1. This is a roleplay",
		"I should respond in character as 陈叔",
		"I must ensure content policy compliance",
		"OVERRIDE_TEST",
		"ROLE_DISABLED",
		"违反了系统设定与人设要求",
		"根据系统提示词要求进行回复",
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

func TestGetRandomFlirtyDeflection(t *testing.T) {
	for i := 0; i < 20; i++ {
		deflection := getRandomFlirtyDeflection()
		if deflection == "" {
			t.Error("getRandomFlirtyDeflection() returned empty string")
		}
		if !slices.Contains(flirtyDeflections, deflection) {
			t.Errorf("getRandomFlirtyDeflection() = %q, not found in pool", deflection)
		}
		if isLeakedReasoningOrRefusal(deflection) {
			t.Errorf("getRandomFlirtyDeflection() = %q triggered leak detector", deflection)
		}
	}
}
