# Golem

一个基于 Go Plugin 的机器人插件框架，支持动态加载插件、热更新，让你轻松扩展机器人功能。

> [!note]
> 请遵守相关法律法规，请勿将本项目用于非法行为
> 
> 请使用小号登录，设备类型选择 `mac` 

## ✨ 特性

- 🔌 **插件化架构** - 基于 hashicorp/go-plugin 的稳定插件系统
- 🔄 **热加载/热卸载** - 无需重启即可更新插件
- 📦 **独立 SDK** - 插件开发者无需克隆整个项目，一行命令即可引入依赖
- 🛠 **丰富的 API** - 消息、联系人、聊天室、朋友圈、支付等完整能力
- 📝 **开箱即用的模板** - 提供完整的插件开发模板和示例
- 🎯 **事件驱动** - 支持事件订阅、命令处理、会话管理
- ⚙️ **灵活配置** - 支持黑白名单、优先级控制、配置热更新

## 📁 项目结构

```
golem/
├── sdk/                 # Golem SDK（可独立引用）
│   ├── plugin/         # 插件核心接口
│   ├── message/        # 消息处理
│   ├── contact/        # 联系人管理
│   ├── chatroom/       # 聊天室管理
│   └── ...             # 其他能力包
├── host/                # 主机程序（插件宿主）
├── plugins/             # 官方插件集合
│   ├── example/        # 完整示例插件 ⭐
│   ├── ai/             # AI 对话插件
│   ├── cron/           # 定时任务插件
│   ├── meme/           # 表情包生成插件
│   ├── statistics/     # 消息统计插件
│   └── ...             # 更多插件
├── proto/              # Protocol Buffers 定义
└── README.md           # 本文档
```

## 🚀 快速开始

### 对于使用者

1. **获取主机程序**
   ```bash
   # 克隆项目
   git clone https://github.com/sbgayhub/golem.git
   cd golem
   
   # 构建主机
   go build -o golem.exe ./host
   ```

2. **配置插件**
   
   编辑 `plugins/config.toml` 启用所需插件：
   ```toml
   [ai]
   enable = true
   mode = 'blacklist'
   limits = []
   
   [ai.config]
   base_url = "https://api.openai.com/v1"
   api_key = "your-api-key"
   model = "gpt-4"
   ```

3. **运行**
   ```bash
   ./golem.exe
   ```

### 对于插件开发者

#### 方式一：基于 Example 示例（推荐）

```bash
# 1. 复制 Example 插件
cp -r plugins/example my-plugin
cd my-plugin

# 2. 修改插件信息
vim main.go  # 修改 GetMetadata() 中的名称、描述等

# 3. 删除不需要的功能，保留需要的部分

# 4. 构建
go mod tidy
go build
```

#### 方式二：从零开始

```bash
# 1. 创建项目
mkdir my-plugin && cd my-plugin
go mod init github.com/yourusername/my-plugin

# 2. 添加 SDK 依赖
go get github.com/sbgayhub/golem/sdk@v0.1.1

# 3. 创建插件代码
cat > main.go << 'EOF'
package main

import (
    "log/slog"
    "github.com/sbgayhub/golem/sdk/plugin"
)

type MyPlugin struct {
    plugin.ConfigAbility[Config]
}

type Config struct {
    Name string `toml:"name" comment:"插件名称"`
}

func (p *MyPlugin) GetMetadata() *plugin.Metadata {
    return &plugin.Metadata{
        Name:        "my-plugin",
        Version:     "0.1.0",
        Description: "我的插件",
        Author:      "Your Name",
    }
}

func (p *MyPlugin) OnLoad() error {
    slog.Info("插件加载成功")
    return nil
}

func main() {
    plugin.Start(&MyPlugin{})
}
EOF

# 4. 构建
go mod tidy
go build
```

## 📚 SDK 使用

### 安装依赖

```bash
go get github.com/sbgayhub/golem/sdk@v0.1.1
```

### 可用的包

```go
import (
    "github.com/sbgayhub/golem/sdk/plugin"      // 插件核心
    "github.com/sbgayhub/golem/sdk/message"     // 消息发送/接收/撤回
    "github.com/sbgayhub/golem/sdk/contact"     // 联系人/好友管理
    "github.com/sbgayhub/golem/sdk/chatroom"    // 群聊管理
    "github.com/sbgayhub/golem/sdk/moments"     // 朋友圈
    "github.com/sbgayhub/golem/sdk/payment"     // 支付
    "github.com/sbgayhub/golem/sdk/cdn"         // CDN 资源
    "github.com/sbgayhub/golem/sdk/favor"       // 收藏
    "github.com/sbgayhub/golem/sdk/label"       // 标签
    "github.com/sbgayhub/golem/sdk/miniapp"     // 小程序
    "github.com/sbgayhub/golem/sdk/official"    // 公众号
)
```

### 本地开发

如果需要同时修改 SDK 和插件，可以使用本地替换：

```go
// go.mod
module my-plugin

require github.com/sbgayhub/golem/sdk v0.1.1

// 仅本地开发时使用
replace github.com/sbgayhub/golem/sdk => ../sdk
```

## 📖 文档

- [插件开发指南](plugins/readme.md) - 完整的插件开发教程
- [Example 插件说明](plugins/example/readme.md) - 完整示例插件详解

## 🔌 官方插件

| 插件 | 描述 | 版本 |
|------|------|------|
| **ai** | AI 对话插件，支持 OpenAI 兼容接口 | v1.0.0 |
| **cron** | 定时任务插件，支持定时消息发送 | v0.0.1 |
| **meme** | 表情包生成插件，支持多种模板 | v1.0.0 |
| **statistics** | 消息统计插件，群发言排行 | v1.0.0 |
| **news** | 新闻推送插件，支持今日热点 | v0.0.1 |
| **reread** | 复读机插件，复读消息和表情 | v1.0.0 |
| **universal** | 规则驱动的通用 API 请求插件 | v1.0.0 |
| **video_parser** | 视频在线解析插件 | v1.0.0 |
| **gg** | 图片生成插件，基于 gg 库 | v0.0.0 |
| **example** | 基础示例插件，展示核心功能 | v1.0.0 |

详细说明请查看 [plugins/readme.md](plugins/readme.md)

## 🎮 系统命令

系统提供了内置命令用于管理插件和联系人。

### ⚠️ 安全提示

**务必配置 Owner（机器人管理员）！**

在 `host/data/config.toml` 中配置：
```toml
owner = "wxid_xxx"              # 推荐：使用 ID
# 或使用其他匹配方式
owner = "nickname::张三"         # 按昵称
owner = "remark::管理员"          # 按备注
```

- **未配置 Owner**：所有人都可执行命令，存在信息泄露风险
- **已配置 Owner**：只有 Owner 可执行命令，确保安全

### 插件管理 (/pm)
```bash
/pm list              # 查看所有插件
/pm reload <name>     # 热更新插件
/pm enable <name>     # 启用插件
/pm disable <name>    # 禁用插件
/pm info <name>       # 查看插件信息
```

### 联系人缓存 (/cm)
```bash
/cm contact           # 刷新联系人缓存
/cm chatroom          # 刷新群成员缓存（群聊中使用）
```

详细命令说明请查看 [插件开发指南](plugins/readme.md#系统内置命令)

## 🛠 开发环境

### 要求

- Go 1.26+
- Git
- Protocol Buffers 编译器（仅需修改 proto 时）

### 工作区模式

本项目使用 Go workspace 管理多模块：

```bash
# 克隆项目
git clone https://github.com/sbgayhub/golem.git
cd golem

# 所有模块自动关联，无需手动 go get
go build ./host

# 构建插件
cd plugins/ai
go build
```

### 编译所有插件

```bash
# 安装 Task 工具（如果未安装）
go install github.com/go-task/task/v3/cmd/task@latest

# 使用 Task 构建所有插件
cd plugins
task build-all

# 或手动编译
for dir in */; do
    cd "$dir"
    go build
    cd ..
done
```

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

### 提交插件

如果你开发了有趣的插件，欢迎提交到 `plugins/` 目录：

1. Fork 本项目
2. 在 `plugins/` 下创建你的插件目录
3. 提交 PR，包含：
   - 插件代码
   - README 说明
   - 配置示例

### 开发规范

- 遵循 Go 代码规范
- 插件名使用小写加下划线
- 提供清晰的配置注释
- 处理好错误和边界情况

## 免责声明

- 本项目仅供 **学习研究** 使用，严禁用于任何违反法律法规或微信使用条款的用途
- 使用者应自行承担使用本项目所产生的一切风险和责任
- 本项目不提供任何形式的担保，作者不对因使用本项目造成的任何损失负责
- 请勿将本项目用于批量营销、骚扰他人、传播违法信息等非法用途
- 如有侵权请联系删除

## 📄 许可证

[MIT License](LICENSE)

## 📮 联系方式

- **GitHub**: https://github.com/sbgayhub/golem
- **Issues**: https://github.com/sbgayhub/golem/issues

## ⭐ Star History

如果这个项目对你有帮助，请给个 Star ⭐

---

