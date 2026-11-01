package parse

import (
	"fmt"
	"runtime/debug"
	"time"
)

// 版本串：号只有一个出处——塔尖那行顶格常量（发布脚本按这一行改号）；拼装归解析站
//（14 号工单答案：版本与帮助同源）。构建标识取运行期构建信息，取不到按老版退化。

// buildInfo 读运行期构建信息：提交号、提交时间、工作区有没有未提交改动。
// 取不到（仓库外编译 / -buildvcs=false）时三样都是零值。
func buildInfo() (rev, tm string, dirty bool) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "", "", false
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			tm = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	return rev, tm, dirty
}

// versionString 拼「版本: v…」那一串。退化形态照老版 versionWithBuildID，不造第三种写法：
//
//   - 有提交号与时间：v<号>+<短提交号> (<提交时间的本地时间>)
//   - 只有提交号：   v<号>+<短提交号>（工作区有未提交改动时短号后带 -dirty）
//   - 只有时间：     v<号> (<时间>)
//   - 都没有：       v<号>
//
// 时间解析不出来按没有算（老代码同一处分支）。
func versionString(num, rev, tm string, dirty bool) string {
	id := ""
	if rev != "" { // 短提交号取前 7 位（与 git 的默认缩写一致）
		id = rev
		if len(id) > 7 {
			id = id[:7]
		}
		if dirty {
			id += "-dirty"
		}
	}
	local := func(ts string) (string, bool) {
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return "", false
		}
		return t.Local().Format("2006-01-02 15:04:05"), true
	}
	switch {
	case id != "" && tm != "":
		if s, ok := local(tm); ok {
			return fmt.Sprintf("v%s+%s (%s)", num, id, s)
		}
		return fmt.Sprintf("v%s+%s", num, id)
	case id != "":
		return fmt.Sprintf("v%s+%s", num, id)
	case tm != "":
		if s, ok := local(tm); ok {
			return fmt.Sprintf("v%s (%s)", num, s)
		}
	}
	return "v" + num
}
