package main

type Config struct {
	APIKey           string   `toml:"api_key" comment:"any2api / seedream API Key"`
	BaseURL          string   `toml:"base_url" comment:"any2api Base URL"`
	Model            string   `toml:"model" comment:"生图模型名称，默认 seedream-4.7"`
	FFmpegPath       string   `toml:"ffmpeg_path" comment:"ffmpeg 路径，用于 webp 转码 jpeg"`
	DefaultAvatarURL string   `toml:"default_avatar_url" comment:"肉丸默认微信头像参考图 URL"`
	ScenicScenes     []string `toml:"scenic_scenes,omitempty" comment:"自定义自拍风景场景列表"`
	TimeoutSeconds   int      `toml:"timeout_seconds,omitempty" comment:"生图请求超时秒数，默认 240"`
}

func defaultConfig() Config {
	return Config{
		APIKey:           "a2a_u5XZ8fOuu6jF3jyYjR40N3Fx3Zw4cOu5soVJxWyULZg",
		BaseURL:          "https://any2api.ai.guantools.top",
		Model:            "seedream-4.7",
		FFmpegPath:       "/opt/homebrew/bin/ffmpeg",
		DefaultAvatarURL: "https://wx.qlogo.cn/mmhead/ver_1/ia1JRyhA4QPRKbuKHBrTNmbaAj8S58icRr2jmjmwVY103ZSq7LDBFRZia0hCe4o2msLSePo9ON8Y3Nkv5V3qLtpjsdJUt3JRlhKLs7zKfAvBFuxoCpuvu1rLeUMtmFxDeBu/132",
		TimeoutSeconds:   240,
	}
}
