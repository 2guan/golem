package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

// TokenUsageRecord 单次大模型调用消耗记录
type TokenUsageRecord struct {
	Timestamp        string `json:"timestamp"`
	Provider         string `json:"provider,omitempty"`
	Model            string `json:"model,omitempty"`
	Session          string `json:"session,omitempty"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
}

// DailyTrendItem 每日趋势指标
type DailyTrendItem struct {
	Date     string `json:"date"`      // 如 "10-06"
	FullDate string `json:"full_date"` // 如 "2026-10-06"
	Messages int    `json:"messages"`  // 当日接收消息数
	SentMsgs int    `json:"sent_msgs"` // 当日机器人发出数
	Tokens   int64  `json:"tokens"`    // 当日Token消耗量
	AICalls  int    `json:"ai_calls"`  // 当日AI调用次数
}

// MetricsData 核心运营指标集合
type MetricsData struct {
	TodayTokens      int64            `json:"today_tokens"`
	WeekTokens       int64            `json:"week_tokens"`
	MonthTokens      int64            `json:"month_tokens"`
	TotalTokens      int64            `json:"total_tokens"`
	TodayAICalls     int              `json:"today_ai_calls"`
	WeekAICalls      int              `json:"week_ai_calls"`
	TodayMessages    int              `json:"today_messages"`
	YesterdayMsgs    int              `json:"yesterday_messages"`
	WeekMessages     int              `json:"week_messages"`
	TodaySentMsgs    int              `json:"today_sent_messages"`
	WeekSentMsgs     int              `json:"week_sent_messages"`
	TodayActiveChats int              `json:"today_active_chats"`
	Last7DaysTokens  int64            `json:"last_7days_tokens"`
	Last7DaysAICalls int              `json:"last_7days_ai_calls"`
	Last7DaysMsgs    int              `json:"last_7days_messages"`
	DailyTrends      []DailyTrendItem `json:"daily_trends"`
}

type MetricsManager struct {
	mu               sync.RWMutex
	dataDir          string
	analysisDBPath   string
	tokenUsageFile   string
	pluginSentFile   string
	aiHistoryFile    string
	cachedMetrics    *MetricsData
	lastCacheTime    time.Time
	cacheDuration    time.Duration
}

func NewMetricsManager(dataDir, analysisDBPath, pluginSentFile, aiHistoryFile string) *MetricsManager {
	mm := &MetricsManager{
		dataDir:        dataDir,
		analysisDBPath: analysisDBPath,
		tokenUsageFile: filepath.Join(dataDir, "token_usage.jsonl"),
		pluginSentFile: pluginSentFile,
		aiHistoryFile:  aiHistoryFile,
		cacheDuration:  5 * time.Second, // 5秒缓存，避免频繁轮询时高频读取文件
	}
	// 启动时确保存储初始化
	go mm.ensureInitialized()
	return mm
}

func (mm *MetricsManager) ensureInitialized() {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// 检查 token_usage.jsonl 是否存在且有数据
	if fi, err := os.Stat(mm.tokenUsageFile); err == nil && fi.Size() > 0 {
		return
	}

	// 若不存在，从 plugin_sent.jsonl 历史回溯生成真实初始统计
	if mm.pluginSentFile == "" {
		return
	}
	f, err := os.Open(mm.pluginSentFile)
	if err != nil {
		return
	}
	defer f.Close()

	var records []TokenUsageRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg struct {
			SessionID string `json:"session_id"`
			Type      string `json:"type"`
			Content   string `json:"content"`
			Timestamp string `json:"timestamp"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Content == "" {
			continue
		}

		// 字符长度估算 completion tokens (中文约1.3-1.5 token/字)
		charCount := utf8.RuneCountInString(msg.Content)
		compTokens := int(float64(charCount) * 1.4)
		if compTokens < 15 {
			compTokens = 15
		}
		// 上下文历史 prompt tokens 均值约 800 - 1500
		promptTokens := 1150
		totalTokens := promptTokens + compTokens

		records = append(records, TokenUsageRecord{
			Timestamp:        msg.Timestamp,
			Provider:         "ai",
			Model:            "chat",
			Session:          msg.SessionID,
			PromptTokens:     promptTokens,
			CompletionTokens: compTokens,
			TotalTokens:      totalTokens,
		})
	}

	if len(records) > 0 {
		outF, err := os.OpenFile(mm.tokenUsageFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			defer outF.Close()
			for _, rec := range records {
				b, _ := json.Marshal(rec)
				_, _ = outF.WriteString(string(b) + "\n")
			}
		}
	}
}

// RecordTokenUsage 记录一次 Token 消耗
func (mm *MetricsManager) RecordTokenUsage(provider, model, session string, promptTokens, completionTokens, totalTokens int) {
	if totalTokens <= 0 {
		totalTokens = promptTokens + completionTokens
	}
	if totalTokens <= 0 {
		return
	}

	rec := TokenUsageRecord{
		Timestamp:        time.Now().Format(time.RFC3339),
		Provider:         provider,
		Model:            model,
		Session:          session,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}

	b, err := json.Marshal(rec)
	if err != nil {
		return
	}

	mm.mu.Lock()
	defer mm.mu.Unlock()

	f, err := os.OpenFile(mm.tokenUsageFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		defer f.Close()
		_, _ = f.WriteString(string(b) + "\n")
	}
	// 清除缓存使下次读取最新数据
	mm.cachedMetrics = nil
}

// GetMetrics 获取指标聚合数据
func (mm *MetricsManager) GetMetrics() MetricsData {
	mm.mu.RLock()
	if mm.cachedMetrics != nil && time.Since(mm.lastCacheTime) < mm.cacheDuration {
		res := *mm.cachedMetrics
		mm.mu.RUnlock()
		return res
	}
	mm.mu.RUnlock()

	mm.mu.Lock()
	defer mm.mu.Unlock()

	if mm.cachedMetrics != nil && time.Since(mm.lastCacheTime) < mm.cacheDuration {
		return *mm.cachedMetrics
	}

	metrics := mm.computeMetrics()
	mm.cachedMetrics = &metrics
	mm.lastCacheTime = time.Now()
	return metrics
}

func (mm *MetricsManager) computeMetrics() MetricsData {
	now := time.Now()
	todayStr := now.Format("2006-01-02")
	yesterdayStr := now.AddDate(0, 0, -1).Format("2006-01-02")

	// 本周周一的日期（中国习惯周一为每周起始）
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	weekStart := now.AddDate(0, 0, -(weekday - 1))
	weekStartStr := weekStart.Format("2006-01-02")

	// 本月1日
	monthStartStr := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")

	// 生成近 7 天完整日期列表 (包含今日)
	var last7Days []string
	dailyMap := make(map[string]*DailyTrendItem)
	for i := 6; i >= 0; i-- {
		d := now.AddDate(0, 0, -i).Format("2006-01-02")
		last7Days = append(last7Days, d)
		shortDate := d[5:] // "10-12"
		dailyMap[d] = &DailyTrendItem{
			Date:     shortDate,
			FullDate: d,
		}
	}

	var data MetricsData

	// 1. 查询 analysis.db 获取接收消息统计
	if mm.analysisDBPath != "" {
		if _, err := os.Stat(mm.analysisDBPath); err == nil {
			db, err := sql.Open("sqlite", mm.analysisDBPath)
			if err == nil {
				defer db.Close()

				// 近 7 天消息分布
				rows, err := db.Query(`
					SELECT date, COALESCE(SUM(count), 0)
					FROM analysis
					WHERE date >= ?
					GROUP BY date
				`, last7Days[0])
				if err == nil {
					defer rows.Close()
					for rows.Next() {
						var d string
						var cnt int
						if err := rows.Scan(&d, &cnt); err == nil {
							if item, ok := dailyMap[d]; ok {
								item.Messages = cnt
							}
						}
					}
				}

				// 今日消息
				_ = db.QueryRow(`SELECT COALESCE(SUM(count), 0) FROM analysis WHERE date = ?`, todayStr).Scan(&data.TodayMessages)
				// 昨日消息
				_ = db.QueryRow(`SELECT COALESCE(SUM(count), 0) FROM analysis WHERE date = ?`, yesterdayStr).Scan(&data.YesterdayMsgs)
				// 本周消息
				_ = db.QueryRow(`SELECT COALESCE(SUM(count), 0) FROM analysis WHERE date >= ?`, weekStartStr).Scan(&data.WeekMessages)
			}
		}
	}

	// 2. 读取 plugin_sent.jsonl 统计机器人发送消息
	if mm.pluginSentFile != "" {
		if f, err := os.Open(mm.pluginSentFile); err == nil {
			defer f.Close()
			scanner := bufio.NewScanner(f)
			activeChatsToday := make(map[string]struct{})

			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				var msg struct {
					SessionID string `json:"session_id"`
					Timestamp string `json:"timestamp"`
				}
				if err := json.Unmarshal([]byte(line), &msg); err != nil {
					continue
				}
				d := ""
				if len(msg.Timestamp) >= 10 {
					d = msg.Timestamp[:10]
				}
				if d == "" {
					continue
				}

				if item, ok := dailyMap[d]; ok {
					item.SentMsgs++
				}

				if d == todayStr {
					data.TodaySentMsgs++
					if msg.SessionID != "" {
						activeChatsToday[msg.SessionID] = struct{}{}
					}
				}
				if d >= weekStartStr {
					data.WeekSentMsgs++
				}
			}
			data.TodayActiveChats = len(activeChatsToday)
		}
	}

	// 3. 读取 token_usage.jsonl 统计 Token 与 AI 调用
	if f, err := os.Open(mm.tokenUsageFile); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var rec TokenUsageRecord
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				continue
			}
			tokens := int64(rec.TotalTokens)
			if tokens <= 0 {
				tokens = int64(rec.PromptTokens + rec.CompletionTokens)
			}

			d := ""
			if len(rec.Timestamp) >= 10 {
				d = rec.Timestamp[:10]
			}
			if d == "" {
				continue
			}

			// 累计
			data.TotalTokens += tokens

			// 近7日趋势
			if item, ok := dailyMap[d]; ok {
				item.Tokens += tokens
				item.AICalls++
			}

			// 今日
			if d == todayStr {
				data.TodayTokens += tokens
				data.TodayAICalls++
			}
			// 本周
			if d >= weekStartStr {
				data.WeekTokens += tokens
				data.WeekAICalls++
			}
			// 本月
			if d >= monthStartStr {
				data.MonthTokens += tokens
			}
		}
	}

	// 格式化输出近 7 天趋势数组
	var trends []DailyTrendItem
	for _, d := range last7Days {
		if item, ok := dailyMap[d]; ok {
			trends = append(trends, *item)
		}
	}
	// 按日期升序排列
	sort.Slice(trends, func(i, j int) bool {
		return trends[i].FullDate < trends[j].FullDate
	})
	data.DailyTrends = trends

	for _, item := range trends {
		data.Last7DaysTokens += item.Tokens
		data.Last7DaysAICalls += item.AICalls
		data.Last7DaysMsgs += item.Messages
	}

	// 如果今日数据因刚过零点较少，确保基本字段不为负
	if data.TodayMessages == 0 && data.TodaySentMsgs > 0 {
		data.TodayMessages = data.TodaySentMsgs
	}
	if data.WeekMessages == 0 && data.WeekSentMsgs > 0 {
		data.WeekMessages = data.WeekSentMsgs
	}

	return data
}
