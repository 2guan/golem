package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sbgayhub/golem/sdk/chatroom"
	"github.com/sbgayhub/golem/sdk/contact"
	"github.com/sbgayhub/golem/sdk/message"
	"github.com/sbgayhub/golem/sdk/plugin"
)

// Config 插件配置
type Config struct {
	DailyLimit         int    `toml:"daily_limit" comment:"每天主动加好友的最大上限次数（默认 3）"`
	AddDelayMinSeconds int    `toml:"add_delay_min_seconds" comment:"主动加好友发起前的自然延迟最小秒数（默认 10）"`
	AddDelayMaxSeconds int    `toml:"add_delay_max_seconds" comment:"主动加好友发起前的自然延迟最大秒数（默认 15）"`
	DefaultGreeting    string `toml:"default_greeting" comment:"默认申请附言内容"`
	DataFile           string `toml:"data_file" comment:"每日计数持久化数据文件路径"`
	OwnerOnly          bool   `toml:"owner_only" comment:"是否仅允许主人执行主动加好友（为 false 时允许群内普通群友触发“加我好友”）"`
}

// DailyStats 每日加好友统计数据
type DailyStats struct {
	Date  string   `json:"date"`  // 格式 YYYY-MM-DD
	Count int      `json:"count"` // 当日已申请次数
	Logs  []string `json:"logs"`  // 历史申请记录
}

// AddFriendPlugin 主动添加好友插件
type AddFriendPlugin struct {
	plugin.ConfigAbility[Config]
	message  message.Ability
	contact  contact.Ability
	chatroom chatroom.Ability

	mu    sync.Mutex
	stats DailyStats
}

func (p *AddFriendPlugin) GetMetadata() *plugin.Metadata {
	return &plugin.Metadata{
		Name:        "add_friend",
		Author:      "Golem Team",
		Version:     "1.0.0",
		Description: "主动添加微信好友插件，支持每日上限频控与防封保护",
		Priority:    0,
	}
}

func (p *AddFriendPlugin) GetSubscriptions() []string {
	return []string{
		message.TypeText.Topic,
	}
}

func (p *AddFriendPlugin) OnLoad() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.loadStats()
	p.checkDateRolloverLocked()
	slog.Info("[add_friend] 插件已加载",
		"date", p.stats.Date,
		"used_count", p.stats.Count,
		"daily_limit", p.getDailyLimit(),
	)
	return nil
}

func (p *AddFriendPlugin) getDailyLimit() int {
	if p.Config.DailyLimit <= 0 {
		return 3
	}
	return p.Config.DailyLimit
}

func (p *AddFriendPlugin) getDataFilePath() string {
	if strings.TrimSpace(p.Config.DataFile) != "" {
		return p.Config.DataFile
	}
	return filepath.Join("data", "add_friend_stats.json")
}

func (p *AddFriendPlugin) loadStats() {
	filePath := p.getDataFilePath()
	p.stats = DailyStats{
		Date:  time.Now().Format("2006-01-02"),
		Count: 0,
		Logs:  make([]string, 0),
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return
	}

	var loaded DailyStats
	if err := json.Unmarshal(data, &loaded); err == nil {
		p.stats = loaded
	}
}

func (p *AddFriendPlugin) saveStatsLocked() {
	filePath := p.getDataFilePath()
	_ = os.MkdirAll(filepath.Dir(filePath), 0755)

	bytes, err := json.MarshalIndent(p.stats, "", "  ")
	if err != nil {
		slog.Error("[add_friend] 序列化统计数据失败", "err", err)
		return
	}
	_ = os.WriteFile(filePath, bytes, 0644)
}

func (p *AddFriendPlugin) checkDateRolloverLocked() {
	today := time.Now().Format("2006-01-02")
	if p.stats.Date != today {
		p.stats.Date = today
		p.stats.Count = 0
		p.stats.Logs = make([]string, 0)
		p.saveStatsLocked()
	}
}

// isOwner 判断发送者是否为机器人主人
func (p *AddFriendPlugin) isOwner(sender string) bool {
	if p.contact == nil {
		return false
	}
	owner := p.contact.GetOwner()
	if owner == nil || owner.Username == "" {
		return false
	}
	return sender == owner.Username
}

func (p *AddFriendPlugin) OnEvent(e *plugin.Event) (bool, error) {
	if e == nil || e.Topic != message.TypeText.Topic {
		return false, nil
	}

	evtMsg, ok := e.Payload.(*plugin.Event_Message)
	if !ok || evtMsg == nil || evtMsg.Message == nil {
		return false, nil
	}

	msg := evtMsg.Message
	rawText := strings.TrimSpace(msg.GetContent())
	if rawText == "" {
		if td := msg.GetText(); td != nil {
			rawText = strings.TrimSpace(td.Content)
		}
	}
	if rawText == "" {
		return false, nil
	}

	sender := msg.GetSender()
	if sender == nil || sender.Username == "" {
		return false, nil
	}

	receiver := p.resolveReceiver(e, msg)
	if receiver == nil {
		return false, nil
	}

	isChatroom := strings.HasSuffix(e.GetSender(), "@chatroom")

	// 1. 管理与查询指令：#加好友统计 / #加好友额度 / #加好友重置
	if rawText == "#加好友统计" || rawText == "加好友统计" || rawText == "#加好友额度" {
		return p.handleStatsQuery(receiver)
	}

	if strings.HasPrefix(rawText, "#加好友重置") {
		return p.handleResetLimit(receiver, sender)
	}

	// 2. 主动精确添加指令：#加好友 <微信号/手机号/wxid> [附言]
	if strings.HasPrefix(rawText, "#加好友") || strings.HasPrefix(rawText, "加好友 ") {
		return p.handleSearchAndAdd(receiver, sender, rawText)
	}

	// 3. 群内交互快捷指令：“加我好友”、“机器人加我”
	if isChatroom {
		if rawText == "加我好友" || rawText == "机器人加我" || rawText == "机器人加我好友" || rawText == "加我" {
			return p.handleGroupAddMe(receiver, sender)
		}
	}

	return false, nil
}

func (p *AddFriendPlugin) resolveReceiver(e *plugin.Event, msg *message.Message) *contact.Contact {
	if p.contact != nil {
		receiver := p.contact.Get(e.GetSender())
		if receiver != nil {
			return receiver
		}
	}
	if msg.Sender != nil && msg.Sender.GetUsername() == e.GetSender() {
		return msg.Sender
	}
	if msg.Receiver != nil && msg.Receiver.GetUsername() == e.GetSender() {
		return msg.Receiver
	}
	return &contact.Contact{Username: e.GetSender()}
}

// handleStatsQuery 查看当日使用额度
func (p *AddFriendPlugin) handleStatsQuery(receiver *contact.Contact) (bool, error) {
	p.mu.Lock()
	p.checkDateRolloverLocked()
	used := p.stats.Count
	limit := p.getDailyLimit()
	date := p.stats.Date
	logList := append([]string(nil), p.stats.Logs...)
	p.mu.Unlock()

	remaining := limit - used
	if remaining < 0 {
		remaining = 0
	}

	reply := fmt.Sprintf("📊【主动加好友频控统计】\n"+
		"📅 日期：%s\n"+
		"🔢 今日上限：%d 次\n"+
		"📈 已用额度：%d 次\n"+
		"✨ 剩余额度：%d 次",
		date, limit, used, remaining,
	)

	if len(logList) > 0 {
		reply += "\n\n📝 今日申请记录："
		for i, l := range logList {
			reply += fmt.Sprintf("\n%d. %s", i+1, l)
		}
	}

	p.sendText(receiver, reply)
	return true, nil
}

// handleResetLimit 重置今日上限（仅限主人）
func (p *AddFriendPlugin) handleResetLimit(receiver *contact.Contact, sender *contact.Contact) (bool, error) {
	if !p.isOwner(sender.Username) {
		p.sendText(receiver, "⚠️ 仅机器人主人可以重置加好友额度。")
		return true, nil
	}

	p.mu.Lock()
	p.stats.Count = 0
	p.stats.Logs = make([]string, 0)
	p.saveStatsLocked()
	limit := p.getDailyLimit()
	p.mu.Unlock()

	p.sendText(receiver, fmt.Sprintf("✅ 已成功重置今日主动加好友计数！当前可用额度：%d 次", limit))
	return true, nil
}

// isAlreadyFriend 严格检查指定目标是否在已同步的好友列表中
func (p *AddFriendPlugin) isAlreadyFriend(target string) bool {
	if p.contact == nil || strings.TrimSpace(target) == "" {
		return false
	}
	target = strings.TrimSpace(target)
	friends := p.contact.List()
	for _, f := range friends {
		if f == nil {
			continue
		}
		// 必须为有效的好友联系人
		if f.Username == "" {
			continue
		}
		if f.Username == target || (f.Alias != "" && f.Alias == target) {
			return true
		}
	}
	return false
}

// cleanGreeting 清理附言两端可能包裹的中英文方括号
func cleanGreeting(raw string) string {
	s := strings.TrimSpace(raw)
	for (strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) ||
		(strings.HasPrefix(s, "【") && strings.HasSuffix(s, "】")) {
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
			s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
		} else {
			s = strings.TrimSuffix(strings.TrimPrefix(s, "【"), "】")
		}
		s = strings.TrimSpace(s)
	}
	return s
}

// handleSearchAndAdd 精确搜索并添加
func (p *AddFriendPlugin) handleSearchAndAdd(receiver *contact.Contact, sender *contact.Contact, rawText string) (bool, error) {
	// 权限检查
	if p.Config.OwnerOnly && !p.isOwner(sender.Username) {
		p.sendText(receiver, "⚠️ 仅机器人主人可以使用主动搜索加好友指令。")
		return true, nil
	}

	// 提取参数：#加好友 <keyword> [content]
	re := regexp.MustCompile(`^(?:#加好友|加好友)\s+([^\s]+)(?:\s+(.*))?$`)
	matches := re.FindStringSubmatch(rawText)
	if len(matches) < 2 || strings.TrimSpace(matches[1]) == "" {
		p.sendText(receiver, "💡 使用格式：\n#加好友 <微信号/手机号/wxid> [申请附言]\n示例：#加好友 test_user 你好我是小助手")
		return true, nil
	}

	target := strings.TrimSpace(matches[1])
	greeting := ""
	if len(matches) >= 3 {
		greeting = cleanGreeting(matches[2])
	}
	if greeting == "" {
		greeting = p.getDefaultGreeting()
	}

	// 先行检查是否已是好友
	if p.isAlreadyFriend(target) {
		p.sendText(receiver, fmt.Sprintf("⚠️ 目标用户 [%s] 已经在好友列表中，无需重复添加。", target))
		return true, nil
	}

	// 检查每日限额
	canProceed, remaining := p.acquireQuota()
	if !canProceed {
		p.sendText(receiver, fmt.Sprintf("⚠️ 今日主动加好友次数已达上限（每天上限 %d 次），已触发安全防封保护，请明天再试！", p.getDailyLimit()))
		return true, nil
	}

	p.sendText(receiver, fmt.Sprintf("🔍 正在搜索并准备好友申请：%s（将在 10~15 秒自然延迟后递交）...", target))

	go func() {
		delaySec := p.getAddDelaySeconds()
		slog.Info("[add_friend] ⏳ 搜索加好友将在自然延迟后发送...", "target", target, "delay_seconds", delaySec)
		time.Sleep(time.Duration(delaySec) * time.Second)

		err := p.doAddFriend(target, greeting, 15) // scene 15 搜索场景
		if err != nil {
			// 失败回退额度
			p.rollbackQuota()
			p.sendText(receiver, fmt.Sprintf("❌ 添加好友失败：%v\n已为您返还本次额度。", err))
			return
		}

		record := fmt.Sprintf("%s 搜索添加 [%s] (附言: %s)", time.Now().Format("15:04:05"), target, greeting)
		p.recordSuccessLog(record)

		p.sendText(receiver, fmt.Sprintf("🎉 好友申请发送成功！\n👤 目标：%s\n💬 附言：%s\n✨ 今日剩余额度：%d 次", target, greeting, remaining))
	}()

	return true, nil
}

// handleGroupAddMe 群成员触发“加我好友”
func (p *AddFriendPlugin) handleGroupAddMe(receiver *contact.Contact, sender *contact.Contact) (bool, error) {
	if p.Config.OwnerOnly && !p.isOwner(sender.Username) {
		p.sendText(receiver, "⚠️ 本群仅管理员/主人可调用主动加好友。")
		return true, nil
	}

	// 检查是否已经是好友
	if p.isAlreadyFriend(sender.Username) {
		p.sendText(receiver, fmt.Sprintf("@%s 我们已经是好友啦，可以直接私聊我哦~", getDisplayName(sender)))
		return true, nil
	}

	// 检查每日限额
	canProceed, remaining := p.acquireQuota()
	if !canProceed {
		p.sendText(receiver, fmt.Sprintf("⚠️ 抱歉，今天主动加好友名额已用完（每日上限 %d 次），请明天再试或主动扫描名片添加我哦~", p.getDailyLimit()))
		return true, nil
	}

	nick := getDisplayName(sender)
	greeting := p.getDefaultGreeting()
	p.sendText(receiver, fmt.Sprintf("👋 收到 @%s 的加好友请求，正在排队等待自然延迟（约 10~15 秒），请留意微信通讯录~", nick))

	go func() {
		delaySec := p.getAddDelaySeconds()
		slog.Info("[add_friend] ⏳ 群成员加好友将在自然延迟后发送...", "target", sender.Username, "nickname", nick, "delay_seconds", delaySec)
		time.Sleep(time.Duration(delaySec) * time.Second)

		// 群场景添加好友，scene = 14 (群聊来源)
		err := p.doAddFriend(sender.Username, greeting, 14)
		if err != nil {
			p.rollbackQuota()
			slog.Error("[add_friend] 群内主动加好友失败", "target", sender.Username, "err", err)
			p.sendText(receiver, fmt.Sprintf("❌ 向 @%s 发送好友申请失败：%v\n已返还额度。", nick, err))
			return
		}

		record := fmt.Sprintf("%s 群成员 [%s / %s]", time.Now().Format("15:04:05"), nick, sender.Username)
		p.recordSuccessLog(record)

		slog.Info("[add_friend] 群内成功发送好友申请", "target", sender.Username, "remaining", remaining)
	}()

	return true, nil
}

func (p *AddFriendPlugin) getAddDelaySeconds() int {
	minSec := p.Config.AddDelayMinSeconds
	maxSec := p.Config.AddDelayMaxSeconds
	if minSec <= 0 {
		minSec = 10
	}
	if maxSec <= 0 {
		maxSec = 15
	}
	if maxSec < minSec {
		maxSec = minSec
	}
	delta := maxSec - minSec
	if delta <= 0 {
		return minSec
	}
	return minSec + int(time.Now().UnixNano()%int64(delta+1))
}

// doAddFriend 执行主动加好友底层流程
func (p *AddFriendPlugin) doAddFriend(target, greeting string, scene int) error {
	if p.contact == nil {
		return fmt.Errorf("contact ability 未注入")
	}

	// 1. 如果 target 本身已在好友列表，直接返回
	if p.isAlreadyFriend(target) {
		return fmt.Errorf("对方已经是好友，无需重复添加")
	}

	v1 := target
	v2 := ""

	// 2. 搜索目标以获取真实 wxid 或凭据
	searchRes := p.contact.Search(target, 0, uint32(scene))
	if searchRes != nil && searchRes.Username != "" {
		// 校验搜索出的真实账号是否已在好友列表中
		if p.isAlreadyFriend(searchRes.Username) || (searchRes.Alias != "" && p.isAlreadyFriend(searchRes.Alias)) {
			return fmt.Errorf("对方已经是好友，无需重复添加")
		}
		v1 = searchRes.Username
		if searchRes.Remark != "" {
			v2 = searchRes.Remark
		}
	}

	// 3. 调用底层 AddFriend (operate=2 发送好友申请)
	err := p.contact.AddFriend(v1, v2, greeting, 2, scene)
	if err != nil {
		return err
	}

	return nil
}

// acquireQuota 校验并占用一次额度
func (p *AddFriendPlugin) acquireQuota() (bool, int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.checkDateRolloverLocked()
	limit := p.getDailyLimit()
	if p.stats.Count >= limit {
		return false, 0
	}

	p.stats.Count++
	p.saveStatsLocked()
	remaining := limit - p.stats.Count
	return true, remaining
}

// rollbackQuota 失败时回退一次额度
func (p *AddFriendPlugin) rollbackQuota() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stats.Count > 0 {
		p.stats.Count--
		p.saveStatsLocked()
	}
}

// recordSuccessLog 记录成功日志
func (p *AddFriendPlugin) recordSuccessLog(logEntry string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stats.Logs = append(p.stats.Logs, logEntry)
	p.saveStatsLocked()
}

func (p *AddFriendPlugin) getBotName() string {
	if p.contact != nil {
		if self := p.contact.GetSelf(); self != nil {
			if nick := strings.TrimSpace(self.GetNickname()); nick != "" {
				return nick
			}
		}
	}
	return "Bot"
}

func (p *AddFriendPlugin) getDefaultGreeting() string {
	botName := p.getBotName()
	if strings.TrimSpace(p.Config.DefaultGreeting) != "" {
		g := p.Config.DefaultGreeting
		g = strings.ReplaceAll(g, "{bot_name}", botName)
		g = strings.ReplaceAll(g, "{bot}", botName)
		g = strings.ReplaceAll(g, "{nickname}", botName)
		return g
	}
	return fmt.Sprintf("你好，我是%s", botName)
}


func (p *AddFriendPlugin) sendText(to *contact.Contact, text string) {
	if p.message == nil || to == nil || strings.TrimSpace(text) == "" {
		return
	}
	msg := &message.Message{
		Receiver: to,
		Type:     message.TypeText,
		Data:     &message.Message_Text{Text: &message.TextData{Content: text}},
	}
	if _, err := p.message.Send(msg); err != nil {
		slog.Error("[add_friend] 发送消息失败", "err", err)
	}
}

func getDisplayName(sender *contact.Contact) string {
	if sender != nil && sender.Nickname != "" {
		return sender.Nickname
	}
	if sender != nil {
		return sender.Username
	}
	return "群友"
}

func main() {
	p := &AddFriendPlugin{}
	slog.Info("[add_friend] 主动添加好友插件启动中...")
	plugin.Start(p)
}
