package main

// Config 仪表盘插件配置
type Config struct {
	Host     string `toml:"host"`      // 监听地址，默认 "0.0.0.0"
	Port     int    `toml:"port"`      // 监听端口，默认 8899
	DataDir  string `toml:"data_dir"`  // 数据目录，默认 "data"
}
