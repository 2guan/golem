package main

import "strings"

// groupGate 某会话生效的群门闩（全局 + 该白名单项覆盖）。
// 指针覆盖为 nil 时沿用全局，不改默认行为。
type groupGate struct {
	TriggerNames          []string
	BubbleRate            float64
	BubbleCooldownMin     int
	DebounceSeconds       int
	MaxContextMessages    int
	GroupPushAll          bool
	EmojiBurstCount       int
	EmojiBurstWindowSec   int
	EmojiBurstCooldownMin int
	Overridden            bool // 该会话至少写了一个覆盖字段
}

func mergeGroupGate(cfg Config, t *Target) groupGate {
	g := groupGate{
		TriggerNames:          append([]string(nil), cfg.TriggerNames...),
		BubbleRate:            cfg.BubbleRate,
		BubbleCooldownMin:     cfg.BubbleCooldownMin,
		DebounceSeconds:       cfg.DebounceSeconds,
		MaxContextMessages:    cfg.MaxContextMessages,
		GroupPushAll:          cfg.GroupPushAll,
		EmojiBurstCount:       cfg.EmojiBurstCount,
		EmojiBurstWindowSec:   cfg.EmojiBurstWindowSec,
		EmojiBurstCooldownMin: cfg.EmojiBurstCooldownMin,
	}
	if t == nil {
		return g
	}
	if t.TriggerNames != nil {
		g.TriggerNames = append([]string(nil), *t.TriggerNames...)
		g.Overridden = true
	}
	if t.BubbleRate != nil {
		g.BubbleRate = *t.BubbleRate
		g.Overridden = true
	}
	if t.BubbleCooldownMin != nil {
		g.BubbleCooldownMin = *t.BubbleCooldownMin
		g.Overridden = true
	}
	if t.DebounceSeconds != nil {
		g.DebounceSeconds = *t.DebounceSeconds
		g.Overridden = true
	}
	if t.MaxContextMessages != nil {
		g.MaxContextMessages = *t.MaxContextMessages
		g.Overridden = true
	}
	if t.GroupPushAll != nil {
		g.GroupPushAll = *t.GroupPushAll
		g.Overridden = true
	}
	if t.EmojiBurstCount != nil {
		g.EmojiBurstCount = *t.EmojiBurstCount
		g.Overridden = true
	}
	if t.EmojiBurstWindowSec != nil {
		g.EmojiBurstWindowSec = *t.EmojiBurstWindowSec
		g.Overridden = true
	}
	if t.EmojiBurstCooldownMin != nil {
		g.EmojiBurstCooldownMin = *t.EmojiBurstCooldownMin
		g.Overridden = true
	}
	return g
}

func (p *BridgePlugin) gateFor(chatID string) groupGate {
	cfg := p.configSnapshot()
	return mergeGroupGate(cfg, p.findTarget(chatID))
}

func countGateOverrides(targets []Target) int {
	n := 0
	for i := range targets {
		if mergeGroupGate(Config{}, &targets[i]).Overridden {
			n++
		}
	}
	return n
}

func (t Target) hasGateOverride() bool {
	return mergeGroupGate(Config{}, &t).Overridden
}

func clampBubble(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func cleanTriggerNames(names []string) []string {
	out := make([]string, 0, len(names))
	seen := map[string]struct{}{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}
