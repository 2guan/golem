package contactability

import (
	"strings"

	"github.com/duke-git/lancet/v2/maputil"
	"github.com/duke-git/lancet/v2/slice"
	contactapi "github.com/sbgayhub/golem/host/api/contact"
	"github.com/sbgayhub/golem/sdk/contact"
)

var def contact.Contact

type strategy interface {
	find(cache map[string]*contact.Contact) *contact.Contact
}

func getStrategy(data string) strategy {
	s, key := parser(data)
	switch s {
	case "nickname":
		return nicknameStrategy{key}
	case "remark":
		return remarkStrategy{key}
	default:
		return usernameStrategy{key}
	}
}

func parser(data string) (string, string) {
	ss := strings.Split(data, "::")
	if len(ss) == 2 {
		return ss[0], ss[1]
	}
	return "username", data
}

type usernameStrategy struct {
	key string
}

func (u usernameStrategy) find(cache map[string]*contact.Contact) *contact.Contact {
	// 检查map中有没有，如果有，但是是默认，说明不存在，返回nil
	if c, ex := cache[u.key]; ex {
		if c == &def {
			return nil
		}
		return c
	}
	// 合法性校验：若 key 包含空白符、标点或特殊字符，非合法微信ID，避免发起 Detail 请求污染缓存
	if strings.ContainsAny(u.key, " \t\r\n:[]()\"'，。！？") {
		return nil
	}
	// 通过api获取联系人详情
	res, err := contactapi.Get().Detail([]string{u.key})
	if err != nil {
		cache[u.key] = &def
		return nil
	}
	if len(res.ContactList) == 0 || res.ContactList[0] == nil || res.ContactList[0].Username == nil || res.ContactList[0].Username.Value == "" {
		cache[u.key] = &def
		return nil
	}
	// 构建联系人并放入map
	if build, err := Build(res.ContactList[0]); err != nil || build.Username == "" {
		cache[u.key] = &def
		return nil
	} else {
		cache[build.Username] = build
		if build.Username != u.key {
			cache[u.key] = build
		}
		return build
	}
}

type nicknameStrategy struct {
	key string
}

func (n nicknameStrategy) find(cache map[string]*contact.Contact) *contact.Contact {
	v, _ := slice.FindBy(maputil.Values(cache), func(index int, item *contact.Contact) bool {
		return item.Nickname == n.key
	})
	return v
}

type remarkStrategy struct {
	key string
}

func (r remarkStrategy) find(cache map[string]*contact.Contact) *contact.Contact {
	v, _ := slice.FindBy(maputil.Values(cache), func(index int, item *contact.Contact) bool {
		return item.Remark == r.key
	})
	return v
}
