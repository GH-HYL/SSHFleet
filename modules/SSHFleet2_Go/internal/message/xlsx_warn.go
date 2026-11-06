package message

// 「某张表没生成出来」那一句（老版 output/output.go 的 writeXlsx 原话）。
//
// 不是致命错误：终端一关，只有工具日志能说明这次的表格为什么没生成，所以这一句同时上屏与
// 进日志；本次执行的结果与退出码都不动。

// XlsxFailed 生成某张表失败时的错误句（红色 [ERROR]，走标准错误）。
func XlsxFailed(name string, err error) Line {
	s := ansiRed + "[ERROR]" + ansiReset + " 生成 " + name + " 失败：" + err.Error() + "（本次跳过，不影响执行结果）\n"
	return Line{s: s, toErr: true}
}
