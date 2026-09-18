package main

import (
	"testing"
	"time"
)

func TestMealTime(t *testing.T) {
	lunchTime := time.Date(2026, 9, 17, 12, 30, 0, 0, time.Local)
	if mt := GetCurrentMealTime(lunchTime); mt != MealLunch {
		t.Fatalf("expected lunch, got %v", mt)
	}

	teaTime := time.Date(2026, 9, 17, 15, 30, 0, 0, time.Local)
	if mt := GetCurrentMealTime(teaTime); mt != MealTea {
		t.Fatalf("expected tea, got %v", mt)
	}

	supperTime := time.Date(2026, 9, 17, 23, 0, 0, 0, time.Local)
	if mt := GetCurrentMealTime(supperTime); mt != MealSupper {
		t.Fatalf("expected supper, got %v", mt)
	}
}

func TestPickAndReroll(t *testing.T) {
	engine := NewEngine()
	sessionID := "test_user"

	dish1, ok := engine.PickDish(sessionID, MealLunch, nil, nil)
	if !ok || dish1 == nil {
		t.Fatalf("failed to pick first dish")
	}

	dish2, ok := engine.PickDish(sessionID, MealLunch, nil, nil)
	if !ok || dish2 == nil {
		t.Fatalf("failed to pick second dish")
	}

	if dish1.Name == dish2.Name {
		t.Logf("Warning: consecutive dishes have same name (unlikely unless only 1 dish)")
	}
}

func TestFilters(t *testing.T) {
	engine := NewEngine()
	sessionID := "test_filter"

	// 减脂过滤
	dish, ok := engine.PickDish(sessionID, MealLunch, []string{"减脂"}, nil)
	if !ok || dish == nil {
		t.Fatalf("failed to pick light dish")
	}
	hasTag := false
	for _, tag := range dish.Tags {
		if tag == "减脂" || tag == "低卡" || tag == "清淡" {
			hasTag = true
			break
		}
	}
	if !hasTag {
		t.Fatalf("dish %s does not match light tag", dish.Name)
	}
}
