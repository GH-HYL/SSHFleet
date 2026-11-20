// Package testtmp 把测试二进制的临时根钉进工作区的 build/tmp。
//
// 各包的测试文件空引用它一句就生效：
//
//	import _ "sshfleet2/internal/testtmp"
//
// **为什么不用 TestMain**：TestMain 的收尾动作是 os.Exit，而退出只有塔尖能做——guard 的类三
// 对 _test.go 不豁免。init 在跑测试之前执行，不碰退出，所以钉在这一个 init 里。
//
// **为什么必须钉**（`docs/agents/按需-写代码.md`「工作区结构」）：产物与临时文件 MUST NOT 出工作区（含 $TEMP）。落在外面的
// 可执行文件会被 360 静默删掉，现象是构建莫名失败、不报原因。所以这里不指望跑测试的人记得
// export 那三个变量。
//
// **为什么只有测试文件 import 得到它**：规则表里这个包的依赖是空的，生产文件按那一行判，
// import 它会红；_test.go 享类二的豁免，才进得来。
package testtmp

import (
	"os"
	"path/filepath"
)

func init() {
	dir := sandbox()
	if dir == "" {
		// 定位不到就不改：宁可让产物落回系统默认（照旧有人手 export 兜着），
		// 也不要猜一个来路不明的地方往里写。
		return
	}
	for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
		_ = os.Setenv(k, dir)
	}
}

// sandbox 落到「模块根上两级/build/tmp/test/<包名>」。
//
// 包名取当前工作目录的末一段：go test 把工作目录设成被测包自己那一层，所以同一个文件
// 在十七个包里各得其所，不必按包抄十七份。
func sandbox() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	pkg := filepath.Base(wd)
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	root := filepath.Join(filepath.Dir(filepath.Dir(dir)), "build", "tmp", "test", pkg)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return ""
	}
	return root
}
