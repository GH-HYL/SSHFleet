package message

import (
	"errors"
	"io/fs"
	"os"
)

// SystemReason 取系统那一句话，去掉「open <路径>:」「GetFileAttributesEx <路径>:」这类前缀。
//
// 口径照老版 `common.SystemReason`：路径已经在第一行说过一遍，原因栏里不该再出现一次；
// 带前缀还会让「找不到文件」看起来像是在说那条路径的写法有问题。
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
