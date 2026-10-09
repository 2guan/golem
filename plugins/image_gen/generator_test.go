package main

import (
	"testing"
)

func TestGeneratorInstance(t *testing.T) {
	cfg := defaultConfig()
	gen := newImageGenerator(cfg)
	if gen == nil {
		t.Fatal("generator is nil")
	}
	prompt, scene := gen.BuildSelfiePrompt("故宫角楼")
	if scene != "故宫角楼" {
		t.Errorf("expected scene 故宫角楼, got %s", scene)
	}
	if prompt == "" {
		t.Error("prompt should not be empty")
	}
}
