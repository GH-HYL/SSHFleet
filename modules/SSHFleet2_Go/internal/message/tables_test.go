package message

import "testing"

// 判据表「出处」那一句（自定义区）：六个字段名与「六项」是同一句话，逐字照迁移前那一处。
func TestDangerRuleKnownFields(t *testing.T) {
	want := "规则只认 name / example / reason / regex / risk_level / enabled 六项，把它删掉或改成这六项之一"
	if got := DangerRuleKnownFields(); got != want {
		t.Fatalf("危险规则表的出处那一句：\n got  %q\n want %q", got, want)
	}
}
