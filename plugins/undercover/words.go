package main

import "math/rand/v2"

// WordPair 词语对
type WordPair struct {
	Civilian   string // 平民词
	Undercover string // 卧底词
}

var defaultWordPairs = []WordPair{
	{"状元", "冠军"},
	{"眉毛", "胡子"},
	{"降落伞", "热气球"},
	{"玫瑰", "月季"},
	{"汉堡包", "肉夹馍"},
	{"班主任", "辅导员"},
	{"牛奶", "豆浆"},
	{"微信", "QQ"},
	{"辣椒", "芥末"},
	{"游泳", "潜水"},
	{"风扇", "空调"},
	{"口红", "唇膏"},
	{"自行车", "电动车"},
	{"小笼包", "灌汤包"},
	{"魔术师", "魔法师"},
	{"橙子", "橘子"},
	{"泡泡糖", "棒棒糖"},
	{"吉他", "尤克里里"},
	{"饼干", "曲奇"},
	{"包子", "馒头"},
	{"大白兔", "阿尔卑斯"},
	{"小黄人", "哆啦A梦"},
	{"秋裤", "毛裤"},
	{"麻辣烫", "串串香"},
	{"雨衣", "雨伞"},
	{"眉笔", "眼线笔"},
	{"纸巾", "湿巾"},
	{"水盆", "水桶"},
	{"蝴蝶", "蜜蜂"},
	{"洗发水", "沐浴露"},
}

// RandomWordPair 随机抽取一组词（随机决定哪边当卧底）
func RandomWordPair() (civilian string, undercover string) {
	pair := defaultWordPairs[rand.IntN(len(defaultWordPairs))]
	if rand.IntN(2) == 0 {
		return pair.Civilian, pair.Undercover
	}
	return pair.Undercover, pair.Civilian
}
