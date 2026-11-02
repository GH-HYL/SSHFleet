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

// 类一第三条判据：console 交出 writer 的那一处，调用点只许在执行站。
//
// 样本里两站各取了一次：解析站那次报红，执行站那次放过。
func TestCheckTerminalWriterHome(t *testing.T) {
	vs := CheckTerminal(loadFixture(t, "writer"))
	if len(vs) != 1 {
		t.Fatalf("want 1 条（解析站取了 writer），got %d：%v", len(vs), vs)
	}
	if !strings.Contains(vs[0].File, "parse") {
		t.Fatalf("该报的应是解析站那一次，实际报在 %s", vs[0].File)
	}
	if !strings.Contains(vs[0].Why, "执行站") {
		t.Fatalf("话术里要说清只许执行站拿：%s", vs[0])
	}
}
