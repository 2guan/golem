# Golem 🤖

<p align="center">
  <a href="https://github.com/sbgayhub/golem"><img src="https://img.shields.io/badge/Forked%20From-sbgayhub%2Fgolem-2ea44f?style=flat-square&logo=github" alt="Forked from sbgayhub/golem"></a>
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/License-MIT-green.svg?style=flat-square" alt="License">
  <img src="https://img.shields.io/badge/Platform-macOS%20%7C%20Linux%20%7C%20Windows-blue?style=flat-square" alt="Platform">
  <img src="https://img.shields.io/badge/Architecture-Plugin%20IPC-orange?style=flat-square" alt="Plugin Architecture">
</p>

**Golem** 是一个高性能、高扩展性的微信机器人插件系统与大模型交互框架。底层基于 `hashicorp/go-plugin` 实现进程级解耦与热更新能力，内置现代化 Web 运维控制台、多厂商大模型容灾调度、原生微信语音条克隆合成（TTS Voice Clone / Voice Design）以及人设提示词编排管理。

> [!NOTE]
> **开源致谢与上游溯源**：
> 本项目 Fork 并基于上游优秀开源项目 [sbgayhub/golem](https://github.com/sbgayhub/golem) 持续演进与增强。在此衷心感谢原作者与开源社区构建的坚实微内核与插件架构基石！
> 
> **本分支主要增强**：集成现代化 Web 运维控制台、大模型调用顺位容灾流水线（Failover Pipeline）、微信原生 Silk 语音条零样本声音克隆（Zero-Shot Voice Clone）、Voice Design 自然语言音色设计、以及全局系统提示词/人设库可视化管理系统。

> [!IMPORTANT]

> **免责声明**：本项目仅供学习、研究与个人交流使用。请严格遵守当地法律法规及微信服务协议，严禁将本项目用于群发营销、电信诈骗、传播不良信息或任何非法用途。作者不对使用本项目产生的任何后果承担法律责任。

---

## ✨ 核心特性

### 1. 🔌 微内核与跨进程插件架构
- **进程隔离与高稳定性**：插件作为独立子进程通过 Unix Domain Socket / 本地 IPC 与宿主（Host）通信，单个插件崩溃不影响主程序运行。
- **动态热加载 / 热重载**：支持不重启宿主服务的情况下动态启用、停用、更新与重载任意插件（`/pm reload <name>`）。
- **解耦的独立 SDK**：提供 `github.com/sbgayhub/golem/sdk`，插件开发者无需克隆完整仓库即可快速导入开发。
- **全能力支持**：覆盖文本/图片/原生语音/视频/表情/撤回等消息收发、联系人与群聊管理、朋友圈、小程序与支付事件。

### 2. 🧠 多厂商大模型智能调度与容灾链（Failover Pipeline）
- **多厂商兼容**：开箱即用支持各大主流大语言模型（如小米 MiMo、Google Gemini、OpenAI、DeepSeek、Moonshot、硅基流动等 OpenAI 兼容接口）。
- **顺位故障容灾降级（Failover Pipeline）**：支持在 Web 控制台按优先级拖拽排序调用链，当首选模型遇到超时、限流（HTTP 429）或云端内容风控拦截时，系统全自动无缝顺位切换至备用模型，保证对话不中断。
- **微信交互深度适配**：
  - **真实打字节奏**：自动解析提示词中的多段落标记并拆分为多个微信气泡相继发出，避免大段“小作文”造成的机械感。
  - **群聊多人社交规范**：精准区分私聊与群聊，严格隔离群成员与自身身份，杜绝认领他人生活照与赞美等尴尬现象。
  - **防越狱与防注入防线**：内置环境时间感知、思维链过滤与指令对抗机制，保障角色稳定沉浸。

### 3. 🎙️ 微信原生语音合成与声音复刻（TTS Engine）
- **微信原生语音条（Silk v3）**：直出微信原生音频格式，支持红点播放与语音转文字。
- **Voice Design 自然语言音色设计**：无需音频样本，直接通过自然语言描述定制独一无二的声线、共鸣与说话质感。
- **Zero-Shot 声音克隆（Voice Clone）**：上传 5~30 秒干净说话音频样本，即可高保真复刻目标声线并生成语音条。
- **声音动作情绪渲染**：智能解析 `<voice>` 标签中的微动作音效代码（`[轻笑]`、`[叹气]`、`[低笑]`、`[停顿]`、`[微喘]`），合成带有真实呼吸感与情绪起伏的温度声音。

### 4. 🖥️ 现代化 Web 可视化管理控制台（Dashboard）
- **自适应设计**：采用极简高质感设计风格，原生支持深色/浅色模式无缝自适应切换。
- **模型配置中心**：
  - `💬 文字对话模型 (LLM)`：可视化多模型接口管理、调用顺位流水线拖拽与实时连通性测试。
  - `🎙️ TTS 语音合成模型 (Voice)`：发音人配置、声音克隆样本管理与浏览器端在线即时试听测速。
  - `🎭 系统提示词管理 (Prompts)`：多套通用人设库管理、一键切换全局生效人设、长文本代码编辑器与字数统计。
- **会话与专属策略覆盖（Target Overrides）**：为特定联系人或微信群单独定制专属提示词、回复概率、静默监听模式与独立模型。
- **Web 实时网页聊天控制台**：直接在浏览器端查看会话历史、发送文字/图片/语音条并一键清空上下文记忆。
- **插件市场与实时运行监控**：可视化插件开关、配置表单编辑与实时日志流查看。

---

## 📁 目录结构

```
golem/
├── host/                    # Golem 核心宿主进程（插件生命周期与 IPC 调度）
├── sdk/                     # 独立插件开发 SDK（消息/联系人/群聊/朋友圈等抽象）
├── plugins/                 # 官方与扩展插件目录
│   ├── ai/                  # AI 对话、提示词工程与 TTS 语音合成引擎
│   ├── dashboard/           # Web 可视化控制台与 RESTful API 服务
│   ├── cron/                # 定时任务触发插件
│   ├── meme/                # 表情包合成与图像生成
│   ├── statistics/          # 群聊发言统计与排行榜
│   ├── auto_accept/         # 自动同意好友申请与新好友问候
│   ├── news/                # 今日热点新闻推送
│   ├── reread/              # 智能复读机
│   └── example/             # 官方推荐插件开发模版 ⭐
├── proto/                   # gRPC / IPC 通信 Protocol Buffers 定义
├── data/                    # 运行时数据与历史记录存储（已 gitignore）
├── tools/                   # Silk 编码器与媒体处理工具
├── start.sh / stop.sh       # 守护进程管理脚本
└── plugins/config.example.toml # 全局配置模板
```

---

## 🚀 快速上手

### 环境要求
- **Go**: 1.26 或更高版本
- **FFmpeg & FFprobe**: 用于音频格式转换（`brew install ffmpeg` 或 `apt-get install ffmpeg`）
- **Git**

### 1. 克隆代码与构建

```bash
# 克隆仓库
git clone https://github.com/sbgayhub/golem.git
cd golem

# 编译宿主核心程序
go build -o golem ./host

# 编译核心插件（如 ai 与 dashboard）
go build -o plugins/ai/golem_plugin_ai ./plugins/ai
go build -o plugins/dashboard/golem_plugin_dashboard ./plugins/dashboard
```

> **提示**：也可以使用项目内置的 `Taskfile.yml` 通过 `task build-all` 批量编译所有官方插件。

### 2. 初始化配置文件

从示例模板复制全局配置文件：

```bash
cp plugins/config.example.toml plugins/config.toml
```

根据需要在 `plugins/config.toml` 中配置大模型提供商 API Key 或启动参数。

### 3. 运行与登录

```bash
# 前台调试运行
./golem

# 或使用后台脚本启动
./start.sh
```

- 启动后，打开浏览器访问 **`http://127.0.0.1:8899`**
- 默认管理员账号：`admin` / 密码：`admin123`
- 在 Web 控制台右上角可自由修改账号密码与安全配置。

---

## 🛠️ 插件开发指南

Golem 提供了极度精简的插件开发体验，开发者仅需实现 `Plugin` 接口并嵌入所需的能力模块（`Ability`）。

### 创建属于你的插件

```go
package main

import (
    "log/slog"
    "github.com/sbgayhub/golem/sdk/message"
    "github.com/sbgayhub/golem/sdk/plugin"
)

type MyPlugin struct {
    plugin.ConfigAbility[MyConfig]
}

type MyConfig struct {
    Greeting string `toml:"greeting" comment:"问候语"`
}

func (p *MyPlugin) GetMetadata() *plugin.Metadata {
    return &plugin.Metadata{
        Name:        "my_plugin",
        Version:     "1.0.0",
        Description: "我的首个 Golem 插件",
        Author:      "Developer",
    }
}

func (p *MyPlugin) OnLoad() error {
    slog.Info("插件加载成功！当前问候语: " + p.Config.Greeting)
    return nil
}

func main() {
    plugin.Start(&MyPlugin{})
}
```

详细教程请参考 [插件开发文档 (plugins/readme.md)](plugins/readme.md)。

---

## 📦 官方插件矩阵

| 插件名称 | 目录 | 核心功能说明 |
|:---|:---|:---|
| **ai** | `plugins/ai` | 大模型多厂商调度、容灾链切换、提示词管理与微信原生 TTS 语音合成 |
| **dashboard** | `plugins/dashboard` | 现代化 Web 运维控制台、网页聊天器、模型配置中心与热更新 |
| **cron** | `plugins/cron` | 高精度 Cron 定时任务编排与定时消息推送 |
| **auto_accept** | `plugins/auto_accept` | 自动化关键词筛选通过好友申请并自动回复个性化欢迎语 |
| **avatar_review** | `plugins/avatar_review` | 群成员与好友头像大模型视觉智能审查 |
| **meme** | `plugins/meme` | 经典表情包文字合成与生成 |
| **statistics** | `plugins/statistics` | 群聊活跃度分析、发言排行榜与消息量监控 |
| **news** | `plugins/news` | 每日早晚报与热点新闻检索推送 |
| **reread** | `plugins/reread` | 智能自然复读与趣味互动 |
| **example** | `plugins/example` | 官方标准示范插件，展示全部核心能力接入方式 |

---

## 🛡️ 安全规范与建议

1. **配置机器人所有者（Owner）**：
   在 `host/data/config.toml` 中配置你的微信号或微信号 ID 作为 Owner，未配置时管理指令将被拦截或限制。
2. **私有配置保护**：
   所有包含 API Key、个人账号隐私的 `plugins/config.toml` 与 `data/` 数据已在 `.gitignore` 中默认排除，请勿将敏感配置上传至公开分支。
3. **风控合规**：
   建议使用小号登录，避免在短时间内高频次触发发送敏感文本或进行大批量好友操作。

---

## 🤝 贡献与反馈

欢迎提交 Issue 与 Pull Request 共同完善 Golem 生态！
- 提交 Bug 或功能建议：[GitHub Issues](https://github.com/sbgayhub/golem/issues)
- 遵循 Go 代码规范（`gofmt`）并保证单元测试通过。

## 💖 开源致谢与上游项目 (Upstream)

本项目基于上游开源项目发展而来，特此致谢：
- **上游官方仓库**：[sbgayhub/golem](https://github.com/sbgayhub/golem)
- **协议**：MIT License
- 感谢上游作者 [@sbgayhub](https://github.com/sbgayhub) 及开源社区贡献者们无私分享的高质量微内核通信与插件架构！

---

## 📄 开源许可证

本项目基于 [MIT License](LICENSE) 协议开源。

