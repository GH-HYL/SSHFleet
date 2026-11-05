package message

import (
	"testing"

	"sshfleet2/internal/contract"
)

// 未送出的代填那一段：序号对位、触发词带直引号、多条顿号相连；取不到触发词只写序号。
// 措辞抄老版 `internal/output/reporter.go` 的 missedNote，逐字比。
func TestAnswerMissedNote(t *testing.T) {
	answers := []contract.Answer{
		{Value: "y", Triggers: []string{"Continue?", "To continue"}},
		{Value: "", Triggers: []string{"Password:"}},
	}

	cases := []struct {
		name   string
		missed []int
		want   string
	}{
		{"一条都没漏：不出现这一段", nil, ""},
		{"漏一条：带触发词原文", []int{2}, `未送出的代填：第 2 条（触发词 "Password:"）`},
		{"漏两条：多个触发词顿号相连", []int{1, 2}, `未送出的代填：第 1 条（触发词 "Continue?"、"To continue"）、第 2 条（触发词 "Password:"）`},
		{"序号越界：只写序号", []int{9}, "未送出的代填：第 9 条"},
	}

	for _, c := range cases {
		if got := AnswerMissedNote(answers, c.missed); got != c.want {
			t.Fatalf("%s：want %q，got %q", c.name, c.want, got)
		}
	}
}
