package guard

import (
	"strings"
	"testing"
)

// 类一第四条：那一屏的正文不许取自 `.Error()`、也不许当场拼。
//
// 样本里三次犯规（两次走 Fatal、一次走 TraceRefusal），一次向 message 要正文的不报。
func TestCheckFatalBody(t *testing.T) {
	vs := CheckTerminal(loadFixture(t, "fatalbody"))

	var got []string
	for _, v := range vs {
		if v.Class != ClassTerminal {
			t.Errorf("该归类一，实际 %s：%s", v.Class, v)
		}
		if !strings.Contains(v.File, "internal/cli") {
			t.Errorf("报出来的位置该在站里：%s", v)
		}
		got = append(got, v.What)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 条（两处拼正文、一处日志那一半），got %d：%v", len(got), got)
	}
	for _, want := range []string{"err.Error()", "当场拼"} {
		found := false
		for _, what := range got {
			if strings.Contains(what, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("话术里要说清是哪种形状（缺 %q）：%v", want, got)
		}
	}
}
