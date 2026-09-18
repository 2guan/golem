package main

// Config 插件配置
type Config struct {
	ImgURL        string `toml:"img_url" comment:"美女图片API"`
	ImgVideoURL   string `toml:"img_video_url" comment:"美女视频API"`
	BoyURL        string `toml:"boy_url" comment:"帅哥图片API"`
	BoyVideoURL   string `toml:"boy_video_url" comment:"帅哥视频API"`
	HeisiURL      string `toml:"heisi_url" comment:"黑丝图片API"`
	BaisiURL      string `toml:"baisi_url" comment:"白丝图片API"`
	HeisiVideoURL string `toml:"heisi_video_url" comment:"黑丝视频API"`
	BaisiVideoURL string `toml:"baisi_video_url" comment:"白丝视频API"`
	// 搜图方式多种，可以百度图片网页，这里用的是 https://www.apihz.cn/api/apihzbqbbaidu.html 提供的
	SearchURL string `toml:"search_url" comment:"搜索图片API（默认使用 apihz.cn 的百度表情搜图）"`
	VideoRate int    `toml:"video_rate" comment:"视频触发概率(0-100)"`
}
