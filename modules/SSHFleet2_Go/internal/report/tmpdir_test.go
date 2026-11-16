package report

import (
	"os"
	"path/filepath"
)

// 这个包的测试一起跑就把临时根钉进工作区的 build/tmp。
//
// 为什么不用 TestMain：TestMain 的收尾动作是 os.Exit，而退出只有塔尖能做——guard 的类三
// 对 _test.go 不豁免。init 在跑测试之前执行，不碰退出。
//
// 规范三.1：产物与临时文件 MUST NOT 出工作区（含 $TEMP）。落在外面的可执行文件会被 360
// 静默删掉，现象是构建莫名失败、不报原因。所以这里不指望跑测试的人记得 export。
func init() {
	dir := buildTmp()
	if dir == "" {
		// 定位不到就不改：宁可让产物落回系统默认（照旧有人手 export 兜着），
		// 也不要猜一个来路不明的地方往里写。
		return
	}
	for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
		_ = os.Setenv(k, dir)
	}
}

// buildTmp 从当前包目录上溯找 go.mod，落到「模块根上两级/build/tmp/test/report」。
func buildTmp() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
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
	root := filepath.Join(filepath.Dir(filepath.Dir(dir)), "build", "tmp", "test", "report")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return ""
	}
	return root
}
