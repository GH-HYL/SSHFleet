package common

import (
	"strings"
	"testing"
)

// 命令行脱敏：内联清单按形状认，--change-password 按旗标名认。
// 后者必须靠旗标名——新密码是明确知道身份的凭据，形态上没有特征可认。

func TestMaskCommandLineMasksPasswordFlag(t *testing.T) {
	argv := []string{"SSHFleet.exe", "-f", "nodes.csv", "--change-password", "NewPass123", "--yes"}
	got := MaskCommandLine(argv)

	for _, part := range got {
		if strings.Contains(part, "NewPass123") {
			t.Fatalf("新密码不该明文留在命令行里：%v", got)
		}
	}
	if got[3] != "--change-password" {
		t.Fatalf("旗标名本身应保留，实际：%q", got[3])
	}
	if got[4] != "Ne****23" {
		t.Fatalf("值应换成脱敏形态，实际：%q", got[4])
	}
}

// `--change-password=密码` 这种写法同样要脱。
func TestMaskCommandLineMasksPasswordFlagInline(t *testing.T) {
	got := MaskCommandLine([]string{"SSHFleet.exe", "--change-password=NewPass123"})
	if strings.Contains(strings.Join(got, " "), "NewPass123") {
		t.Fatalf("等号写法也要脱敏：%v", got)
	}
	if got[1] != "--change-password=Ne****23" {
		t.Fatalf("等号写法应保留前缀，实际：%q", got[1])
	}
}

// 内联清单那条规则不受影响。
func TestMaskCommandLineKeepsInlineListBehaviour(t *testing.T) {
	got := MaskCommandLine([]string{"SSHFleet.exe", "-f", "1.1.1.1,22,root,secret"})
	if got[2] != "1.1.1.1,22,root,****" {
		t.Fatalf("内联清单的密码列应整段脱敏，实际：%q", got[2])
	}
}

// 密码短到没有可留的前后位时整段遮住。
func TestMaskCommandLineShortPassword(t *testing.T) {
	got := MaskCommandLine([]string{"--change-password", "pw"})
	if got[1] != "****" {
		t.Fatalf("短密码应整段遮住，实际：%q", got[1])
	}
}
