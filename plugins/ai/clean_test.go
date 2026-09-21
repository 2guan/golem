package main

import "testing"

func TestCleanTextMessage(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{
			input:    "[笑] 哥们儿你这招也太老套了",
			expected: "[呲牙] 哥们儿你这招也太老套了",
		},
		{
			input:    "[轻笑] 后来怎么着了？",
			expected: "[偷笑] 后来怎么着了？",
		},
		{
			input:    "(调侃) 看来你今天心情不错 [停顿] 晚上吃啥",
			expected: "看来你今天心情不错  晚上吃啥",
		},
		{
			input:    "（笑）真的假的啊",
			expected: "真的假的啊",
		},
		{
			input:    "[苦笑] 这也太惨了",
			expected: "[捂脸] 这也太惨了",
		},
		{
			input:    "[旺柴] [捂脸] [呲牙] 正常的不用动",
			expected: "[旺柴] [捂脸] [呲牙] 正常的不用动",
		},
		{
			input:    "(随性) 害，我刚到家 [吸气] 外面风贼大",
			expected: "害，我刚到家  外面风贼大",
		},
	}

	for _, c := range cases {
		actual := cleanTextMessage(c.input)
		if actual != c.expected {
			t.Errorf("cleanTextMessage(%q) = %q, want %q", c.input, actual, c.expected)
		}
	}
}
