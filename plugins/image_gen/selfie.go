package main

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

var defaultScenicScenes = []string{
	"故宫角楼护城河边，微风吹拂，蓝天白云，阳光明媚，真实生活感手机自拍",
	"什刹海后海湖畔夕阳西下，金光洒在水面，随性自然生活手机前置自拍",
	"景山公园万春亭俯瞰紫禁城全景，阳光晴朗，真实生活感手机自拍",
	"奥林匹克森林公园秋天银杏树下，金黄落叶铺地，温暖阳光斜照手机自拍",
	"望京SOHO夜景流光溢彩，现代都市科技感背景，真实生活感随手拍",
	"三里屯露天咖啡馆木桌前，手边放着一杯美式咖啡，午后阳光惬意，休闲生活自拍照",
	"健身房力量训练区，背景杠铃哑铃铁馆器械，运动后微汗微喘，结实肌肉感自拍照",
	"老北京胡同青砖灰瓦大槐树下，斑驳树影，生活气息浓厚，随性真实自拍",
	"颐和园十七孔桥畔，湖面波光粼粼，远山如黛，晴朗冬日生活自拍",
	"亮马河国际风情水岸夜景，河畔灯光璀璨，夜风惬意，微醺休闲生活自拍",
	"798艺术区红砖包豪斯厂房前，阳光透过绿叶，文艺悠闲周末生活自拍照",
	"北海公园白塔湖畔，湖面泛着微波，绿柳依依，初夏清凉生活自拍",
	"国贸CBD繁华十字路口天桥上，车水马龙，都市夕阳晚霞生活自拍",
	"北京地坛公园幽静林荫小道，光斑洒在肩头，初夏葱郁自然生活自拍",
	"首钢园三高炉与大跳台工业遗址公园，钢铁工业硬朗背景，阳光写实自拍",
	"朝阳公园大草坪与湖畔野餐区，蓝天绿草，周末户外露营休闲写实自拍",
	"家中书房码农升降桌前，双显示器亮着代码编辑器，随性格子衬衫，真实居家生活自拍",
	"前门大栅栏老街牌楼前，熙熙攘攘烟火气，随手写实自拍照",
	"香山脚下红叶漫山，秋高气爽，登山休闲装扮，自然写实手机自拍",
	"陶然亭公园湖畔长椅，微风拂柳，安静闲适生活自拍照",
}

// BuildSelfiePrompt 根据给定的场景（或随机挑选的风景场景）构造自拍提示词
func (g *ImageGenerator) BuildSelfiePrompt(customScene string) (string, string) {
	scene := strings.TrimSpace(customScene)
	if scene == "" {
		pool := g.cfg.ScenicScenes
		if len(pool) == 0 {
			pool = defaultScenicScenes
		}
		scene = pool[rand.IntN(len(pool))]
	}

	prompt := fmt.Sprintf("参考图中的微胖成熟男性自拍照，保持人物五官相貌特征与微胖体型，背景在%s，真实自然手机前置镜头自拍视角，无美颜滤镜，真实生活自然光影，高清晰度写实摄影", scene)
	return prompt, scene
}

// GenerateSelfie 生成肉丸的自拍照片
func (g *ImageGenerator) GenerateSelfie(customScene string, avatarURL string) ([]byte, string, string, error) {
	prompt, chosenScene := g.BuildSelfiePrompt(customScene)

	refImage := strings.TrimSpace(avatarURL)
	if refImage == "" {
		refImage = strings.TrimSpace(g.cfg.DefaultAvatarURL)
	}

	data, mime, err := g.GenerateImage(prompt, refImage)
	if err != nil {
		return nil, "", chosenScene, err
	}
	return data, mime, chosenScene, nil
}
