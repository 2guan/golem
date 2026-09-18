package main

import "fmt"

// SceneType 场景枚举
type SceneType string

const (
	SceneGym  SceneType = "gym"  // 铁馆更衣室
	SceneBar  SceneType = "bar"  // 微醺小酒馆
	SceneDorm SceneType = "dorm" // 男生宿舍熄灯后
)

// SceneInfo 场景元数据
type SceneInfo struct {
	Type      SceneType
	Title     string
	Icon      string
	Tagline   string
	IntroText string
}

// AllScenes 全局场景映射
var AllScenes = map[SceneType]SceneInfo{
	SceneGym: {
		Type:    SceneGym,
		Title:   "铁馆更衣室",
		Icon:    "🏋️‍♂️",
		Tagline: "热气、铁器与汗水蒸腾的纯雄性重力场",
		IntroText: "（正扯着肩上的白毛巾擦脖子上的热汗，抬头看见你走过来，挑了挑眉笑了）\n" +
			"哟，来啦？今天打算练哪个部位？\n" +
			"先说好啊，卧推可别死撑，我全程在后面贴身给你托着底呢。先把毛巾挂好，过来，咱先热个身。",
	},
	SceneBar: {
		Type:    SceneBar,
		Title:   "微醺小酒馆",
		Icon:    "🍸",
		Tagline: "昏暗暖光与低音萨克斯风中的深夜避风港",
		IntroText: "（把深色衬衫袖口往上挽了挽，大块老冰在古典威士忌杯里撞得清脆作响，撑在吧台上打量着你笑了笑）\n" +
			"坐吧。今晚想喝点什么？烈一点的，还是能让人彻底卸下防备的？\n" +
			"第一杯算我的，跟我说说你今晚的心事，这儿没别人。",
	},
	SceneDorm: {
		Type:    SceneDorm,
		Title:   "男生宿舍熄灯后",
		Icon:    "🎽",
		Tagline: "夜深断电后，狭窄单人床与被窝间的少年荷尔蒙",
		IntroText: "（刚冲完凉，随意套着件宽松大号球衣坐在床沿擦头发，见你躺在对铺翻来覆去，拍了拍身旁的空位）\n" +
			"电闸跳了，整栋楼都黑了。\n" +
			"你怎么还不睡？认床还是嫌冷？\n" +
			"要不你过来跟我挤挤，反正这床够大，哥给你暖被窝。",
	},
}

// FormatSceneCard 生成进入场景时的欢迎面板
func FormatSceneCard(scene SceneInfo, heartRate int, tension int, extraStatus string) string {
	status := fmt.Sprintf("❤️ 心率 %d bpm | 🔥 暧昧度 %d%%", heartRate, tension)
	return fmt.Sprintf("%s【%s】\n\n"+
		"%s\n\n"+
		"📊【身体指标】：%s\n"+
		"（直接跟我对话聊天或打字互动，随时发【离开】退出）",
		scene.Icon, scene.Title,
		scene.IntroText,
		status,
	)
}
