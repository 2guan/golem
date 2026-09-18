package main

type MealTime string

const (
	MealBreakfast MealTime = "breakfast"
	MealLunch     MealTime = "lunch"
	MealTea       MealTime = "tea"
	MealDinner    MealTime = "dinner"
	MealSupper    MealTime = "supper"
)

type Dish struct {
	Name        string
	Category    string
	MealTimes   []MealTime
	Tags        []string
	Calories    string
	Description string
	MeatballTip string
}

var defaultMenu = []Dish{
	// ================= 早餐 / 早午餐 =================
	{
		Name:        "老北京烧饼夹牛肉 + 羊杂汤",
		Category:    "汤品面点",
		MealTimes:   []MealTime{MealBreakfast, MealLunch},
		Tags:        []string{"清真", "暖胃", "碳水", "肉食", "京味"},
		Calories:    "⭐⭐⭐⭐ (扎实抗饿，浑身发热)",
		Description: "刚出炉酥得掉渣的芝麻烧饼，夹上卤得酱香浓郁的腱子肉，配上一大碗白汤滚烫的羊杂。",
		MeatballTip: "肉丸秘籍：香菜必须抓两大把，泼上半勺红油辣椒，先吸两口原汤，再把烧饼在汤里稍微蘸一下，绝了！",
	},
	{
		Name:        "双蛋煎饼果子 + 现磨豆浆",
		Category:    "街头快餐",
		MealTimes:   []MealTime{MealBreakfast, MealLunch},
		Tags:        []string{"快餐", "便宜", "碳水", "经典"},
		Calories:    "⭐⭐⭐ (碳水加倍，快乐加倍)",
		Description: "绿豆面摊得又薄又脆，磕俩笨鸡蛋，抹上甜面酱与红油辣酱，裹上酥脆焦香的薄脆果箅儿。",
		MeatballTip: "肉丸秘籍：千万别加什么火腿肠生菜乱七八糟的，就双蛋加双份薄脆！听那一口咔嚓声，比赢了团战还解压。",
	},
	{
		Name:        "皮蛋瘦肉粥 + 广式虾饺皇",
		Category:    "广式茶点",
		MealTimes:   []MealTime{MealBreakfast, MealLunch, MealDinner},
		Tags:        []string{"清淡", "养胃", "鲜美"},
		Calories:    "⭐⭐ (温润清爽，负罪感极低)",
		Description: "大米熬得开花融糯，皮蛋碎与鲜嫩姜丝瘦肉丝温润交融，虾饺晶莹剔透，里头裹着整颗大虾仁。",
		MeatballTip: "肉丸秘籍：宿醉或昨晚夜宵吃油腻了？来这碗热粥刮刮油，配两滴白胡椒粉，胃瞬间舒服了。",
	},
	{
		Name:        "武汉热干面 + 冰米酒蛋花汤",
		Category:    "面食",
		MealTimes:   []MealTime{MealBreakfast, MealLunch},
		Tags:        []string{"面食", "碳水", "重口", "便宜"},
		Calories:    "⭐⭐⭐⭐ (浓郁芝麻酱暴击)",
		Description: "碱水面弹牙筋道，淋满醇香稠密的黑芝麻酱、香油与秘制卤水，撒上酸豆角和辣萝卜丁。",
		MeatballTip: "肉丸秘籍：趁热八秒内必须拌匀！每一根面条都要裹满酱，吃完再灌一大口冰蛋酒，甘甜清凉解大腻。",
	},

	// ================= 午餐 / 工作日快餐 =================
	{
		Name:        "兰州牛肉面（二细加肉加蛋）",
		Category:    "面食",
		MealTimes:   []MealTime{MealLunch, MealDinner},
		Tags:        []string{"面食", "快餐", "清真", "暖胃", "便宜"},
		Calories:    "⭐⭐⭐ (高蛋白适量碳水)",
		Description: "一清二白三红四绿五黄，牛骨清汤醇厚见底，白萝卜软烂入味，蒜苗香菜翠绿欲滴。",
		MeatballTip: "肉丸秘籍：跟师傅喊‘二细，辣子多些！’，点一份切片牛肉直接倒进汤里泡着，就着剥皮生蒜，这顿午饭才算有灵魂。",
	},
	{
		Name:        "老北京小碗干炸炸酱面",
		Category:    "面食",
		MealTimes:   []MealTime{MealLunch, MealDinner},
		Tags:        []string{"面食", "京味", "家常", "碳水"},
		Calories:    "⭐⭐⭐⭐ (酱香四溢，碳水天堂)",
		Description: "五花肉丁煸到吐油微焦，六必居黄酱与甜面酱慢火干炸出明油，配上手擀过水细面与七八样脆嫩菜码。",
		MeatballTip: "肉丸秘籍：别一上来全倒进去！先挖两勺酱，黄瓜丝心儿里美豆芽拌匀，就一口生紫皮蒜，肉丸从小吃到大吃不腻。",
	},
	{
		Name:        "隆江猪脚饭（加卤蛋肉卷）",
		Category:    "盖饭便当",
		MealTimes:   []MealTime{MealLunch, MealDinner},
		Tags:        []string{"肉食", "快餐", "下饭", "满足"},
		Calories:    "⭐⭐⭐⭐⭐ (男人的浪漫，胶原蛋白狂欢)",
		Description: "卤到红亮诱人、软烂弹糯的四点金和蹄膀肉，剁碎浇上一勺粘嘴唇的老卤汤，搭配清爽解腻酸菜。",
		MeatballTip: "肉丸秘籍：老板要是手抖少给浇卤汁，记得厚着脸皮多要一勺！把酸菜和米饭搅拌在一起，肥瘦相间一口闷，人间值得。",
	},
	{
		Name:        "川香回锅肉盖浇饭",
		Category:    "盖饭便当",
		MealTimes:   []MealTime{MealLunch, MealDinner},
		Tags:        []string{"川菜", "微辣", "下饭", "肉食"},
		Calories:    "⭐⭐⭐⭐ (米饭杀手，油香扑鼻)",
		Description: "煮透的三层五花肉切薄片，热锅下豆瓣酱与豆豉煸出灯盏窝，配上青蒜苗大火爆炒，镬气十足。",
		MeatballTip: "肉丸秘籍：青蒜苗炒到微微断生带甜味是精髓，肥肉部分的油全煸出来了，卷着蒜苗能扒下两碗大米饭。",
	},
	{
		Name:        "老西关烧腊双拼饭（烧鹅拼蜜汁叉烧）",
		Category:    "粤菜快餐",
		MealTimes:   []MealTime{MealLunch, MealDinner},
		Tags:        []string{"粤菜", "鲜甜", "肉食", "快餐"},
		Calories:    "⭐⭐⭐⭐ (皮脆肉嫩，蜜汁诱惑)",
		Description: "烧鹅皮脆脂香，咬下去汁水迸发；叉烧边缘微焦带蜜汁回甘，配上一颗溏心荷包蛋与两根菜心。",
		MeatballTip: "肉丸秘籍：酸梅酱多蘸点！鹅油渗进热米饭里之后，每一口米饭都香得让人跺脚。",
	},
	{
		Name:        "新疆重辣炒米粉 + 烤包子",
		Category:    "面食小吃",
		MealTimes:   []MealTime{MealLunch, MealDinner},
		Tags:        []string{"重口", "爆辣", "过瘾", "面食"},
		Calories:    "⭐⭐⭐⭐ (辣出眼泪，极度爽快)",
		Description: "粗粉嚼劲十足，裹着厚重香浓的红油辣酱，芹菜碎爽脆解腻，牛肉片大块实在，烤包子酥皮爆汁。",
		MeatballTip: "肉丸秘籍：不能吃辣的自觉点微辣！吃完赶紧备一盒冰酸奶，否则下午打游戏键盘上全是汗珠子。",
	},
	{
		Name:        "重庆豌杂小面（干溜干拌）",
		Category:    "面食",
		MealTimes:   []MealTime{MealLunch, MealDinner},
		Tags:        []string{"面食", "麻辣", "重口", "便宜"},
		Calories:    "⭐⭐⭐ (沙软豌豆加麻辣肉酱)",
		Description: "煮到沙软起沙的大白豌豆，覆盖在喷香的杂酱肉臊子上，碗底埋着花椒油、保宁醋与油辣子。",
		MeatballTip: "肉丸秘籍：必须选干溜！不要汤！拌匀后让每根细面都裹上沙沙的烂豌豆泥，麻辣鲜香直冲天灵盖。",
	},
	{
		Name:        "黄焖鸡米饭（微辣加金针菇厚豆皮）",
		Category:    "盖饭便当",
		MealTimes:   []MealTime{MealLunch, MealDinner},
		Tags:        []string{"快餐", "家常", "下饭", "实惠"},
		Calories:    "⭐⭐⭐ (打工人标配安心粮)",
		Description: "砂锅文火焖炖的鲜嫩三黄鸡腿肉，浓郁咸鲜的汤汁里浸透了软烂的土豆块、脆嫩的青椒与香菇。",
		MeatballTip: "肉丸秘籍：把金针菇和吸饱汤汁的土豆块捣碎在白米饭里，连汤带肉一勺舀下去，性价比之王不是吹的。",
	},

	// ================= 减脂 / 清淡 / 养生 =================
	{
		Name:        "三文鱼牛油果经典波奇碗",
		Category:    "轻食减脂",
		MealTimes:   []MealTime{MealLunch, MealDinner},
		Tags:        []string{"减脂", "清淡", "高蛋白", "健康", "低卡"},
		Calories:    "⭐ (自律纯净，优质油脂)",
		Description: "新鲜三文鱼刺身切方丁，搭配熟透软糯的牛油果、甜玉米、海苔碎、水煮毛豆仁与黑米杂粮底。",
		MeatballTip: "肉丸秘籍：别选热量炸弹蛋黄酱！淋半勺日式油醋汁或低钠酱油芥末，健康又不委屈嘴巴。",
	},
	{
		Name:        "酸汤金汤巴沙鱼 + 粗粮饭",
		Category:    "家常快餐",
		MealTimes:   []MealTime{MealLunch, MealDinner},
		Tags:        []string{"减脂", "酸辣", "高蛋白", "开胃"},
		Calories:    "⭐⭐ (开胃不长肉)",
		Description: "鲜嫩无刺的巴沙鱼片滑嫩入味，黄灯笼辣椒与金酸汤底酸辣开胃，铺底配有金针菇与爽脆豆芽。",
		MeatballTip: "肉丸秘籍：鱼肉纯蛋白质几乎没脂肪，酸辣汤汁特别提神醒脑，下午不容易犯困！",
	},
	{
		Name:        "全麦金枪鱼三明治 + 冰美式",
		Category:    "轻食减脂",
		MealTimes:   []MealTime{MealBreakfast, MealLunch, MealTea},
		Tags:        []string{"减脂", "快餐", "低卡", "提神"},
		Calories:    "⭐ (精英干练，消肿利尿)",
		Description: "烘烤至酥脆的全麦吐司，夹着水浸金枪鱼、水煮荷包蛋、生菜番茄片，搭配无糖纯黑咖啡。",
		MeatballTip: "肉丸秘籍：赶时间或者想控糖减肥时首选，冰美式一大口下去，水肿全消，战斗力拉满。",
	},

	// ================= 摸鱼下午茶 / 续命水 =================
	{
		Name:        "鸭屎香手打柠檬茶（少糖少冰）",
		Category:    "下午茶饮",
		MealTimes:   []MealTime{MealTea},
		Tags:        []string{"饮品", "清爽", "解渴", "解腻"},
		Calories:    "⭐ (刮油提神)",
		Description: "香水柠檬重度捣压出浓郁果皮精油香，融合凤凰单丛鸭屎香乌龙茶汤，冰爽清冽回甘悠长。",
		MeatballTip: "肉丸秘籍：下午三四点脑子发懵、嘴巴发干？整一杯少糖大冰，猛吸一口直通脑门，精神抖擞。",
	},
	{
		Name:        "生椰拿铁 + 海盐芝士贝果",
		Category:    "下午茶饮",
		MealTimes:   []MealTime{MealTea},
		Tags:        []string{"咖啡", "面包", "下午茶", "打工人"},
		Calories:    "⭐⭐⭐ (幸福感拉满的碳水充电)",
		Description: "冷萃厚椰乳碰撞浓缩咖啡油脂，椰香醇厚甘甜；烤热的贝果抹上厚厚一层微咸海盐芝士酪。",
		MeatballTip: "肉丸秘籍：摸鱼三点半的黄金搭档！贝果一定要让店员加热烤脆皮，咬一口外脆里韧嚼劲十足。",
	},

	// ================= 晚餐 / 聚餐 / 锅物 =================
	{
		Name:        "潮汕鲜牛肉火锅（吊龙 + 匙柄 + 熟牛肉丸）",
		Category:    "火锅聚餐",
		MealTimes:   []MealTime{MealDinner},
		Tags:        []string{"火锅", "聚餐", "牛肉", "高蛋白", "清淡"},
		Calories:    "⭐⭐⭐ (纯牛肉高蛋白，只要不喝麻酱就不慌)",
		Description: "牛骨清汤沸水翻滚，鲜切吊龙肉伴着细微雪花，八上八下九秒即捞，搭配手打爆汁牛肉丸与炸腐竹。",
		MeatballTip: "肉丸秘籍：沙茶酱调一勺，兑半勺芹菜粒和普宁豆酱！咬那手打牛肉丸小心点，真会滋一身肉汁。",
	},
	{
		Name:        "铜锅涮羊肉（手切热温羊上脑 + 糖蒜芝麻酱）",
		Category:    "火锅聚餐",
		MealTimes:   []MealTime{MealDinner},
		Tags:        []string{"火锅", "京味", "羊肉", "聚餐", "经典"},
		Calories:    "⭐⭐⭐⭐ (老北京地道聚会王牌)",
		Description: "景泰蓝炭火铜锅咕嘟冒泡，清水下葱姜大枣，手切鲜羊肉倒盘不洒，蘸上二八酱、腐乳汁、韭菜花调制的碗底。",
		MeatballTip: "肉丸秘籍：糖蒜必须剥三瓣搁手边！羊肉变白立刻捞，蘸透麻酱，嚼一口肉嚼一口糖蒜，神仙都不换。",
	},
	{
		Name:        "川味九宫格牛油火锅（千层肚 + 鲜鸭血 + 贡菜）",
		Category:    "火锅聚餐",
		MealTimes:   []MealTime{MealDinner, MealSupper},
		Tags:        []string{"火锅", "麻辣", "过瘾", "聚餐", "重口"},
		Calories:    "⭐⭐⭐⭐⭐ (牛油浸透，罪恶与快感齐飞)",
		Description: "醇厚老牛油融化在九宫格内，干辣椒与大红袍花椒肆意翻腾，七上八下的鲜毛肚与久煮不散的果冻鲜鸭血。",
		MeatballTip: "肉丸秘籍：香油蒜泥碟是保命护胃的，千万别加麻酱！毛肚下中心沸水格子，数七下脆生生起锅，过香油一口吞！",
	},
	{
		Name:        "东北铁锅炖大鹅（贴玉米面饼子 + 宽粉豆角）",
		Category:    "热锅大菜",
		MealTimes:   []MealTime{MealDinner},
		Tags:        []string{"聚餐", "大菜", "家常", "热乎"},
		Calories:    "⭐⭐⭐⭐ (量大实在，围炉温暖)",
		Description: "柴火大灶铁锅现炖，肥美笨大鹅酱香软烂，油豆角吸透鹅油，锅沿上贴满焦黄酥脆的现烤玉米饼。",
		MeatballTip: "肉丸秘籍：锅底那把吸饱鹅油高汤的东北土豆宽粉才是真神！铲下一块焦脆锅巴饼蘸着浓汁吃，香迷糊了。",
	},
	{
		Name:        "湘味小炒黄牛肉 + 擂辣椒皮蛋茄子",
		Category:    "湘菜",
		MealTimes:   []MealTime{MealLunch, MealDinner},
		Tags:        []string{"湘菜", "爆辣", "下饭", "肉食"},
		Calories:    "⭐⭐⭐ (辣香下饭，连炫三碗)",
		Description: "大火猛炒的嫩牛肉片带着野山椒与小米辣的炽热，擂钵里青椒皮蛋茄子被木槌捣出灵魂浓汁。",
		MeatballTip: "肉丸秘籍：把擂辣椒皮蛋直接扣在大米饭正中央，挖一勺牛肉连油带肉拌开，辣得胃里暖烘烘，米饭根本不够吃。",
	},

	// ================= 罪恶夜宵 / 疗愈酒肆 =================
	{
		Name:        "炭火烤羊肉串 + 烤脑花 + 冰镇精酿啤酒",
		Category:    "烧烤夜宵",
		MealTimes:   []MealTime{MealDinner, MealSupper},
		Tags:        []string{"夜宵", "烧烤", "微醺", "肉食", "罪恶"},
		Calories:    "⭐⭐⭐⭐⭐ (今晚不提卡路里，开心最重要)",
		Description: "肥瘦相间的羊肉串在炭火上滋滋冒油，孜然辣椒面撒得厚厚一层；锡纸盒里的烤脑花浸在红油大头菜里，嫩如豆腐。",
		MeatballTip: "肉丸秘籍：脑花一定要趁热拿勺子挖，入口即化！再闷一大口冰凉挂壁的精酿，什么烦心事儿都去他大爷的。",
	},
	{
		Name:        "柳州螺蛳粉（加炸蛋 + 卤虎皮鸭掌）",
		Category:    "夜宵粉面",
		MealTimes:   []MealTime{MealLunch, MealDinner, MealSupper},
		Tags:        []string{"夜宵", "酸辣", "重口", "上头"},
		Calories:    "⭐⭐⭐⭐ (臭香扑鼻，深夜灵魂补给)",
		Description: "螺蛳熬骨浓汤鲜辣酸爽，酸笋木耳花生腐竹铺满，蓬松多孔的巨大炸蛋吸饱了浓郁红油汤汁。",
		MeatballTip: "肉丸秘籍：炸蛋必须在汤底最深处浸泡三分钟！捞出来沉甸甸咬开爆汁，鸭掌一嗦脱骨，深夜吃这个简直犯罪。",
	},
	{
		Name:        "麻辣小龙虾（十三香 / 油焖）+ 拌热方便面",
		Category:    "夜宵霸主",
		MealTimes:   []MealTime{MealDinner, MealSupper},
		Tags:        []string{"夜宵", "海鲜", "聚会", "下酒", "麻辣"},
		Calories:    "⭐⭐⭐⭐ (剥虾手停不下来)",
		Description: "红亮油润的小龙虾个大黄满，蒜蓉与十三香酱汁深层渗透，虾肉紧实Q弹。",
		MeatballTip: "肉丸秘籍：虾吃完了千万别让服务员收盘子！扔两饼泡软的华丰三鲜伊面倒进汤里拌匀，那一碗面比龙虾肉还抢手。",
	},
	{
		Name:        "校门口烤冷面（双蛋双肠多放糖醋洋葱）",
		Category:    "街头小吃",
		MealTimes:   []MealTime{MealTea, MealSupper},
		Tags:        []string{"街头", "小吃", "便宜", "酸甜"},
		Calories:    "⭐⭐⭐ (学生时代的快乐喷泉)",
		Description: "铁板压制冷面皮，刷满金黄蛋液，挤上酸甜适口的番茄辣酱，卷入焦香淀粉肠、脆生生洋葱碎与香菜。",
		MeatballTip: "肉丸秘籍：跟大爷喊‘多放醋少放辣，陈醋顺着边浇出焦香味！’，热乎乎扎上一块，瞬间梦回大学宿舍通宵打游戏的时光。",
	},
}
