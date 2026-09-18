package main

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

type Holiday struct {
	Name string
	Date time.Time
}

// 节假日表（支持 2026-2027）
var holidays = []Holiday{
	{"中秋节", time.Date(2026, 9, 25, 0, 0, 0, 0, time.Local)},
	{"国庆节", time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)},
	{"元旦", time.Date(2027, 1, 1, 0, 0, 0, 0, time.Local)},
	{"春节", time.Date(2027, 2, 6, 0, 0, 0, 0, time.Local)},
	{"清明节", time.Date(2027, 4, 5, 0, 0, 0, 0, time.Local)},
	{"劳动节", time.Date(2027, 5, 1, 0, 0, 0, 0, time.Local)},
	{"端午节", time.Date(2027, 6, 19, 0, 0, 0, 0, time.Local)},
	{"国庆节", time.Date(2027, 10, 1, 0, 0, 0, 0, time.Local)},
}

var quotes = []string{
	"工作再累，钱是老板的，命是自己的。放下手头工作，去倒杯热水活动活动颈椎！",
	"带薪拉屎是打工人的神圣权利。每天带薪拉屎10分钟，一年等于多休5天年假！",
	"别看了，今天再怎么拼命，下个月工资也不会翻倍，去活动活动手腕！",
	"摸鱼不是偷懒，而是为了在下一次全力摸鱼时保持更充沛的精力！",
	"手头做不完的事情可以明天做，但今天不摸鱼，今天就彻底亏了！",
	"认认真真上班只能叫用劳动换报酬，上班摸鱼才叫从老板手里赚钱！",
	"人生得意须尽欢，莫使工位空对月。去吃点小零食吧，血糖低了摸鱼没劲！",
	"同事吵架别掺和，老板画饼别当真，安安心心做个快乐的带薪旁观者！",
	"喝杯温水，去走廊看看风景。窗外的鸽子都在晒太阳，你为什么要在电脑前卷！",
}

func GenerateMoyuDaily() string {
	now := time.Now()
	weekdayStr := getWeekdayCN(now.Weekday())

	// 1. 周末倒计时
	var weekendDesc string
	switch now.Weekday() {
	case time.Saturday, time.Sunday:
		weekendDesc = "🎉 今天是周末！合法躺平摆烂中，拒绝任何形式的工作骚扰！"
	case time.Friday:
		weekendDesc = "🔥 今天就是周五啦！坚持到 18:00 下班直接开溜！"
	default:
		daysToFriday := int(time.Friday - now.Weekday())
		weekendDesc = fmt.Sprintf("距离本周五下班还剩：%d 天", daysToFriday)
	}

	// 2. 发工资倒计时（默认每月10号和15号）
	payday10 := getDaysUntilDay(now, 10)
	payday15 := getDaysUntilDay(now, 15)

	// 3. 下一个法定节假日倒计时
	var holidayDesc string
	for _, h := range holidays {
		diff := int(h.Date.Sub(truncateToDay(now)).Hours() / 24)
		if diff == 0 {
			holidayDesc = fmt.Sprintf("🎉 今天就是【%s】假期第一天！祝大家节日快乐！", h.Name)
			break
		} else if diff > 0 {
			holidayDesc = fmt.Sprintf("距离【%s】假期还剩：%d 天", h.Name, diff)
			break
		}
	}
	if holidayDesc == "" {
		holidayDesc = "距离下一个假期已经在路上啦！"
	}

	quote := quotes[rand.IntN(len(quotes))]

	var sb strings.Builder
	sb.WriteString("🐟【打工人摸鱼办 · 每日温馨提醒】\n\n")
	sb.WriteString(fmt.Sprintf("📅 今天是：%s %s\n\n", now.Format("2006年01月02日"), weekdayStr))
	sb.WriteString(fmt.Sprintf("⏳ %s\n", weekendDesc))
	sb.WriteString(fmt.Sprintf("💰 距离发工资（10号）：%s\n", payday10))
	sb.WriteString(fmt.Sprintf("💳 距离发工资（15号）：%s\n", payday15))
	sb.WriteString(fmt.Sprintf("🏖️ %s\n\n", holidayDesc))
	sb.WriteString("💬【今日摸鱼箴言】：\n" + quote)

	return sb.String()
}

func getWeekdayCN(w time.Weekday) string {
	switch w {
	case time.Sunday:
		return "星期日"
	case time.Monday:
		return "星期一"
	case time.Tuesday:
		return "星期二"
	case time.Wednesday:
		return "星期三"
	case time.Thursday:
		return "星期四"
	case time.Friday:
		return "星期五"
	case time.Saturday:
		return "星期六"
	default:
		return ""
	}
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func getDaysUntilDay(now time.Time, targetDay int) string {
	currentDay := now.Day()
	if currentDay == targetDay {
		return "🎉 就是今天！数钱数到手抽筋！"
	}
	if currentDay < targetDay {
		return fmt.Sprintf("还剩 %d 天", targetDay-currentDay)
	}

	// 目标日在下个月
	nextMonth := now.AddDate(0, 1, 0)
	targetDate := time.Date(nextMonth.Year(), nextMonth.Month(), targetDay, 0, 0, 0, 0, now.Location())
	days := int(targetDate.Sub(truncateToDay(now)).Hours() / 24)
	return fmt.Sprintf("还剩 %d 天", days)
}
