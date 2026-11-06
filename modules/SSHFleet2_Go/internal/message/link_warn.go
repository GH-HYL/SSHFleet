package message

// 「最新一次」链接那两句（老版 output/archive.go 的原话，黄色 [警告] 前缀、走标准错误）。
//
// 名字由调用方给（`archive.LatestName`）——入口名只许有一处出处。
//
// 两件都不是致命错误：归档目录已经写完了，链接只是入口。所以上面打一句、日志记一笔，
// 本次执行的结果与退出码都不动。

// LatestLinkSkipped 当前目录里已经有一个同名普通文件时的警告（不删它，跳过创建）。
func LatestLinkSkipped(name string) Line {
	return Line{
		s:     ansiYellow + "[警告]" + ansiReset + " 当前目录已存在同名文件 " + name + "，跳过链接创建（如需快捷入口，删除该文件后重跑）\n",
		toErr: true,
	}
}

// LatestLinkFailed 链接建不起来时的警告：一句原因，一句怎么绕。
func LatestLinkFailed(name string, err error) Line {
	s := ansiYellow + "[警告]" + ansiReset + " 创建 " + name + " 目录链接失败：" + err.Error() + "\n"
	s += "提示：Windows 使用目录联接（需目标位于本地 NTFS 卷），POSIX 使用符号链接\n"
	return Line{s: s, toErr: true}
}
