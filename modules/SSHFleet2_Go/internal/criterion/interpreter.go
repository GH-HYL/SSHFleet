package criterion

import (
	"path"
	"strings"
)

// InterpreterBaseName 把配置里的解释器取值归一成「认名用的标准名」：取第一个词
// （带参数如 `perl -w` 时只看第一个词）、去目录、转小写、去尾部版本号
// （/usr/bin/python3 → python、php8.1 → php、zsh5 → zsh）。
//
// 这套剥法全工具只有这一份：`-a` 的支持范围、$0 补名的家族判定、`source` 前要关的选项、
// 确认屏要不要省掉「解释器」那一行，都认它。
//
// **只认 `/`**（照老版 `ssh.InterpreterBaseName` 的 `path.Base`）：解释器是喂给**目标机**的，
// 那里的分隔符就是 `/`。写成 `C:/Python/bash` 这种本机路径归不出 `bash`，于是确认屏照实印出
// 这一格、`-a` 也说「不支持」——两处同一个结论，且错在解析期就暴露。若为「让两处都对上」
// 把 `\` 也切掉，归出来的 `bash` 会把这条配错的解释器放行到执行期才失败，比认不出更糟。
func InterpreterBaseName(interpreter string) string {
	fields := strings.Fields(interpreter)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimRight(strings.ToLower(path.Base(fields[0])), "0123456789.")
}
