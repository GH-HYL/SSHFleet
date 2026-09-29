package ssh

// 批量改密（--change-password）的提示词表：认出远端在问什么，就喂对应的值。
//
// 值不在这张表里——当前密码取清单里那台机器的登录密码，新密码取命令行参数。
// 表只回答两件事：哪一段文字是提问，这一问该喂哪个值。
//
// 表的来源与校验不在这里：它由 `config.LoadPasswdConfig` 从 `config/passwd.conf` 读出来
//（同一个文件里还装着失败分类，那份给 result 用），见 `internal/config/passwd.go`。
// 本包只消费——`PasswdPrompts` 由装配层从配置填好再传进来。

// PasswdValue 一条 step 该喂的值从哪来。只能是下面两个取值之一。
type PasswdValue string

const (
	// PasswdValueCurrent 喂清单里那台机器的登录密码。
	PasswdValueCurrent PasswdValue = "current"
	// PasswdValueNew 喂 --change-password 给的新密码。
	PasswdValueNew PasswdValue = "new"
)

// PasswdStep 一条提示：提示原文 + 该喂哪个值 + 最多喂几次。
type PasswdStep struct {
	Keywords []string
	Value    PasswdValue
	Max      int
}

// PasswdPrompts 提示词表。顺序即提问顺序。
type PasswdPrompts struct {
	Steps []PasswdStep
}

// stepBudget 全表最多会喂多少次（送信通道的容量按它给，保证钩子永不阻塞）。
func (p *PasswdPrompts) stepBudget() int {
	total := 0
	for _, s := range p.Steps {
		total += s.Max
	}
	return total
}
