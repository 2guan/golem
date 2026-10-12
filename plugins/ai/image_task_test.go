package main

import (
	"testing"
)

func TestExtractImageTasks(t *testing.T) {
	tests := []struct {
		input       string
		expectedLen int
		expectedRef string
		expectedPrm string
		cleanText   string
	}{
		{
			input:       "刚在故宫角楼溜达呢，给你拍一张。<image ref=\"selfie\" prompt=\"故宫角楼护城河边，阳光明媚，真实生活感手机自拍\"/>",
			expectedLen: 1,
			expectedRef: "selfie",
			expectedPrm: "故宫角楼护城河边，阳光明媚，真实生活感手机自拍",
			cleanText:   "刚在故宫角楼溜达呢，给你拍一张。",
		},
		{
			input:       "给你画了一张猫咪。<image prompt=\"一只橘猫在草地上\"/>",
			expectedLen: 1,
			expectedRef: "",
			expectedPrm: "一只橘猫在草地上",
			cleanText:   "给你画了一张猫咪。",
		},
		{
			input:       "<image ref=\"last\" prompt=\"变成赛博朋克风格\"/>改好啦！",
			expectedLen: 1,
			expectedRef: "last",
			expectedPrm: "变成赛博朋克风格",
			cleanText:   "改好啦！",
		},
		{
			input:       "<image ref='selfie'>什刹海夕阳自拍</image>",
			expectedLen: 1,
			expectedRef: "selfie",
			expectedPrm: "什刹海夕阳自拍",
			cleanText:   "",
		},
		{
			input:       "刚练完腿。<br/>[我发送了一张生活自拍照，场景在：健身房力量训练区，刚练完腿大汗淋漓，真实生活感手机自拍]",
			expectedLen: 1,
			expectedRef: "selfie",
			expectedPrm: "健身房力量训练区，刚练完腿大汗淋漓，真实生活感手机自拍",
			cleanText:   "刚练完腿。<br/>",
		},
		{
			input:       "[我发送了一张绘制的图片，画面内容是：一只可爱的小猫咪在阳光下睡觉]",
			expectedLen: 1,
			expectedRef: "",
			expectedPrm: "一只可爱的小猫咪在阳光下睡觉",
			cleanText:   "",
		},
	}

	for i, tt := range tests {
		tasks, clean := extractImageTasks(tt.input)
		if len(tasks) != tt.expectedLen {
			t.Errorf("test %d: expected %d tasks, got %d", i, tt.expectedLen, len(tasks))
			continue
		}
		if tasks[0].Ref != tt.expectedRef {
			t.Errorf("test %d: expected ref %q, got %q", i, tt.expectedRef, tasks[0].Ref)
		}
		if tasks[0].Prompt != tt.expectedPrm {
			t.Errorf("test %d: expected prompt %q, got %q", i, tt.expectedPrm, tasks[0].Prompt)
		}
		if clean != tt.cleanText {
			t.Errorf("test %d: expected clean text %q, got %q", i, tt.cleanText, clean)
		}
	}
}
