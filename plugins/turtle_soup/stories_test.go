package main

import (
	"strings"
	"testing"
)

func TestDefaultStories(t *testing.T) {
	if len(defaultStories) != 35 {
		t.Fatalf("expected 35 stories, got %d", len(defaultStories))
	}

	seenIDs := make(map[int]bool)
	seenTitles := make(map[string]bool)

	for i, s := range defaultStories {
		if s.ID != i+1 {
			t.Errorf("story %d has wrong ID %d", i+1, s.ID)
		}
		if seenIDs[s.ID] {
			t.Errorf("duplicate story ID %d", s.ID)
		}
		seenIDs[s.ID] = true

		if strings.TrimSpace(s.Title) == "" {
			t.Errorf("story %d has empty Title", s.ID)
		}
		if seenTitles[s.Title] {
			t.Errorf("duplicate story Title: %s", s.Title)
		}
		seenTitles[s.Title] = true

		if strings.TrimSpace(s.Surface) == "" {
			t.Errorf("story %s has empty Surface", s.Title)
		}
		if strings.TrimSpace(s.Bottom) == "" {
			t.Errorf("story %s has empty Bottom", s.Title)
		}
		if len(s.Clues) < 3 {
			t.Errorf("story %s has fewer than 3 Clues (%d)", s.Title, len(s.Clues))
		}
		for j, clue := range s.Clues {
			if strings.TrimSpace(clue) == "" {
				t.Errorf("story %s clue %d is empty", s.Title, j)
			}
		}
		if len(s.KeyMatches) == 0 {
			t.Errorf("story %s has no KeyMatches", s.Title)
		}
	}
}

func TestExtractJSONObject(t *testing.T) {
	raw := "```json\n{\"title\": \"测试\", \"surface\": \"汤面\", \"bottom\": \"汤底\"}\n```"
	extracted := extractJSONObject(raw)
	expected := "{\"title\": \"测试\", \"surface\": \"汤面\", \"bottom\": \"汤底\"}"
	if extracted != expected {
		t.Errorf("extractJSONObject failed:\ngot: %s\nwant: %s", extracted, expected)
	}

	conversational := "好的，为您创作的海龟汤如下：\n```json\n{\"title\": \"测试2\"}\n```\n祝您玩得开心！"
	extracted2 := extractJSONObject(conversational)
	expected2 := "{\"title\": \"测试2\"}"
	if extracted2 != expected2 {
		t.Errorf("extractJSONObject failed on conversational text:\ngot: %s\nwant: %s", extracted2, expected2)
	}
}

func TestParseAIStoryCmd(t *testing.T) {
	cases := []struct {
		input       string
		wantMatched bool
		wantTopic   string
	}{
		{"私房海龟汤", true, ""},
		{"私房海龟汤 温馨爱情", true, "温馨爱情"},
		{"私房海龟汤: 职场商战", true, "职场商战"},
		{"私房海龟汤：悬疑反转", true, "悬疑反转"},
		{"AI海龟汤", true, ""},
		{"ai海龟汤 科幻脑洞", true, "科幻脑洞"},
		{"来碗AI海龟汤 治愈故事", true, "治愈故事"},
		{"来碗私房汤 校园搞笑", true, "校园搞笑"},
		{"来碗海龟汤", false, ""},
		{"开局海龟汤", false, ""},
		{"海龟汤提示", false, ""},
	}

	for _, c := range cases {
		matched, topic := parseAIStoryCmd(c.input)
		if matched != c.wantMatched {
			t.Errorf("parseAIStoryCmd(%q) matched = %v, want %v", c.input, matched, c.wantMatched)
		}
		if topic != c.wantTopic {
			t.Errorf("parseAIStoryCmd(%q) topic = %q, want %q", c.input, topic, c.wantTopic)
		}
	}
}
