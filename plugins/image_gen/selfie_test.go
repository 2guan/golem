package main

import (
	"strings"
	"testing"
)

func TestBuildSelfiePrompt(t *testing.T) {
	g := newImageGenerator(defaultConfig())

	// 1. 默认随机场景
	prompt, scene := g.BuildSelfiePrompt("")
	if scene == "" {
		t.Errorf("scene should not be empty")
	}
	if !strings.Contains(prompt, scene) {
		t.Errorf("prompt should contain scene %q", scene)
	}
	if !strings.Contains(prompt, "微胖成熟男性") {
		t.Errorf("prompt should contain persona characteristics")
	}

	// 2. 指定自定义场景
	custom := "天安门广场"
	prompt2, scene2 := g.BuildSelfiePrompt(custom)
	if scene2 != custom {
		t.Errorf("expected scene %q, got %q", custom, scene2)
	}
	if !strings.Contains(prompt2, custom) {
		t.Errorf("prompt should contain custom scene")
	}
}

func TestMatchSelfieTrigger(t *testing.T) {
	p := &ImageGenPlugin{}

	matches := []string{
		"自拍",
		"来张自拍",
		"发张自拍",
		"拍张照",
		"来张在故宫的自拍",
		"发张海边的自拍",
	}

	for _, m := range matches {
		isSelfie, _ := p.matchSelfieTrigger(m)
		if !isSelfie {
			t.Errorf("expected %q to match selfie trigger", m)
		}
	}

	nonMatches := []string{
		"你觉得自拍好看吗？",
		"今天天气真好",
		"生图 一只猫",
	}

	for _, nm := range nonMatches {
		isSelfie, _ := p.matchSelfieTrigger(nm)
		if isSelfie {
			t.Errorf("expected %q to NOT match selfie trigger", nm)
		}
	}
}

func TestMatchImageGenTrigger(t *testing.T) {
	p := &ImageGenPlugin{}

	isGen, prompt := p.matchImageGenTrigger("画图 一只小狗")
	if !isGen || prompt != "一只小狗" {
		t.Errorf("expected prompt '一只小狗', got %q", prompt)
	}

	isGen2, prompt2 := p.matchImageGenTrigger("生图：可爱的仓鼠")
	if !isGen2 || prompt2 != "可爱的仓鼠" {
		t.Errorf("expected prompt '可爱的仓鼠', got %q", prompt2)
	}

	isGen3, _ := p.matchImageGenTrigger("你好啊")
	if isGen3 {
		t.Errorf("should not match ordinary text")
	}
}
