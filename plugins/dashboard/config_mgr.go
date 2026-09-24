package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"
)

// PluginMeta 插件展示元数据
// PluginUsageInfo 插件详细使用说明元数据
type PluginUsageInfo struct {
	Triggers    []string `json:"triggers"`    // 触发关键词/命令
	Scope       string   `json:"scope"`       // 生效场景（群聊/私聊/全局）
	Examples    []string `json:"examples"`    // 典型交互示例
	Description string   `json:"description"` // 运行机制与核心玩法
	Tips        string   `json:"tips"`        // 注意事项与优化建议
}

type PluginMeta struct {
	Title       string
	Description string
	Icon        string
	Category    string
	Usage       PluginUsageInfo
}

// PluginMetaMap 预置所有官方与内建插件的中文名称、详细功能介绍与使用指南
var PluginMetaMap = map[string]PluginMeta{
	"ai": {
		Title:       "AI 智能对话大模型",
		Description: "核心多模态大模型交互引擎，支持 Gemini 与小米 Mimo，具备长上下文记忆、拟真语气与单人/单群专属人设支持。",
		Icon:        "🤖",
		Category:    "AI 核心",
		Usage: PluginUsageInfo{
			Triggers: []string{"直接私聊发送消息", "@机器人 [聊天内容]", "引用机器人发言回复", "群聊自由聊天(由回复率决定)"},
			Scope:    "群聊 / 私聊全场景支持",
			Examples: []string{
				"私聊直接发送：“今天心情有点低落，陪我聊聊” -> AI 拟真安慰与共情回复",
				"群聊发送：“@机器人 帮我写一份下周工作计划与排期” -> 条理化输出结构化建议",
				"微信发送图片：“@机器人 看看这道题怎么解答” -> 自动调用多模态 Vision 视觉模型识图解答",
			},
			Description: "AI 对话大脑与智能核心，具备长程上下文记忆机制与性格语气。可在后台「专属策略」模块中针对特定联系人或群定制独立提示词（System Prompt）、模型提供方与独立回复概率。",
			Tips:        "若群内发言频繁担心打扰其他群友，可在后台「专属策略」中开启该群的“单目标静默模式”或将回复率调低（如 0.1）。",
		},
	},
	"dashboard": {
		Title:       "Web 管理控制台",
		Description: "提供可视化后台页面，管理全局参数、插件管理、单人专属人设、全模态聊天审计与独立日志诊断中心。",
		Icon:        "📊",
		Category:    "系统管理",
		Usage: PluginUsageInfo{
			Triggers: []string{"浏览器访问 http://127.0.0.1:8899"},
			Scope:    "Web 浏览器管理端",
			Examples: []string{
				"登录后台 -> 实时查看系统运行状态、插件开关与通讯录",
				"「插件管理」-> 查阅所有插件详细使用指南，或可视化微调 TOML 专属参数",
				"「实时对话」-> 管理员以当前机器人身份直接向联系人或群聊推送微信消息并同步 AI 记忆",
				"「系统日志」-> 实时跟踪各模块运行日志、过滤 Warning 与 Error 异常",
			},
			Description: "Golem 的一站式可视化 Web 管理后台。采用单二进制自包含嵌入式架构，免外部前端依赖部署，开箱即用。",
			Tips:        "默认管理员账号为 admin / 密码为 admin123，初次部署后建议前往「安全与管理」菜单修改高强度密码。",
		},
	},
	"auto_accept": {
		Title:       "自动通过好友与欢迎语",
		Description: "全自动处理新的微信好友添加申请，发送预置个性化欢迎语，并实时向所有者推送好友验证通知。",
		Icon:        "👋",
		Category:    "好友管理",
		Usage: PluginUsageInfo{
			Triggers: []string{"微信收到新好友添加申请时全自动触发"},
			Scope:    "好友添加申请（私聊）",
			Examples: []string{
				"陌生人扫码或通过群聊发起加好友申请 -> Golem 秒级自动同意验证请求",
				"通过后延迟设定的秒数，机器人自动主动发送热情个性化欢迎词与功能菜单",
				"微信实时向管理员微信推送一条提醒通知：“收到新好友添加通知：[昵称]”",
			},
			Description: "全天候自动化接管好友申请审批与新客破冰流程。支持设置关键词过滤（只有验证申请消息中包含特定关键词才自动通过），支持配置欢迎语发送延迟（模拟真人反应时间）。",
			Tips:        "可在本后台「配置参数」中灵活修改欢迎语文本、通知开关以及验证关键词过滤规则。",
		},
	},
	"avatar_review": {
		Title:       "好友头像锐评",
		Description: "调用多模态视觉大模型对微信好友头像进行幽默犀利、情商在线的多维度艺术构图与性格点评。",
		Icon:        "🎨",
		Category:    "AI 趣味",
		Usage: PluginUsageInfo{
			Triggers: []string{"评测头像", "锐评头像", "看头像", "头像打分", "测头像", "打分头像", "我的头像", "看看头像"},
			Scope:    "群聊 / 私聊均支持（群聊中可 @机器人 亦可直接发送）",
			Examples: []string{
				"群内发送：“评测头像” -> 机器人调取你的高清微信头像，输出 0-100 分美学打分、构图与色调点评、潜在性格潜台词与诙谐改造建议",
				"群内发送：“@机器人 锐评头像” -> 当场公布幽默犀利的毒舌鉴赏报告",
			},
			Description: "提取用户当前在微信中的真实头像图片，送入多模态大模型进行美学、情绪与性格维度解构，兼具趣味性与社交传播度。",
			Tips:        "需要确保 AI 模型具备 Vision 图像识别能力（默认配置的小米 Mimo 具备完整多模态能力）。",
		},
	},
	"statistics": {
		Title:       "群发言统计与排行",
		Description: "记录群内每条发言，提供群活跃度排行榜、每日水群榜、词数统计以及历史发言跨群查询能力。",
		Icon:        "📈",
		Category:    "数据分析",
		Usage: PluginUsageInfo{
			Triggers: []string{"今日排行", "昨日排行", "本周排行", "本月排行", "总排行", "发言详情"},
			Scope:    "微信群聊专属",
			Examples: []string{
				"群内发送：“今日排行” -> 输出今日水群活跃榜 Top 10，列出群友发言条数与百分比",
				"群内发送：“昨日排行” / “本周排行” -> 汇总更长周期的水群龙王榜单",
				"群内发送：“发言详情” -> 详细展示群内不同时段活跃曲线与成员互动分布",
			},
			Description: "基于本地轻量 SQLite 数据库，无感记录群内每条消息的时间、发送者与字数，并对外暴露 statistics.query_messages 能力供词云、画像等高级插件复用。",
			Tips:        "发言统计按群完全物理隔离，保护各群隐私；私聊中不触发排行榜指令。",
		},
	},
	"wordcloud": {
		Title:       "群聊词云图生成",
		Description: "自动分词提炼群聊高频讨论热词，并渲染合成具有强烈视觉冲击力的高清群聊词云图片。",
		Icon:        "☁️",
		Category:    "数据分析",
		Usage: PluginUsageInfo{
			Triggers: []string{"词云", "词云 今日", "词云 昨日", "词云 本周", "词云 本月", "词云 [N]天", "词云 @某人"},
			Scope:    "微信群聊专属",
			Examples: []string{
				"群内发送：“词云” -> 提取近 7 天群聊核心讨论热词，合成并发送高清彩色词云图片",
				"群内发送：“词云 今日” -> 即时汇总今天群友聊得最多的话题词汇",
				"群内发送：“词云 @张三” -> 针对指定群友的历史发言生成其个人专属词云肖像",
			},
			Description: "调用底层 statistics 历史发言，结合中文分词结巴算法（Jieba）过滤虚词与符号，按 TF-IDF 权重精美排版并渲染出高清词云图。",
			Tips:        "依赖 statistics 插件提供消息历史；群内需积累一定发言量（建议 20 条以上）方能呈现丰富词云效果。",
		},
	},
	"moyu_daily": {
		Title:       "摸鱼日历推送",
		Description: "每日定时生成打工人摸鱼办温馨日历，提醒距离周末、发薪日与法定节假日的实时倒计时。",
		Icon:        "🐟",
		Category:    "日常推送",
		Usage: PluginUsageInfo{
			Triggers: []string{"摸鱼", "摸鱼办", "摸鱼日报", "摸鱼日历", "摸鱼倒计时", "今日摸鱼", "工作日 10:00 自动定时推送"},
			Scope:    "群聊 / 私聊均支持",
			Examples: []string{
				"发送：“摸鱼日历” -> 立即返回打工人摸鱼办日历：精准测算距离本周末、发薪日以及元旦/春节/清明/劳动节等下一个假期的倒计时，附带一句防内卷扎心金句",
				"工作日 10:00：自动向配置的群聊列表中推送当天的摸鱼早报",
			},
			Description: "打工人灵魂伴侣，内置法定节假日实时日历算法，提醒群友放慢节奏、劳逸结合。",
			Tips:        "在插件配置中的 auto_push_chatrooms 添加目标群的 wxid（如 xxx@chatroom），即可开启工作日全自动定时推送。",
		},
	},
	"what_to_eat": {
		Title:       "今天吃什么",
		Description: "解决世纪难题，根据早中晚餐随机推荐美食菜单，支持摇号、随机挑选与特色菜谱推荐。",
		Icon:        "🍲",
		Category:    "生活工具",
		Usage: PluginUsageInfo{
			Triggers: []string{"今天吃什么", "中午吃什么", "晚上吃什么", "夜宵吃什么", "早饭吃什么", "换一个", "换道菜", "吃什么 减脂", "不想吃面", "吃什么帮助"},
			Scope:    "群聊 / 私聊均支持",
			Examples: []string{
				"发送：“今天吃什么” -> 自动感知当前时间（早餐/午餐/下午茶/晚餐/夜宵），推荐一道招牌菜品、热量与老饕评语",
				"发送：“换一个” / “换道菜” -> 不合口味随时再换，系统自动记忆规避近期重复菜品",
				"偏好定制：“吃什么 减脂” / “吃什么 清淡” / “吃什么 爆辣” / “来点喝的”",
				"反向避雷：“不想吃米饭” / “不吃辣” / “不想喝奶茶” -> 智能排除对应类别",
				"私房定制：“吃什么 胃难受” / “吃什么 预算20” -> AI 大模型当场量身定制专属食谱",
			},
			Description: "智能时段感知美食盲盒，内置数百道中华与国际经典美食数据库，支持口味正向偏好过滤、反向精准避坑与大模型私房定制菜谱。",
			Tips:        "在配置中开启 enable_ai 后，遇到生僻或复杂预算/健康要求时将自动调用大模型定制菜谱。",
		},
	},
	"cron": {
		Title:       "定时任务调度器",
		Description: "支持标准 5 字段 Cron 表达式的定时任务，可配置定时向指定群或好友自动推送消息或提醒。",
		Icon:        "⏰",
		Category:    "实用工具",
		Usage: PluginUsageInfo{
			Triggers: []string{"/cron list", "/cron add -c [cron] -p [能力] -t [目标]", "/cron delete -i [ID]", "后台 jobs 列表配置"},
			Scope:    "微信管理员私聊 / 全局系统调度",
			Examples: []string{
				"微信私聊发送：“/cron list” -> 列出当前所有运行中的定时任务与下次执行时间",
				"私聊发送：“/cron add -c \"0 9 * * *\" -p news.today -t 123456@chatroom” -> 每天早上 9:00 向群推送早报",
				"私聊发送：“/cron delete -i 1” -> 删除序号为 1 的定时任务",
			},
			Description: "基于 Robfig Cron 工业级标准时间引擎，将 Golem 中各插件暴露的能力（如 news.today、text.to.image）按时间计划自动化执行并分发至指定好友或群聊。",
			Tips:        "更推荐直接在本管理后台中通过可视化表单编辑 jobs 列表，修改后一键保存自动热加载生效。",
		},
	},
	"news": {
		Title:       "每日早报 60秒",
		Description: "聚合每日全网 60 秒简报，精选国内外最新大事新闻，晨间自动推送早安资讯卡片。",
		Icon:        "📰",
		Category:    "日常推送",
		Usage: PluginUsageInfo{
			Triggers: []string{"今日新闻", "今日图卦"},
			Scope:    "群聊 / 私聊均支持",
			Examples: []string{
				"发送：“今日新闻” -> 立即返回全网精选 60 秒大事要闻长图早报",
				"发送：“今日图卦” -> 返回每日新闻视点图卦卡片",
			},
			Description: "每日聚合权威新闻源要闻资讯，内置内存与本地双重图片缓存机制，当天高频查询无需重复爬取、秒级响应推送。",
			Tips:        "配合 cron 插件可实现每天早晨 8:30 自动向指定微信群或好友定时发送今日新闻早报。",
		},
	},
	"daily_gossip": {
		Title:       "每日娱乐与社会八卦",
		Description: "搜集全网当日最火爆的吃瓜八卦、娱乐圈动向与热搜事件，供群友闲聊讨论解闷。",
		Icon:        "🍉",
		Category:    "日常推送",
		Usage: PluginUsageInfo{
			Triggers: []string{"今日八卦", "群八卦", "吃瓜日报", "八卦日报", "吃瓜晚报", "群报", "每晚 22:00 自动推送"},
			Scope:    "微信群聊专属",
			Examples: []string{
				"群内发送：“今日八卦” -> 自动提取并盘点今天本群最劲爆的话题讨论、群友神吐槽与吃瓜排行榜",
				"每晚 22:00：系统自动为白天产生较多讨论的活跃群聊排版并下发当天的吃瓜晚报",
			},
			Description: "自动梳理群内一整天的热聊脉络与金句发言，提炼群友关注的高频话题，生成带排版的高质量群聊专属八卦小报。",
			Tips:        "若群内当天发言条数过少，晚报会自动跳过以保证推送内容的精彩度。",
		},
	},
	"music": {
		Title:       "网易云音乐点歌",
		Description: "在群聊中按歌名、歌手快速搜索音乐并分享网易云歌曲卡片，支持群内试听与点歌互动。",
		Icon:        "🎵",
		Category:    "多媒体",
		Usage: PluginUsageInfo{
			Triggers: []string{"点歌 [歌名]", "来首 [歌名]", "放首歌 [歌名]", "搜歌 [歌名/歌手]", "我想听 [歌名]"},
			Scope:    "群聊 / 私聊均支持",
			Examples: []string{
				"发送：“点歌 七里香” -> 搜索并分享周杰伦《七里香》网易云音乐卡片",
				"发送：“来首歌 孙燕姿 遇见” -> 精准搜索并推送匹配歌曲",
				"发送：“我想听 漠河舞厅” -> 自动识别自然语言意图并完成点播",
			},
			Description: "智能中文点歌语义分析器，自动识别歌名与歌手信息，同时过滤掉“我想听你说话”等非点歌日常用语，返回微信原生小程序音乐试听卡片。",
			Tips:        "点击推送的音乐卡片即可在微信中直接播放，支持群内一边聊天一边听歌。",
		},
	},
	"video_parser": {
		Title:       "短视频无水印解析",
		Description: "自动识别抖音、快手、小红书、B站等短视频分享链接，秒级解析出超清无水印直链视频。",
		Icon:        "🎬",
		Category:    "多媒体",
		Usage: PluginUsageInfo{
			Triggers: []string{"微信中直接粘贴/分享含有短视频链接的文本（无需特定命令）"},
			Scope:    "群聊 / 私聊全场景无感支持",
			Examples: []string{
				"群友在群内分享抖音/小红书短视频链接 -> 机器人秒级自动识别并抓取，直接回复无水印高清视频直链与标题封面卡片",
			},
			Description: "全自动无感嗅探解析引擎，支持抖音、快手、小红书、哔哩哔哩、微博等数十个主流平台的视频与图集分享链接，去除平台水印提取纯净原片直链。",
			Tips:        "全自动被动监听模式，无需配置任何前缀指令即可生效。",
		},
	},
	"setu": {
		Title:       "趣味美图与二次元",
		Description: "支持搜寻或生成二次元动漫精美壁纸与高清图片，支持多种关键字与分类标签检索。",
		Icon:        "🌸",
		Category:    "娱乐交互",
		Usage: PluginUsageInfo{
			Triggers: []string{"plmm", "漂亮妹妹", "来点美女", "美女视频", "来点黑丝", "来点白丝", "看看腿", "来点帅哥", "帅哥视频", "来点[关键词]"},
			Scope:    "群聊 / 私聊均支持",
			Examples: []string{
				"发送：“plmm” / “来点美女” -> 随机发送高质量美图（按概率出视频）",
				"发送：“来点黑丝” / “来点白丝” -> 随机推送穿搭美图或短视频",
				"发送：“来点柯基” / “来点猫咪” -> 自动以关键词联网搜图并发送",
			},
			Description: "高可用娱乐图文/视频分发插件，集成优质壁纸与视频接口，支持关键词搜图与图/视频随机按权重比例推送。",
			Tips:        "可在配置参数中调整 VideoRate（视频概率，默认 50）以及各图源接口地址。",
		},
	},
	"robbery": {
		Title:       "群打劫小游戏",
		Description: "趣味群内互动游戏，支持群友之间互相打劫金币、被反杀防守与黑吃黑搞笑惩罚。",
		Icon:        "💰",
		Category:    "群内小游戏",
		Usage: PluginUsageInfo{
			Triggers: []string{"打劫", "我的资产", "排行榜", "职业列表", "转职 [职业]", "购买 [装备]", "技能", "商店", "任务", "救济", "打劫帮助"},
			Scope:    "微信群聊专属",
			Examples: []string{
				"群内发送：“打劫” -> 随机挑选群内一名成员实施打劫，成功掠夺金币，失败可能被反杀或关入大牢",
				"群内发送：“我的资产” -> 查询自己的金币、当前职业、装备加成与打劫胜率",
				"群内发送：“转职 刺客” / “购买 锁子甲” -> 提升职业专属战斗力与防守成功率",
				"群内发送：“救济” -> 破产时领取低保金币东山再起",
			},
			Description: "极具互动黏性与笑料的群内文字 RPG 小游戏，包含职业克制体系、装备交易商店、每日赏金任务与监狱保释机制，能极大激活沉寂群聊。",
			Tips:        "如需在严肃商务群禁用，可在插件管理中将该群加入黑名单 limits 即可。",
		},
	},
	"farm": {
		Title:       "欢乐小农场",
		Description: "群内虚拟农场种植模拟，包括翻土、播种、施肥、成熟收获与好友互相偷菜互动。",
		Icon:        "🌾",
		Category:    "群内小游戏",
		Usage: PluginUsageInfo{
			Triggers: []string{"农场", "我的农场", "种植 [作物]", "收菜", "偷菜 @群友", "浇水", "农场购买 [种子]", "购买土地", "农场等级", "农场帮助"},
			Scope:    "微信群聊专属",
			Examples: []string{
				"群内发送：“我的农场” -> 渲染查看自己的土地状态、作物生长倒计时与金币余额",
				"群内发送：“农场购买 白菜 5” -> 种子商店采购种子，发送：“种植 白菜”完成播种",
				"群内发送：“偷菜 @张三” -> 潜入好友农场偷摘成熟蔬菜，体验经典偷菜乐趣",
				"群内发送：“浇水 @李四” -> 助人为乐增加经验值",
			},
			Description: "高度还原经典社交农场玩法的文字群聊游戏，涵盖土地开垦、农作物升级、化肥加速、防盗看护犬与偷菜互动。",
			Tips:        "偷菜存在被防盗犬咬伤扣除金币的风险，合理安排作物成熟时间防止被偷光。",
		},
	},
	"turtle_soup": {
		Title:       "海龟汤情境推理",
		Description: "经典悬疑情境推理游戏，机器人担任主持人，群友通过提问“是/否/无关”逐步推理真相。",
		Icon:        "🐢",
		Category:    "群内小游戏",
		Usage: PluginUsageInfo{
			Triggers: []string{"来碗海龟汤", "开局海龟汤", "私房海龟汤", "海龟汤提示", "看汤底", "海龟汤帮助"},
			Scope:    "群聊 / 私聊均支持",
			Examples: []string{
				"发送：“来碗海龟汤” -> 机器人给出一段离奇悬疑的故事汤面（如：盲人下火车喝水后自杀）",
				"群友自由提问：“死者是在车上受了重伤吗？” -> 机器人根据汤底裁决：“【不是】”",
				"群友提问：“死者的眼睛在车上治好了吗？” -> 机器人提示：“【关键线索！】”",
				"推理受阻时发：“海龟汤提示”；最后还原真相发：“看汤底”揭晓全部情节",
				"发送：“私房海龟汤” -> 由大模型现熬从未公开过的全新独家悬疑名案",
			},
			Description: "经典情境推理文字游戏。内置数十道经典推理案底，更支持大模型现场编写全新私房故事并担任主持人判定提问。",
			Tips:        "支持群内多位探长同时提问互动，非常适合朋友聚会或群内破冰解谜。",
		},
	},
	"undercover": {
		Title:       "谁是卧底聚会游戏",
		Description: "经典文字卧底聚会游戏，支持群内自动发词、隐藏卧底、轮流发言、投票处决与胜负判定。",
		Icon:        "🕵️",
		Category:    "群内小游戏",
		Usage: PluginUsageInfo{
			Triggers: []string{"发起谁是卧底", "报名 / +1", "开始游戏", "进入投票", "投[N]号", "结束谁是卧底", "谁是卧底帮助"},
			Scope:    "微信群聊专属（配合私聊发词）",
			Examples: []string{
				"群内发送：“发起谁是卧底” -> 开启房间报名（支持 3-8 人）",
				"群友回复：“+1” 或 “报名” -> 报名完毕发起人发送：“开始游戏”",
				"机器人私聊发送词语（平民词 vs 卧底词） -> 玩家群内轮流发言描述 -> 发送：“进入投票”投票处决卧底",
			},
			Description: "全自动主持经典社交聚会桌游「谁是卧底」，支持群内报名招募、私聊保密发词、发言轮次引导、票数自动统计与阵营胜负揭晓。",
			Tips:        "为接收私聊发放的专属卧底词语，群友参赛前需先添加机器人微信好友。",
		},
	},
	"heartbeat_club": {
		Title:       "心跳俱乐部",
		Description: "群聊交友互动破冰小游戏，支持真心话大冒险、好友默契度测试与匿名心动匹配。",
		Icon:        "💓",
		Category:    "群内小游戏",
		Usage: PluginUsageInfo{
			Triggers: []string{"@某人 掰手腕", "@某人 比拼身材", "微醺大冒险", "酒吧摇骰", "宿管查寝", "契合度 @某人", "心跳沉浸馆", "心跳帮助"},
			Scope:    "群聊荷尔蒙互动 + 私聊 1V1 沉浸互动剧场",
			Examples: []string{
				"群内：“@张三 掰手腕” -> 实时比拼双方力量值，输出热血搞笑胜负解说",
				"群内：“契合度 @李四” -> 测算两人性张力与心动契合指数",
				"私聊发送：“心跳沉浸馆” -> 选择场景（铁馆更衣室/微醺清吧/男生宿舍），与 AI 开启电影级 1V1 自由交互剧场",
			},
			Description: "专为情调破冰打造的荷尔蒙交互空间。群聊主打快节奏力量比拼与微醺大冒险；私聊主打多重电影感沉浸式即兴角色扮演剧场。",
			Tips:        "私聊沉浸剧场中自由打字即可推动剧情演进，随时发送“离开场景”即可退出剧场。",
		},
	},
	"infinite_adventure": {
		Title:       "无限冒险文字RPG",
		Description: "基于大模型实时演进的文字无限流地牢冒险 RPG，分支剧情由玩家指令动态即时生成。",
		Icon:        "⚔️",
		Category:    "群内小游戏",
		Usage: PluginUsageInfo{
			Triggers: []string{"开启冒险 [主题]", "双人冒险 @群友 [主题]", "创建小队 [主题]", "行动 [动作描述]", "解散队伍", "结束冒险"},
			Scope:    "群聊 / 私聊均支持",
			Examples: []string{
				"发送：“开启冒险 赛博朋克深空空间站” -> 大模型即时生成世界观背景、角色属性与危机开局",
				"发送：“行动 拔出腰间短刃刺向仿生人守卫” -> AI 判定行动成功率与局势变动，实时演进下个场景",
				"群聊发送：“双人冒险 @群友 克苏鲁古宅探秘” -> 开启双人组队探险跑团",
			},
			Description: "基于大模型全自由演进的无限流跑团冒险，无任何死板固定剧本，玩家的每一次决策和脑洞都会实时重塑故事世界线。",
			Tips:        "输入“解散队伍”或“结束冒险”可随时结算当前故事。",
		},
	},
	"meme": {
		Title:       "表情包制作生成",
		Description: "根据群友发言动态合成熊猫头、金馆长、汪汪队等流行带字表情包图片与动图。",
		Icon:        "🤪",
		Category:    "娱乐交互",
		Usage: PluginUsageInfo{
			Triggers: []string{"meme list", "表情 list", "meme [名称] [文字] [@群友]", "表情 [名称] [文字] [@群友]"},
			Scope:    "群聊 / 私聊均支持",
			Examples: []string{
				"群内发送：“表情 揍 @群友” -> 自动抓取该群友的真实头像，合成并发送对其暴揍的搞笑动图表情包",
				"群内发送：“表情 赞 你真棒” -> 动态合成带自定义文字的点赞表情包",
				"发送：“meme list” -> 列出当前系统支持的全部表情包模板清单",
			},
			Description: "融合头像抓取与动态文本排版渲染的表情包引擎，支持动图 GIF 与高清静态图，快速生成群友间恶搞调侃表情包。",
			Tips:        "可在插件配置中配置 meme 渲染服务的后端 Url。",
		},
	},
	"fake_forward": {
		Title:       "聊天伪造与合并转发",
		Description: "支持生成逼真的多成员聊天记录合并转发卡片，用于趣味调侃、剧本演示与段子创作。",
		Icon:        "📑",
		Category:    "娱乐交互",
		Usage: PluginUsageInfo{
			Triggers: []string{"/fake chat 名字1:内容1|名字2:内容2"},
			Scope:    "群聊 / 私聊均支持",
			Examples: []string{
				"发送：“/fake chat 马化腾:今天全员发红包|雷军:Are you ok?|Bot:冲啊！” -> 自动生成一条逼真的微信合并转发聊天记录卡片",
			},
			Description: "趣味段子创作工具，能够将你指定的发言人与对话剧本，打包生成微信官方标准的「聊天记录」合并转发消息卡片。",
			Tips:        "不同发言人之间请用竖线“|”分隔，名字与发言内容用冒号“:”分隔。",
		},
	},
	"profile": {
		Title:       "群友人物性格画像",
		Description: "基于大模型深度分析群成员历史发言习惯，提炼性格标签、高频话题与表达风格并生成深度画像报告。",
		Icon:        "📇",
		Category:    "数据分析",
		Usage: PluginUsageInfo{
			Triggers: []string{"人物画像", "人物画像 @群友", "人物画像 成员名 #群名", "人物画像 --global"},
			Scope:    "微信群聊（查自己/群友）与私聊（管理员查全局）",
			Examples: []string{
				"群内发送：“人物画像” -> 提取你在该群的所有历史发言，输出性格潜台词、社牛/社恐指数、口癖标签与群内角色定位",
				"群内发送：“人物画像 @张三” -> 生成对群友张三的性格多维度心理画像报告",
			},
			Description: "调用 AI 大模型对群友历史发言进行深度语义分析、情绪基调判别与语言模式提取，构建生动趣味的个人画像报告。",
			Tips:        "群成员在群内需要积累一定数量的发言记录，以保证画像分析的饱满度与准确度。",
		},
	},
	"reread": {
		Title:       "人类本质复读机",
		Description: "智能检测群内连续发言，当多人发出相同内容时自动加入复读队伍，烘托群内活跃氛围。",
		Icon:        "🦜",
		Category:    "趣味互动",
		Usage: PluginUsageInfo{
			Triggers: []string{"群内连续 2 人发送完全一致的文字或表情时自动触发（无需手动输入指令）"},
			Scope:    "微信群聊专属",
			Examples: []string{
				"群友 A 发送：“太强了” -> 群友 B 紧接着发送：“太强了” -> 机器人自动跟风复读：“太强了”",
				"群友连续发送同一张表情包 -> 机器人自动跟发同一张表情包",
			},
			Description: "模拟微信群经典“人类的本质是复读机”社交文化。当检测到群内形成复读节奏时主动跟风一次，融入群聊活跃气氛。",
			Tips:        "同一句话在同一群中仅复读一次，绝不会造成机器人死循环复读刷屏。",
		},
	},
	"pawzochat": {
		Title:       "PawzoChat 宠物拟人聊天",
		Description: "专为萌宠打造的宠物拟人化互动聊天插件，支持喵星人与汪星人专属语言口吻风格。",
		Icon:        "🐾",
		Category:    "AI 趣味",
		Usage: PluginUsageInfo{
			Triggers: []string{"向配置了专属 routes 的目标联系人或群聊发送消息时触发"},
			Scope:    "群聊 / 私聊（由 routes 路由规则指定）",
			Examples: []string{
				"配置指定群对应猫咪角色 -> 群友与机器人互动时，自动以傲娇可爱的喵星人口吻与习惯回复",
			},
			Description: "专门用于连接 PawzoChat 拟人化宠物大模型角色服务，将特定好友或群聊无缝映射到特定宠物角色大脑中。",
			Tips:        "需在插件配置参数中填入有效的 PawzoChat BaseURL 与 Bearer Token。",
		},
	},
	"universal": {
		Title:       "通用规则引擎与 Webhook",
		Description: "提供灵活的自定义命令前缀响应，支持快速接入自定义脚本与外部 Webhook 接口。",
		Icon:        "⚡",
		Category:    "系统工具",
		Usage: PluginUsageInfo{
			Triggers: []string{"根据 plugins/config.toml 中 [universal] 配置的 rules 规则触发"},
			Scope:    "群聊 / 私聊均支持",
			Examples: []string{
				"配置规则匹配正则：“查天气 (.*)” -> 自动抓取城市名并请求外部 Webhook API 返回实时天气",
				"配置快捷指令回复特定图文模板或业务查询",
			},
			Description: "轻量级规则驱动转发网关，支持基于正则表达式匹配消息、提取参数并向外部 HTTP 服务发起请求，将响应格式化回推微信。",
			Tips:        "在后台插件管理的 [universal] 专属参数中可自由添加多条 rules 映射规则。",
		},
	},
	"gg": {
		Title:       "GG 图形渲染引擎",
		Description: "基于 gogpu/gg 提供高性能文本与 Markdown 渲染为图片的能力，为新闻、摸鱼、词云等插件提供图文排版支持。",
		Icon:        "🌐",
		Category:    "系统工具",
		Usage: PluginUsageInfo{
			Triggers: []string{"底层系统级图形能力，供其他插件内部调用（无需用户手动输入指令）"},
			Scope:    "系统内部底层能力",
			Examples: []string{
				"提供 text.to.image 能力：将纯文本优雅折行排版渲染为高清图片",
				"提供 markdown.to.image 能力：将 Markdown 语法格式化渲染为图文长图",
			},
			Description: "Golem 的图形生成基础设施插件，利用硬件加速或纯 Go 图像处理库快速合成各类新闻长图、早报与词云背景。",
			Tips:        "无需用户手动触发，作为系统依赖能力在后台常驻运行。",
		},
	},
}

type ConfigManager struct {
	mu            sync.Mutex
	globalConfig  string
	pluginsConfig string
}

func NewConfigManager(globalConfig, pluginsConfig string) *ConfigManager {
	return &ConfigManager{
		globalConfig:  globalConfig,
		pluginsConfig: pluginsConfig,
	}
}

// PluginConfigItem 单个插件信息
type PluginConfigItem struct {
	Name        string          `json:"name"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Icon        string          `json:"icon"`
	Category    string          `json:"category"`
	Enable      bool            `json:"enable"`
	Mode        string          `json:"mode"`
	Limits      []string        `json:"limits"`
	Config      map[string]any  `json:"config"`
	RawToml     string          `json:"raw_toml,omitempty"`
	Usage       PluginUsageInfo `json:"usage"`
}

// ReadGlobalConfigStructured 读取全局配置（返回结构化对象和原始 TOML）
func (cm *ConfigManager) ReadGlobalConfigStructured() (map[string]any, string, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.globalConfig)
	if err != nil {
		return nil, "", err
	}
	var res map[string]any
	if err := toml.Unmarshal(data, &res); err != nil {
		return nil, string(data), err
	}
	return res, string(data), nil
}

// WriteGlobalConfigStructured 结构化更新全局配置
func (cm *ConfigManager) WriteGlobalConfigStructured(data map[string]any) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 读取现有配置以保留未修改字段和注释结构
	existing := make(map[string]any)
	if raw, err := os.ReadFile(cm.globalConfig); err == nil {
		_ = toml.Unmarshal(raw, &existing)
	}

	for k, v := range data {
		if subMap, ok := v.(map[string]any); ok {
			if existingSub, ok := existing[k].(map[string]any); ok {
				for subK, subV := range subMap {
					existingSub[subK] = subV
				}
				continue
			}
		}
		existing[k] = v
	}

	newData, err := toml.Marshal(existing)
	if err != nil {
		return err
	}

	dir := filepath.Dir(cm.globalConfig)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(cm.globalConfig, newData, 0644)
}

// ReadGlobalConfig 读取全局配置 raw toml 文本
func (cm *ConfigManager) ReadGlobalConfig() (string, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.globalConfig)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteGlobalConfig 校验并写入全局配置
func (cm *ConfigManager) WriteGlobalConfig(rawToml string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var dummy map[string]any
	if err := toml.Unmarshal([]byte(rawToml), &dummy); err != nil {
		return fmt.Errorf("TOML 格式不合法: %w", err)
	}

	dir := filepath.Dir(cm.globalConfig)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(cm.globalConfig, []byte(rawToml), 0644)
}

// ListPlugins 列出所有插件配置（附带中文名称与介绍）
func (cm *ConfigManager) ListPlugins() ([]PluginConfigItem, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.pluginsConfig)
	if err != nil {
		return nil, err
	}

	var root map[string]map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("解析 plugins/config.toml 失败: %w", err)
	}

	var list []PluginConfigItem
	for name, section := range root {
		meta, hasMeta := PluginMetaMap[name]
		title := name
		desc := "暂无插件描述"
		icon := "🧩"
		category := "扩展插件"
		if hasMeta {
			title = meta.Title
			desc = meta.Description
			icon = meta.Icon
			category = meta.Category
		}

		usage := meta.Usage
		if usage.Description == "" {
			usage = PluginUsageInfo{
				Triggers:    []string{name},
				Scope:       "群聊 / 私聊",
				Description: desc,
				Tips:        "可在配置参数中设置白名单/黑名单模式以及目标成员限制。",
			}
		}

		item := PluginConfigItem{
			Name:        name,
			Title:       title,
			Description: desc,
			Icon:        icon,
			Category:    category,
			Enable:      true,
			Mode:        "blacklist",
			Limits:      []string{},
			Config:      make(map[string]any),
			Usage:       usage,
		}

		if en, ok := section["enable"].(bool); ok {
			item.Enable = en
		}
		if md, ok := section["mode"].(string); ok {
			item.Mode = md
		}
		if lm, ok := section["limits"].([]any); ok {
			for _, l := range lm {
				if s, ok := l.(string); ok {
					item.Limits = append(item.Limits, s)
				}
			}
		}
		if cfg, ok := section["config"].(map[string]any); ok {
			item.Config = cfg
		}

		if secBytes, err := toml.Marshal(map[string]any{name: section}); err == nil {
			item.RawToml = string(secBytes)
		}

		list = append(list, item)
	}

	sort.Slice(list, func(i, j int) bool {
		// 置顶 ai 和 dashboard
		if list[i].Name == "ai" {
			return true
		}
		if list[j].Name == "ai" {
			return false
		}
		if list[i].Name == "dashboard" {
			return true
		}
		if list[j].Name == "dashboard" {
			return false
		}
		return list[i].Name < list[j].Name
	})

	return list, nil
}

// TogglePlugin 切换指定插件启用/停用
func (cm *ConfigManager) TogglePlugin(name string, enable bool) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.pluginsConfig)
	if err != nil {
		return err
	}

	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return err
	}

	section, ok := root[name].(map[string]any)
	if !ok {
		section = make(map[string]any)
		root[name] = section
	}
	section["enable"] = enable

	newData, err := toml.Marshal(root)
	if err != nil {
		return err
	}

	return os.WriteFile(cm.pluginsConfig, newData, 0644)
}

// CleanJSONNumbers 递归将 float64 整数转换为 int64，避免 TOML 序列化出现如 72.0
func CleanJSONNumbers(v any) any {
	switch val := v.(type) {
	case map[string]any:
		res := make(map[string]any, len(val))
		for k, item := range val {
			res[k] = CleanJSONNumbers(item)
		}
		return res
	case []any:
		res := make([]any, len(val))
		for i, item := range val {
			res[i] = CleanJSONNumbers(item)
		}
		return res
	case float64:
		if float64(int64(val)) == val {
			return int64(val)
		}
		return val
	default:
		return val
	}
}

// SavePluginSection 更新单个插件配置
func (cm *ConfigManager) SavePluginSection(name string, section map[string]any) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.pluginsConfig)
	if err != nil {
		return err
	}

	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return err
	}

	cleaned, ok := CleanJSONNumbers(section).(map[string]any)
	if !ok {
		cleaned = section
	}
	root[name] = cleaned

	newData, err := toml.Marshal(root)
	if err != nil {
		return err
	}

	return os.WriteFile(cm.pluginsConfig, newData, 0644)
}

// SavePluginRawToml 解析并更新单个插件的原始 TOML 配置
func (cm *ConfigManager) SavePluginRawToml(name string, rawToml string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var parsed map[string]any
	if err := toml.Unmarshal([]byte(rawToml), &parsed); err != nil {
		return fmt.Errorf("TOML 格式语法错误: %w", err)
	}

	var newSection map[string]any
	if sec, ok := parsed[name].(map[string]any); ok {
		newSection = sec
	} else {
		newSection = parsed
	}

	data, err := os.ReadFile(cm.pluginsConfig)
	if err != nil {
		return err
	}

	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("读取主配置文件失败: %w", err)
	}

	root[name] = newSection

	newData, err := toml.Marshal(root)
	if err != nil {
		return fmt.Errorf("序列化 TOML 失败: %w", err)
	}

	return os.WriteFile(cm.pluginsConfig, newData, 0644)
}

// SaveTargetOverrideToAIConfig 同步目标专属配置到 plugins/config.toml
func (cm *ConfigManager) SaveTargetOverrideToAIConfig(targetID string, customPrompt string, provider string, replyRate *float64, silence *bool) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.pluginsConfig)
	if err != nil {
		return err
	}

	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return err
	}

	aiSec, ok := root["ai"].(map[string]any)
	if !ok {
		aiSec = make(map[string]any)
		root["ai"] = aiSec
	}
	aiCfg, ok := aiSec["config"].(map[string]any)
	if !ok {
		aiCfg = make(map[string]any)
		aiSec["config"] = aiCfg
	}

	sessConfigs, ok := aiCfg["session_configs"].(map[string]any)
	if !ok {
		sessConfigs = make(map[string]any)
		aiCfg["session_configs"] = sessConfigs
	}

	targetSessKey := targetID
	if !strings.HasPrefix(targetSessKey, "chatroom:") && !strings.HasPrefix(targetSessKey, "contact:") {
		if strings.HasSuffix(targetSessKey, "@chatroom") {
			targetSessKey = "chatroom:" + targetSessKey
		} else {
			targetSessKey = "contact:" + targetSessKey
		}
	}

	sessItem, ok := sessConfigs[targetSessKey].(map[string]any)
	if !ok {
		sessItem = make(map[string]any)
		sessConfigs[targetSessKey] = sessItem
	}

	if customPrompt != "" {
		prompts, ok := aiCfg["prompts"].(map[string]any)
		if !ok {
			prompts = make(map[string]any)
			aiCfg["prompts"] = prompts
		}
		promptKey := "target_" + strings.ReplaceAll(strings.ReplaceAll(targetID, "@", "_"), ":", "_")
		prompts[promptKey] = customPrompt
		sessItem["active_prompt"] = promptKey
	}

	if provider != "" {
		sessItem["active_provider"] = provider
	}
	if replyRate != nil {
		sessItem["reply_rate"] = *replyRate
	}
	if silence != nil {
		sessItem["silence"] = *silence
	}

	newData, err := toml.Marshal(root)
	if err != nil {
		return err
	}

	return os.WriteFile(cm.pluginsConfig, newData, 0644)
}

// RemoveTargetOverrideFromAIConfig 从 plugins/config.toml 中移除目标专属配置
func (cm *ConfigManager) RemoveTargetOverrideFromAIConfig(targetID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.pluginsConfig)
	if err != nil {
		return err
	}

	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return err
	}

	aiSec, ok := root["ai"].(map[string]any)
	if !ok {
		return nil
	}
	aiCfg, ok := aiSec["config"].(map[string]any)
	if !ok {
		return nil
	}
	sessConfigs, ok := aiCfg["session_configs"].(map[string]any)
	if !ok {
		return nil
	}

	targetSessKey := targetID
	if !strings.HasPrefix(targetSessKey, "chatroom:") && !strings.HasPrefix(targetSessKey, "contact:") {
		if strings.HasSuffix(targetSessKey, "@chatroom") {
			targetSessKey = "chatroom:" + targetSessKey
		} else {
			targetSessKey = "contact:" + targetSessKey
		}
	}

	delete(sessConfigs, targetSessKey)

	newData, err := toml.Marshal(root)
	if err != nil {
		return err
	}

	return os.WriteFile(cm.pluginsConfig, newData, 0644)
}
