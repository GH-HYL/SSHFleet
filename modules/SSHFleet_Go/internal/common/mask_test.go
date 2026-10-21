package common

import (
	"encoding/csv"
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

// Excel 导出的单行写法：整段带引号。取值判定认它是一条节点，
// 脱敏也得认——否则这行密码会明文进日志、确认屏和报告。
func TestMaskCommandLineMasksQuotedInlineList(t *testing.T) {
	got := MaskCommandLine([]string{"SSHFleet.exe", "-f", `"10.0.0.1",22,root,s3cret`, "-c", "uptime"})
	if strings.Contains(got[2], "s3cret") {
		t.Fatalf("带引号的内联清单也要脱敏，实际：%q", got[2])
	}
	// 期望值从 CSV 口径写出来：IP、端口、用户名原样，第 4 段遮住。
	var want strings.Builder
	w := csv.NewWriter(&want)
	_ = w.Write([]string{"10.0.0.1", "22", "root", "****"})
	w.Flush()
	if got[2] != strings.TrimSuffix(want.String(), "\n") {
		t.Fatalf("IP、端口、用户名应原样保留，实际：%q", got[2])
	}
}

// 六列都带引号：第 4 段密码、第 6 段口令都整段遮住，其余保留。
func TestMaskCommandLineMasksQuotedInlineListSixColumns(t *testing.T) {
	got := MaskCommandLine([]string{"SSHFleet.exe", "-f", `"10.0.0.1","22","root","pw1","key.bin","keypw1"`})
	if strings.Contains(got[2], "pw1") || strings.Contains(got[2], "keypw1") {
		t.Fatalf("密码与私钥口令都不该留下明文，实际：%q", got[2])
	}
	var want strings.Builder
	w := csv.NewWriter(&want)
	_ = w.Write([]string{"10.0.0.1", "22", "root", "****", "key.bin", "****"})
	w.Flush()
	if got[2] != strings.TrimSuffix(want.String(), "\n") {
		t.Fatalf("第 4、6 段应遮住、其余保留，实际：%q", got[2])
	}
}

// 引号里包着逗号：按字段切才不会把段号切错，密码照样遮住。
func TestMaskCommandLineMasksQuotedComma(t *testing.T) {
	got := MaskCommandLine([]string{"SSHFleet.exe", "-f", `10.0.0.1,22,"ro,ot",s3cret`})
	if strings.Contains(got[2], "s3cret") {
		t.Fatalf("引号前有逗号时密码仍要遮住，实际：%q", got[2])
	}
	var want strings.Builder
	w := csv.NewWriter(&want)
	_ = w.Write([]string{"10.0.0.1", "22", "ro,ot", "****"})
	w.Flush()
	if got[2] != strings.TrimSuffix(want.String(), "\n") {
		t.Fatalf("引号里的逗号不该把段号切错，实际：%q", got[2])
	}
}

// 裸 IP 没有凭据段，不是什么「特殊写法」，就是没有可脱的东西。
func TestMaskCommandLineBareIPUntouched(t *testing.T) {
	got := MaskCommandLine([]string{"SSHFleet.exe", "-f", "10.0.0.1"})
	if got[2] != "10.0.0.1" {
		t.Fatalf("裸 IP 应原样保留，实际：%q", got[2])
	}
}

// 密码短到没有可留的前后位时整段遮住。
func TestMaskCommandLineShortPassword(t *testing.T) {
	got := MaskCommandLine([]string{"--change-password", "pw"})
	if got[1] != "****" {
		t.Fatalf("短密码应整段遮住，实际：%q", got[1])
	}
}
