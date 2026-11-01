package guard

import (
	"testing"

	"sshfleet2/internal/parse"
)

// 红线：解析站的入口（递一张纸、收一张纸）。只判形状——配置在时不是起不来、
// 配置不在时是起不来；两种情况都要有话。配置在不在靠切换工作目录，不靠注入。

func TestRedlineParseSaysOrFail(t *testing.T) {
	t.Chdir("../..") // 配置在：随包那份
	say, fail := parse.Run([]string{"sshfleet2", "-h"}, "9.9.9")
	if fail != nil {
		t.Errorf("配置在时不应是起不来")
	}
	if say.S == "" {
		t.Errorf("应有一句要打给使用者的话")
	}

	t.Chdir(t.TempDir()) // 配置不在
	blank, fail := parse.Run([]string{"sshfleet2", "-h"}, "9.9.9")
	if fail == nil {
		t.Errorf("配置不在应是起不来")
	}
	if blank.S == "" {
		t.Errorf("起不来也要有一句话")
	}
}
