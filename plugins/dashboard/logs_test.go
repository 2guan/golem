package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStripANSI(t *testing.T) {
	colored := "\x1b[90m2026-09-23 09:57:41\x1b[0m \x1b[32mINF\x1b[0m \x1b[1m日志系统初始化完成\x1b[0m"
	expected := "2026-09-23 09:57:41 INF 日志系统初始化完成"
	actual := StripANSI(colored)
	if actual != expected {
		t.Errorf("expected %q, got %q", expected, actual)
	}
}

func TestLogEngineParsingAndQuery(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test_golem.log")

	sampleContent := `
2026-09-23 09:57:41 INF 日志系统初始化完成 level=INFO output=console
2026-09-23 09:57:44 DBG starting plugin path=plugins/ai/golem_plugin_ai
2026-09-23 09:57:45 DBG golem_plugin_ai: 2026/09/23 09:57:45 INFO [ai] 历史对话上下文恢复成功
2026-09-23 12:10:05 INF golem_plugin_ai: 机器人 -> 用户A: [文本] 害，一上午忙着处理工作呢
2026-09-23 12:10:08 WRN [contact ability] 发生警告信息
2026-09-23 12:15:00 ERR golem_plugin_ai: 请求大模型超时 error=timeout
`
	if err := os.WriteFile(logFile, []byte(sampleContent), 0644); err != nil {
		t.Fatalf("failed to write sample log: %v", err)
	}

	engine := NewLogEngine(logFile)

	// 测试级别过滤：ERROR
	respErr, err := engine.Query(LogFilterOptions{Level: "ERROR"})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if respErr.Filtered != 1 {
		t.Errorf("expected 1 ERROR log, got %d", respErr.Filtered)
	}

	// 测试关键字过滤：用户A
	respKW, err := engine.Query(LogFilterOptions{Keyword: "用户A"})
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if respKW.Filtered != 1 {
		t.Errorf("expected 1 log matching '用户A', got %d", respKW.Filtered)
	}

	// 测试标签提取
	tags, err := engine.GetDistinctTags()
	if err != nil {
		t.Fatalf("get distinct tags error: %v", err)
	}
	if len(tags) == 0 {
		t.Errorf("expected tags, got 0")
	}
}
