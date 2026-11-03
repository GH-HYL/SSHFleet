package parse

// 脚本模式定解释器：按后缀查配置里的映射，命中就用它；没命中回退到命令解释器，
// 并把「这个后缀没配到」记成纸上一条事实——闸门据此问那一句「继续用 bash 吗」。
//
// 为什么回退要问一句：后缀五花八门，用户没配到的（.bin 之类）也得能跑，但「没配却照样
// 按某个解释器跑」这件事要让用户知情、点头，免得跑错了还不知道（老版 2026-10-07 定）。
//
// 上传／下载／改密不参与：它们各自的中间命令另有固定形态，配置随意改会把那些写死的命令
// 弄失效。所以那三支的解释器是空串。

import (
	"path/filepath"
	"strings"
)

// interpreterFor 定本次执行用的解释器，以及「回退时那个后缀」。
//
// 后缀按**本机**规则取（`filepath.Ext`）：脚本是本地文件，Windows 上 `D:\a\b.sh` 的最后
// 一段是 `b.sh`。用 POSIX 的 `path.Ext` 会拿整条 `D:\a\b.sh` 当最后一段——那一段里没有点
// 就直接判成「没有后缀」，静默回退。
//
// 比较忽略大小写：配置加载时键已归一成小写，这里把取到的后缀也转小写。
func interpreterFor(scriptPath, commandInterpreter string, table map[string]string) (interpreter, fallbackExt string) {
	if scriptPath == "" {
		return "", ""
	}
	ext := filepath.Ext(scriptPath)
	if ext != "" {
		if v, ok := table[strings.ToLower(ext)]; ok {
			return v, ""
		}
	}
	return commandInterpreter, ext
}

// scriptBody 脚本正文的整备：剥 UTF-8 BOM、去掉整块首尾空白。
//
// BOM 不剥会跟着内容进远端 bash：第一行变成带 BOM 的命令，bash 报
// "No such file or directory"、那一行废掉（清单侧同口径）。
//
// 只做一次：危险检测、长度判据与实际下发读的都是这一份，两处各整备一次就会算出不同的长度。
func scriptBody(data []byte) string {
	return strings.TrimSpace(strings.TrimPrefix(string(data), "\ufeff"))
}
