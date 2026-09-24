package main

import (
	"bufio"
	"os"
	"regexp"
	"sort"
	"strings"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI 清除字符串中的 ANSI 终端彩色转义字符
func StripANSI(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

// LogEngine 日志解析与检索服务
type LogEngine struct {
	logFilePath string
}

func NewLogEngine(logFilePath string) *LogEngine {
	return &LogEngine{logFilePath: logFilePath}
}

// ParseLine 解析单行日志
func (le *LogEngine) ParseLine(id int, line string) LogItem {
	clean := strings.TrimSpace(StripANSI(line))
	item := LogItem{
		ID:        id,
		Timestamp: "",
		Level:     "INFO",
		Tag:       "system",
		Message:   clean,
		Raw:       clean,
	}

	if len(clean) >= 19 && clean[4] == '-' && clean[7] == '-' && clean[10] == ' ' && clean[13] == ':' && clean[16] == ':' {
		item.Timestamp = clean[:19]
		rest := strings.TrimSpace(clean[19:])

		// 匹配日志级别
		if strings.HasPrefix(rest, "DBG") {
			item.Level = "DEBUG"
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "DBG"))
		} else if strings.HasPrefix(rest, "INF") {
			item.Level = "INFO"
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "INF"))
		} else if strings.HasPrefix(rest, "WRN") {
			item.Level = "WARN"
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "WRN"))
		} else if strings.HasPrefix(rest, "ERR") {
			item.Level = "ERROR"
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "ERR"))
		} else if strings.HasPrefix(rest, "DEBUG") {
			item.Level = "DEBUG"
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "DEBUG"))
		} else if strings.HasPrefix(rest, "INFO") {
			item.Level = "INFO"
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "INFO"))
		} else if strings.HasPrefix(rest, "WARN") {
			item.Level = "WARN"
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "WARN"))
		} else if strings.HasPrefix(rest, "ERROR") {
			item.Level = "ERROR"
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "ERROR"))
		}

		// 匹配标签 tag，如 "golem_plugin_ai: ..." 或 "[contact ability] ..."
		if strings.HasPrefix(rest, "golem_plugin_") {
			colonIdx := strings.Index(rest, ":")
			if colonIdx > 0 {
				item.Tag = rest[:colonIdx]
				rest = strings.TrimSpace(rest[colonIdx+1:])
			}
		} else if strings.HasPrefix(rest, "[") {
			bracketIdx := strings.Index(rest, "]")
			if bracketIdx > 0 {
				item.Tag = strings.Trim(rest[1:bracketIdx], " ")
				rest = strings.TrimSpace(rest[bracketIdx+1:])
			}
		}

		item.Message = rest
	}

	return item
}

type LogFilterOptions struct {
	Level   string // ALL, DEBUG, INFO, WARN, ERROR
	Keyword string
	Tag     string
	Since   string
	Until   string
	Order   string // "asc" or "desc" (default "desc")
	Limit   int
	Offset  int
}

// Query 检索符合条件的日志项
func (le *LogEngine) Query(opts LogFilterOptions) (LogQueryResponse, error) {
	file, err := os.Open(le.logFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return LogQueryResponse{Total: 0, Filtered: 0, Limit: opts.Limit, Offset: opts.Offset, Items: []LogItem{}}, nil
		}
		return LogQueryResponse{}, err
	}
	defer file.Close()

	if opts.Limit <= 0 {
		opts.Limit = 200
	}
	if opts.Limit > 2000 {
		opts.Limit = 2000
	}
	if opts.Order == "" {
		opts.Order = "desc"
	}

	scanner := bufio.NewScanner(file)
	// 允许单行最大 512KB
	buf := make([]byte, 0, 512*1024)
	scanner.Buffer(buf, 512*1024)

	var allItems []LogItem
	lineID := 1
	keywordLower := strings.ToLower(strings.TrimSpace(opts.Keyword))
	levelFilter := strings.ToUpper(strings.TrimSpace(opts.Level))
	tagFilter := strings.ToLower(strings.TrimSpace(opts.Tag))

	for scanner.Scan() {
		line := scanner.Text()
		item := le.ParseLine(lineID, line)
		lineID++

		// 过滤级别
		if levelFilter != "" && levelFilter != "ALL" {
			if item.Level != levelFilter {
				continue
			}
		}

		// 过滤标签
		if tagFilter != "" && !strings.Contains(strings.ToLower(item.Tag), tagFilter) {
			continue
		}

		// 过滤时间
		if opts.Since != "" && item.Timestamp != "" && item.Timestamp < opts.Since {
			continue
		}
		if opts.Until != "" && item.Timestamp != "" && item.Timestamp > opts.Until {
			continue
		}

		// 关键字检索
		if keywordLower != "" {
			inMsg := strings.Contains(strings.ToLower(item.Message), keywordLower)
			inTag := strings.Contains(strings.ToLower(item.Tag), keywordLower)
			inTime := strings.Contains(item.Timestamp, keywordLower)
			if !inMsg && !inTag && !inTime {
				continue
			}
		}

		allItems = append(allItems, item)
	}

	totalFiltered := len(allItems)

	// 排序
	if opts.Order == "desc" {
		// 倒序：新日志在前
		for i, j := 0, len(allItems)-1; i < j; i, j = i+1, j-1 {
			allItems[i], allItems[j] = allItems[j], allItems[i]
		}
	}

	// 分页切片
	start := opts.Offset
	if start > len(allItems) {
		start = len(allItems)
	}
	end := start + opts.Limit
	if end > len(allItems) {
		end = len(allItems)
	}

	pageItems := allItems[start:end]
	if pageItems == nil {
		pageItems = []LogItem{}
	}

	return LogQueryResponse{
		Total:    lineID - 1,
		Filtered: totalFiltered,
		Limit:    opts.Limit,
		Offset:   opts.Offset,
		Items:    pageItems,
	}, nil
}

// GetDistinctTags 提取日志中出现的标签列表
func (le *LogEngine) GetDistinctTags() ([]string, error) {
	file, err := os.Open(le.logFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 256*1024)
	scanner.Buffer(buf, 256*1024)

	tagMap := make(map[string]int)
	lineID := 1
	for scanner.Scan() {
		line := scanner.Text()
		item := le.ParseLine(lineID, line)
		lineID++
		if item.Tag != "" && item.Tag != "system" {
			tagMap[item.Tag]++
		}
	}

	tags := make([]string, 0, len(tagMap))
	for t := range tagMap {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags, nil
}
