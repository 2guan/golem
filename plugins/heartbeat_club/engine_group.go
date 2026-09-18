package main

import (
	"fmt"
	"math/rand/v2"
)

// HandleArmWrestle 处理群内【掰手腕 / 比拼身材】
func HandleArmWrestle(userA, userB string) string {
	if userB == "" || userA == userB {
		userB = "肉丸"
	}

	powerA := rand.IntN(40) + 60 // 60-99
	powerB := rand.IntN(40) + 60

	winner := userA
	loser := userB
	if powerB > powerA {
		winner = userB
		loser = userA
	}

	scenes := []string{
		"两人的手肘重重砸在实木桌面上，十指紧扣，手臂青筋瞬间暴突！近距离的粗重呼吸喷薄在彼此锁骨上，荷尔蒙在空气中噼啪作响！",
		"铁馆的器械在背景轰鸣，两人胸膛剧烈起伏，衣服下摆随着发力被汗水紧紧贴在腹肌上，眼神中满是互不相让的野性与较量！",
		"宿舍的窄桌几乎被压得嘎吱作响，两人双膝顶在一起，浑身的每一块肌肉都在剧烈紧绷充血，甚至能感受到对方手心滚烫的温度！",
	}
	sceneDesc := scenes[rand.IntN(len(scenes))]

	penalties := []string{
		fmt.Sprintf("【更衣室惩罚】：【%s】必须在群里向【%s】双手奉上一瓶运动饮料，并夸赞对方一句“体魄惊人”！", loser, winner),
		fmt.Sprintf("【荷尔蒙惩罚】：【%s】需在群内对【%s】发出一句低音情话，限时3分钟内不可撤回！", loser, winner),
		fmt.Sprintf("【宿管惩罚】：【%s】今晚负责把两人的臭球衣全包洗掉！", loser),
		fmt.Sprintf("【微醺惩罚】：今晚酒吧的下一轮特调，由【%s】全额买单！", loser),
	}
	penalty := penalties[rand.IntN(len(penalties))]

	reviews := []string{
		"肉丸在旁一边喝着蛋白粉一边打量：“好家伙，这两副身板较起劲来，连我都想上去加一组大重量了！”",
		"肉丸挑了挑眉：“这力量感真绝了，青筋暴起的样子够man，胜负就在毫厘之间啊！”",
		"肉丸抱臂笑道：“别死撑了，手心都出汗了吧？赶紧分出胜负，待会儿一起冲凉去！”",
	}
	review := reviews[rand.IntN(len(reviews))]

	return fmt.Sprintf("🏋️‍♂️【铁馆重力场 · 力量与荷尔蒙对决】\n\n"+
		"⚔️ 对决双方：【%s】VS【%s】\n"+
		"💥 战况实录：\n%s\n\n"+
		"📊 力量输出检测：\n"+
		"• %s 荷尔蒙战力：%d 点\n"+
		"• %s 荷尔蒙战力：%d 点\n\n"+
		"🏆 最终胜者：👑【%s】以绝对力量制霸全场！\n"+
		"⚠️ 败者大冒险：%s\n\n"+
		"🎙️ 肉丸老爹锐评：\n%s",
		userA, userB, sceneDesc, userA, powerA, userB, powerB, winner, penalty, review)
}

// HandleBarDare 处理【微醺大冒险 / 酒吧摇骰】
func HandleBarDare(senderName string) string {
	dice := rand.IntN(6) + 1

	cocktails := []string{
		"🍸【今夜耳语】：泥煤威士忌 + 烟熏苦精，烈度极高，适合酒后吐真言。",
		"🍹【热带荷尔蒙】：百加得黑朗姆 + 碎冰青柠，酸甜冰爽，点燃躁动夜色。",
		"🥃【孤胆老将】：纯饮波本威士忌，带着橡木桶的陈年烟草香，回味深沉霸道。",
		"🥂【天菜微醺特调】：金酒 + 迷迭香起泡酒，香气扑鼻，让人悄悄脸红心跳。",
	}
	drink := cocktails[rand.IntN(len(cocktails))]

	dares := []string{
		"选一位群内在线的群友，用最磁性的气音向他发送一句：“今晚别走，陪我喝完这杯。”",
		"坦白局：在群里坦白你最无法抗拒男人的哪一个瞬间（如：穿白背心、喉结滚动、手臂青筋暴起）。",
		"大冒险：发送一张你相册里最帅、最具荷尔蒙张力（哪怕是肌肉线条或侧脸）的私藏图/表情包！",
		"挑选一位群友，隔空对视3秒，并评价他的身材或气质最像哪种烈酒！",
		"向肉丸发送一句微醺试探情话，看看能不能把这位老江湖撩到脸红心跳！",
	}
	dare := dares[rand.IntN(len(dares))]

	return fmt.Sprintf("🍸【微醺小酒馆 · 深夜摇骰大冒险】\n\n"+
		"👤 摇骰客人：【%s】\n"+
		"🎲 骰子点数：%d 点（微醺度爆表！）\n\n"+
		"🍷 今夜特供特调：\n%s\n\n"+
		"🔥 心跳大冒险挑战：\n%s\n\n"+
		"🎙️ 调酒师肉丸寄语：\n“酒杯一碰，心事落地。既然骰子摇出来了，可别想耍赖蒙混过关啊！”",
		senderName, dice, drink, dare)
}

// HandleDormInspection 处理【宿管查寝】
func HandleDormInspection(senderName string) string {
	events := []string{
		"突袭检查！1号床与2号床的被窝挤成一团，两人声称‘被子太薄在互相取暖’，现场查获大号球衣两件！",
		"灯光一照！床底下发现藏着两只高阻力握力器和一瓶喝了一半的冰镇乌龙茶，旁边散落着两件湿透的工字背心！",
		"阳台突击！发现两位舍友正就着月光暗戳戳比拼引体向上，浑身肌肉线条清晰，当场被抓包破坏就寝纪律！",
		"查寝通报！宿舍四人正在上床下桌十指紧扣较量掰手腕，床架晃动被隔壁敲墙抗议，现场荷尔蒙严重超标！",
	}
	event := events[rand.IntN(len(events))]

	scores := rand.IntN(30) + 70 // 70-99
	return fmt.Sprintf("🎽【体校男生宿舍 · 深夜突击查寝通报】\n\n"+
		"🚨 巡查干事：【肉丸】\n"+
		"🏠 重点核查寝室：【%s 与他的好室友们】\n\n"+
		"📋 现场抓包纪要：\n%s\n\n"+
		"📈 本寝荷尔蒙超标指数：%d / 100\n"+
		"🏆 处罚与奖赏：\n“看在你们感情这么深厚的份上，罚今晚继续合盖一床被子，谁也不准偷偷回自己床上！”",
		senderName, event, scores)
}

// HandleCompatibility 处理【契合度 @某人】
func HandleCompatibility(userA, userB string) string {
	if userB == "" || userA == userB {
		userB = "肉丸"
	}

	score := rand.IntN(45) + 55 // 55 - 99%
	tempMatch := rand.IntN(30) + 70
	electric := rand.IntN(30) + 70
	jersey := rand.IntN(30) + 70

	verdict := "微热互探，眼神已经快拉丝了！"
	if score >= 90 {
		verdict = "🔥 灵魂共震！荷尔蒙天作之合，赶紧原地领证锁死！"
	} else if score >= 80 {
		verdict = "💓 强张力羁绊！铁馆能互保大重量、被窝能挤同一张床的双倍快乐！"
	} else if score >= 70 {
		verdict = "🍸 微醺试探期！就差一杯烈酒和一句深夜耳语就能点燃火花！"
	}

	return fmt.Sprintf("💘【荷尔蒙雷达 · 双人性张力契合度测算】\n\n"+
		"👥 测算对象：【%s】×【%s】\n"+
		"💯 综合荷尔蒙契合指数：%d%%\n"+
		"🏷️ 羁绊评级：%s\n\n"+
		"📊 细节三维维度：\n"+
		"• 🌡️ 体温共鸣度：%d%%\n"+
		"• ⚡ 眼神带电率：%d%%\n"+
		"• 🎽 湿水球衣互穿率：%d%%\n\n"+
		"🎙️ 肉丸老将撮合点评：\n“这两位站一块儿，光是气场碰撞就够写三万字心动大戏了。别端着了，赶快约个时间铁馆开练或者酒馆碰一杯！”",
		userA, userB, score, verdict, tempMatch, electric, jersey)
}
