package main

import (
	"log/slog"

	"github.com/sbgayhub/golem/sdk/plugin"
)

func main() {
	p := &SetuPlugin{
		ConfigAbility: plugin.ConfigAbility[Config]{
			Config: Config{
				ImgURL:        "https://api.yujn.cn/api/ksxjj.php",
				ImgVideoURL:   "https://api.yujn.cn/api/zzxjj.php",
				BoyURL:        "https://api.yujn.cn/api/boy.php",
				BoyVideoURL:   "https://api.yujn.cn/api/xgg.php",
				HeisiURL:      "https://api.yujn.cn/api/heisi.php?",
				BaisiURL:      "https://api.yujn.cn/api/baisi.php?",
				HeisiVideoURL: "https://api.yujn.cn/api/heisis.php?type=video",
				BaisiVideoURL: "https://api.yujn.cn/api/baisis.php?type=video",
				// 搜图方式多种，可以百度图片网页，这里用的是 https://www.apihz.cn/api/apihzbqbbaidu.html 提供的
				SearchURL: "https://cn.apihz.cn/api/img/apihzbqbbaidu.php?id=88888888&key=88888888&limit=10&page=1&words=",
				VideoRate: 50,
			},
		},
		client: newHTTPClient(),
	}
	slog.Info("[setu] 色图插件启动中...")
	plugin.Start(p)
}
