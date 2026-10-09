package main

import (
	"log/slog"

	"github.com/sbgayhub/golem/sdk/plugin"
)

func main() {
	cfg := defaultConfig()
	p := &ImageGenPlugin{
		ConfigAbility: plugin.ConfigAbility[Config]{
			Config: cfg,
		},
		generator:    newImageGenerator(cfg),
		recentImages: make(map[string]*recentImageInfo),
	}
	slog.Info("[image_gen] AI 图像生成与自拍插件启动中...")
	plugin.Start(p)
}
