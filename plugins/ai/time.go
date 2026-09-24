package main

import (
	"fmt"
	"time"
)

// getTimePeriod 返回当前时间所处的日常时段（用于大模型感知生活作息）
func getTimePeriod(t time.Time) string {
	hour := t.Hour()
	switch {
	case hour >= 5 && hour < 9:
		return "早晨/清晨"
	case hour >= 9 && hour < 12:
		return "上午"
	case hour >= 12 && hour < 14:
		return "中午"
	case hour >= 14 && hour < 18:
		return "下午"
	case hour >= 18 && hour < 23:
		return "晚上"
	default:
		return "深夜/凌晨"
	}
}

// formatTimeGap 计算距离上一轮对话的时间间隔提示，防止模型停留在陈旧的时间语境中
func formatTimeGap(lastTime, now time.Time) string {
	if lastTime.IsZero() {
		return ""
	}
	diff := now.Sub(lastTime)
	// 短时间内连续聊天（如 1 小时内，或者深夜熬夜跨午夜且间隔小于 3 小时），不插入时间分割提示
	if diff < 1*time.Hour || (diff < 3*time.Hour && now.Hour() < 6 && lastTime.Hour() >= 21) {
		return ""
	}

	period := getTimePeriod(now)
	timeFmt := now.Format("15:04")

	// 跨天且当前已进入次日清晨及之后
	if (lastTime.Year() != now.Year() || lastTime.YearDay() != now.YearDay()) && now.Hour() >= 5 {
		daysDiff := int(now.Truncate(24*time.Hour).Sub(lastTime.Truncate(24*time.Hour)).Hours() / 24)
		if daysDiff <= 1 {
			return fmt.Sprintf("[已隔夜，当前现实时间为次日%s %s]", period, timeFmt)
		}
		return fmt.Sprintf("[距离上一轮对话已过去 %d 天，当前现实时间为%s %s]", daysDiff, period, timeFmt)
	}

	hours := int(diff.Hours())
	if hours <= 1 {
		return fmt.Sprintf("[距离上一轮对话已过去约 1 小时，当前现实时间为%s %s]", period, timeFmt)
	}
	return fmt.Sprintf("[距离上一轮对话已过去约 %d 小时，当前现实时间为%s %s]", hours, period, timeFmt)
}
