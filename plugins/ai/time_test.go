package main

import (
	"strings"
	"testing"
	"time"
)

func TestGetTimePeriod(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	tests := []struct {
		hour     int
		expected string
	}{
		{6, "早晨/清晨"},
		{8, "早晨/清晨"},
		{9, "上午"},
		{11, "上午"},
		{12, "中午"},
		{13, "中午"},
		{14, "下午"},
		{17, "下午"},
		{18, "晚上"},
		{22, "晚上"},
		{23, "深夜/凌晨"},
		{2, "深夜/凌晨"},
		{4, "深夜/凌晨"},
	}

	for _, tc := range tests {
		tm := time.Date(2026, 9, 23, tc.hour, 30, 0, 0, loc)
		actual := getTimePeriod(tm)
		if actual != tc.expected {
			t.Errorf("hour %d: expected %s, got %s", tc.hour, tc.expected, actual)
		}
	}
}

func TestFormatTimeGap(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")

	t.Run("zero_last_time", func(t *testing.T) {
		now := time.Date(2026, 9, 23, 9, 30, 0, 0, loc)
		res := formatTimeGap(time.Time{}, now)
		if res != "" {
			t.Errorf("expected empty string, got %s", res)
		}
	})

	t.Run("continuous_chat_short_gap", func(t *testing.T) {
		last := time.Date(2026, 9, 23, 9, 20, 0, 0, loc)
		now := time.Date(2026, 9, 23, 9, 30, 0, 0, loc)
		res := formatTimeGap(last, now)
		if res != "" {
			t.Errorf("expected empty string for 10m gap, got %s", res)
		}
	})

	t.Run("late_night_continuous_chat", func(t *testing.T) {
		last := time.Date(2026, 9, 22, 23, 50, 0, 0, loc)
		now := time.Date(2026, 9, 23, 0, 20, 0, 0, loc)
		res := formatTimeGap(last, now)
		if res != "" {
			t.Errorf("expected empty string for late night short gap across midnight, got %s", res)
		}
	})

	t.Run("overnight_next_morning", func(t *testing.T) {
		last := time.Date(2026, 9, 22, 23, 15, 0, 0, loc)
		now := time.Date(2026, 9, 23, 9, 26, 0, 0, loc)
		res := formatTimeGap(last, now)
		if !strings.Contains(res, "已隔夜") || !strings.Contains(res, "上午 09:26") {
			t.Errorf("expected overnight hint with 09:26, got %s", res)
		}
	})

	t.Run("same_day_several_hours_gap", func(t *testing.T) {
		last := time.Date(2026, 9, 23, 10, 0, 0, 0, loc)
		now := time.Date(2026, 9, 23, 15, 30, 0, 0, loc)
		res := formatTimeGap(last, now)
		if !strings.Contains(res, "5 小时") || !strings.Contains(res, "下午 15:30") {
			t.Errorf("expected 5 hours gap with afternoon 15:30, got %s", res)
		}
	})

	t.Run("multi_days_gap", func(t *testing.T) {
		last := time.Date(2026, 9, 20, 10, 0, 0, 0, loc)
		now := time.Date(2026, 9, 23, 10, 0, 0, 0, loc)
		res := formatTimeGap(last, now)
		if !strings.Contains(res, "3 天") {
			t.Errorf("expected 3 days gap, got %s", res)
		}
	})
}
