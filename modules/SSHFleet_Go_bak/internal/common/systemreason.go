// 系统报错原文提取：错误文案里只留系统说的那一句，前面的路径不重复。
package common

import (
	"errors"
	"io/fs"
	"os"
)

// SystemReason 取系统给的那一段原因，去掉前面重复的路径。
//
// 报错第一行已经点过当前值了，这里只留错因本身：`open /x/y: no such file or directory`
// 取成 `no such file or directory`。两种形态都剥——*fs.PathError（stat / open / read）
// 与 *os.LinkError（rename 一类）。
//
// cli 与 credential 各有一份逐字同构的实现，合并到这里（一个概念一处定义）。
func SystemReason(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		return le.Err.Error()
	}
	return err.Error()
}
