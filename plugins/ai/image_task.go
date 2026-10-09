package main

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/sbgayhub/golem/sdk/contact"
)

var (
	imageTagRegex   = regexp.MustCompile(`(?is)<image\b([^>]*?)(?:/>|>([\s\S]*?)</image>)`)
	attrRefRegex    = regexp.MustCompile(`(?i)\bref\s*=\s*["']?([^"'\s>]+)["']?`)
	attrPromptRegex = regexp.MustCompile(`(?is)\bprompt\s*=\s*["']([^"']*)["']`)
)

type imageTask struct {
	Ref    string
	Prompt string
}

func extractImageTasks(text string) ([]imageTask, string) {
	var tasks []imageTask
	clean := imageTagRegex.ReplaceAllStringFunc(text, func(match string) string {
		sub := imageTagRegex.FindStringSubmatch(match)
		if len(sub) < 2 {
			return ""
		}
		attrs := sub[1]
		body := ""
		if len(sub) > 2 {
			body = strings.TrimSpace(sub[2])
		}

		ref := ""
		if m := attrRefRegex.FindStringSubmatch(attrs); len(m) > 1 {
			ref = strings.TrimSpace(m[1])
		}

		prompt := ""
		if m := attrPromptRegex.FindStringSubmatch(attrs); len(m) > 1 {
			prompt = strings.TrimSpace(m[1])
		}
		if prompt == "" && body != "" {
			prompt = body
		}

		tasks = append(tasks, imageTask{
			Ref:    ref,
			Prompt: prompt,
		})
		return ""
	})
	return tasks, strings.TrimSpace(clean)
}

func stripImageTags(s string) string {
	return imageTagRegex.ReplaceAllString(s, "")
}

func (p *AiPlugin) triggerImageTasks(receiver *contact.Contact, sessionKey string, tasks []imageTask) {
	if len(tasks) == 0 || p.caller == nil || receiver == nil {
		return
	}

	for _, task := range tasks {
		t := task
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("[ai] 异步生图任务崩溃", "err", r)
				}
			}()

			receiverWxid := receiver.GetUsername()
			slog.Info("[ai] 触发 AI 生图/自拍任务", "ref", t.Ref, "prompt", t.Prompt, "receiver", receiverWxid)

			if t.Ref == "selfie" {
				args := map[string]string{
					"scene":    t.Prompt,
					"receiver": receiverWxid,
				}
				_, respBytes, err := p.caller.CallPlugin("image.selfie", args)
				if err != nil {
					slog.Error("[ai] 调用 image.selfie 失败", "err", err)
				} else if len(respBytes) > 0 {
					mediaURL := strings.TrimSpace(string(respBytes))
					if strings.HasPrefix(mediaURL, "/api/") {
						p.updateLastAssistantMedia(sessionKey, mediaURL)
					}
				}
				return
			}

			// 普通生图或参考图改图
			args := map[string]string{
				"prompt":   t.Prompt,
				"receiver": receiverWxid,
			}

			if t.Ref == "last" {
				if cached := p.getRecentImage(sessionKey); cached != nil && len(cached.Data) > 0 {
					mime := cached.MimeType
					if mime == "" {
						mime = "image/jpeg"
					}
					args["image"] = fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(cached.Data))
				}
			}

			_, respBytes, err := p.caller.CallPlugin("image.generate", args)
			if err != nil {
				slog.Error("[ai] 调用 image.generate 失败", "err", err)
			} else if len(respBytes) > 0 {
				mediaURL := strings.TrimSpace(string(respBytes))
				if strings.HasPrefix(mediaURL, "/api/") {
					p.updateLastAssistantMedia(sessionKey, mediaURL)
				}
			}
		}()
	}
}
