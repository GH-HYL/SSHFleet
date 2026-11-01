package guard

import (
	"strings"
	"testing"
)

// 类一：非 console 包写终端要报；console 包与 _test.go 不报。
func TestCheckTerminal(t *testing.T) {
	m := loadFixture(t, "violations")
	vs := CheckTerminal(m)

	if len(vs) != 2 {
		t.Fatalf("want 2 条（fmt.Println、os.Stderr），got %d：%v", len(vs), vs)
	}
	for _, v := range vs {
		if v.File != "internal/parse/run.go" {
			t.Errorf("位置 want internal/parse/run.go，got %s", v.File)
		}
		if !strings.Contains(v.Fix, "console") {
			t.Errorf("话术里要说清该走 console：%s", v)
		}
	}
	if inClass(CheckTerminal(loadFixture(t, "clean")), ClassTerminal) != nil {
		t.Error("clean 样本不该有写终端")
	}
	for _, v := range vs {
		if strings.Contains(v.File, "console") {
			t.Errorf("console 包写终端不算越界，却报了：%s", v)
		}
	}
}

// 非 _test.go 豁免：测试文件里打印不报。
func TestCheckTerminalExemptsTestFiles(t *testing.T) {
	m := loadFixture(t, "testsite")
	for _, v := range CheckTerminal(m) {
		if strings.HasSuffix(v.File, "_test.go") {
			t.Errorf("_test.go 里打印应当豁免：%s", v)
		}
	}
}
