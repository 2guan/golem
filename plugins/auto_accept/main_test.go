package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseVerifyMsg(t *testing.T) {
	rawXML := `<msg fromusername="muse851202" encryptusername="v3_020b3826fd030100000000009cf2053026a467000000501ea9a3dba12f95f6b60a0536a1adb6a3ee31aadbfb3af52e96733d563442de34715d1b906b5eee664e339a1015a0269007151d11d5a0825fcb121cd7@stranger" fromnickname="muse" content="我是muse" fullpy="muse" shortpy="MUSE" imagestatus="3" scene="17" country="CN" province="Beijing" city="Haidian" sign="" percard="1" sex="1" alias="" weibo="" albumflag="0" albumstyle="0" albumbgimgid="" snsflag="305" ticket="v4_000b708f0b040000010000000000b33e255ddc56cc91df4b7f8fac6a1000000050ded0b020927e3c97896a09d47e6e9ecade3cb0e3ebb2cab88811a1f145c122ba17dcdbc819e1bf60b079acc82f20fb8c81a294741a41c7760a2187cb838ae51a85d853367d2554de6562cf6b78783fcfe2d5d52f6d5cd12fbfd5907e8bd1d4854d508692a4f93c90e76fadcc9f46a0eb302f4577d22eff611a48ba9e990f255af975997334f5de5ae1944a527e0c2d8d66f7ea37203c28aa9e491a57652d34bf5a29b9bd1871d42ba676bc920d536cc38c6c41471e2d67b6ab696c61459c9ca19a9e142476109df0cdf26b7edff5e77f2c8d8ac359c6384c6f38f7f85ed9f8199283a3bcc4abf3bdfaee5d2ce8edeb8d4b30b4e106c71d233358f0012150a07a22dc9a40b3c58875@stranger" opcode="2" googlecontact="" qrticket="" chatroomusername="" sourceusername="wxid_recommender" sourcenickname="推荐人" sharecardusername="wxid_recommender" sharecardnickname="推荐人" cardversion="0" extflag="0"><brandlist count="0" ver="602420238"></brandlist></msg>`

	msg, err := parseVerifyMsg(rawXML)
	if err != nil {
		t.Fatalf("parseVerifyMsg failed: %v", err)
	}

	if msg.FromUsername != "muse851202" {
		t.Errorf("expected FromUsername muse851202, got %s", msg.FromUsername)
	}
	if msg.FromNickname != "muse" {
		t.Errorf("expected FromNickname muse, got %s", msg.FromNickname)
	}
	if msg.Content != "我是muse" {
		t.Errorf("expected Content 我是muse, got %s", msg.Content)
	}
	if msg.Scene != 17 {
		t.Errorf("expected Scene 17, got %d", msg.Scene)
	}
	if msg.SourceNickname != "推荐人" {
		t.Errorf("expected SourceNickname 推荐人, got %s", msg.SourceNickname)
	}
	if !strings.HasPrefix(msg.EncryptUsername, "v3_") {
		t.Errorf("expected v3 encryptusername, got %s", msg.EncryptUsername)
	}
	if !strings.HasPrefix(msg.Ticket, "v4_") {
		t.Errorf("expected v4 ticket, got %s", msg.Ticket)
	}
}

func TestBuildGreeting(t *testing.T) {
	p := &AutoAcceptPlugin{}

	// Default greeting with sourceNickname
	greeting := p.buildGreeting("小明", "张三")
	if !strings.Contains(greeting, "小明") {
		t.Errorf("greeting should contain nickname, got: %s", greeting)
	}
	if !strings.Contains(greeting, "我是张三介绍的Bot。") {
		t.Errorf("greeting should mention '我是张三介绍的Bot。', got: %s", greeting)
	}

	// Default greeting without sourceNickname (falls back to 推荐人)
	greetingNoSource := p.buildGreeting("小红", "")
	if !strings.Contains(greetingNoSource, "我是推荐人介绍的Bot。") {
		t.Errorf("greeting should mention '我是推荐人介绍的Bot。', got: %s", greetingNoSource)
	}

	// Empty nickname
	greetingEmpty := p.buildGreeting("", "张三")
	if greetingEmpty != "你好，我是张三介绍的Bot。" {
		t.Errorf("unexpected empty nickname greeting: %s", greetingEmpty)
	}

	// Custom greeting
	p.Config.CustomGreeting = "你好 {nickname}，{time_greeting}！我是 {bot}。"
	custom := p.buildGreeting("李雷", "张三")
	if !strings.Contains(custom, "李雷") || !strings.Contains(custom, "我是 Bot。") {
		t.Errorf("unexpected custom greeting: %s", custom)
	}
}

func TestDedup(t *testing.T) {
	p := &AutoAcceptPlugin{
		dedup: make(map[string]time.Time),
	}

	if p.isDuplicate("ticket123", "encrypt456") {
		t.Error("first time should not be duplicate")
	}

	if !p.isDuplicate("ticket123", "encrypt456") {
		t.Error("second time should be duplicate")
	}
}
