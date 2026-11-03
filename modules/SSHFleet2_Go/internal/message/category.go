package message

// 分类名：判据表给的那些由配置文件写，剩下三个由代码给——成功那一类（在 `modename.go`
// 那张模式名字表里）、按退出码兜底的那一类、完全无信息时的兜底。
//
// 这三个都是给人看的字，所以住这里（ADR-0032 第四条「唯一改字处」）。三个站读的是同一份：
// 执行站写进纸里、结算站拿它算成败、报告站印出来——分住三处就等于同一句话有三个来源。

import (
	"strconv"
	"strings"
)

// CategoryPasswdExpired 是判据表里名为「密码过期」的那一类。
//
// 代填拿它的关键词当**中止词**（碰上就地收手）；改密拿它当**入场信号**（出现才开始喂）。
// 同一份关键词，方向相反——两处都从这里取，改名只改这一处。
const CategoryPasswdExpired = "密码过期"

// CategoryUnknown 完全无信息时的兜底分类（老版 `verdict.Unclassified`，一字不改）。
const CategoryUnknown = "原因未知"

// exitCodeCategoryPrefix 按退出码自动给的兜底分类的前缀。
//
// 那个名字里带一个数字，配置文件写不了这样的 `name`，所以它是本工程唯一一处「代码给分类名」
// （ADR-0021 决定第 3 条的例外，老版 `verdict.exitCodeFailPrefix` 同口径）。
const exitCodeCategoryPrefix = "执行失败(退出码"

// IsExitCodeCategory 分类名是不是「按退出码自动给的那一类」（`执行失败(退出码N)`）。
//
// 结算站与报告站要认这一类（它的退出码另有出处、报告要单列），判据只有前缀一条——
// 认名字的地方只此一处，别处拿它别再抄一遍前缀。
func IsExitCodeCategory(name string) bool {
	return strings.HasPrefix(name, exitCodeCategoryPrefix)
}

// CategoryOfExitCode 目的命令以非 0 收场、判据表第二组也没认出来时按退出码给的那一类。
//
// 名字是「执行失败(退出码3)」这种形状。要那个数字时读纸上「退出码」那一格，**不要去解名字**
// ——解名字等于把一处字面量当成数据源（老版 2026-10-09 就为这个改过一次）。
func CategoryOfExitCode(code int) string {
	return exitCodeCategoryPrefix + strconv.Itoa(code) + ")"
}
