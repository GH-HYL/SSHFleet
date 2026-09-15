package credential

import (
	"strings"
	"testing"
)

// 主密钥来源一致性检测（2026-09-15）：不一致是「凭据解密报主密钥不匹配」的常见根因，
// 且加密方向必须拦下（用旧密钥加的密，重开终端后就解不开了）。

func TestKeySourcesDiverged(t *testing.T) {
	cases := []struct {
		name string
		src  KeySources
		want bool
	}{
		{"两处一致", KeySources{Env: "A", Persisted: "A", PersistedWhere: "~/.bashrc"}, false},
		{"两处不同", KeySources{Env: "A", Persisted: "B"}, true},
		{"仅环境变量（未持久化）", KeySources{Env: "A"}, false},
		{"仅持久值（终端未读到）", KeySources{Persisted: "A"}, false},
		{"都为空", KeySources{}, false},
		{"两侧 rc 文件不同", KeySources{Env: "A", Persisted: "A", OtherKey: "B", OtherWhere: "~/.zshrc"}, true},
		{"两侧 rc 文件相同", KeySources{Env: "A", Persisted: "A", OtherKey: "A"}, false},
		{"首尾空白不算差异", KeySources{Env: " A ", Persisted: "A"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.src.Diverged(); got != c.want {
				t.Fatalf("Diverged() 应为 %v，实际 %v", c.want, got)
			}
		})
	}
}

func TestDivergenceNote(t *testing.T) {
	if note := divergenceNote(KeySources{Env: "A", Persisted: "A"}); note != "" {
		t.Fatalf("一致时不该有提示，实际：%q", note)
	}

	note := divergenceNote(KeySources{
		Env: "A", Persisted: "B", PersistedWhere: "注册表 HKCU\\Environment",
	})
	for _, want := range []string{
		"主密钥来源不一致",
		Fingerprint("A"), Fingerprint("B"),
		envName,
		"注册表 HKCU\\Environment",
	} {
		if !strings.Contains(note, want) {
			t.Fatalf("提示缺少 %q：\n%s", want, note)
		}
	}
	// 不得把密钥本体打出来
	if strings.Contains(note, "A") && !strings.Contains(note, Fingerprint("A")) {
		t.Fatalf("提示疑似泄露密钥本体：\n%s", note)
	}
}

func TestFingerprint(t *testing.T) {
	if Fingerprint("") != "（未设置）" || Fingerprint("   ") != "（未设置）" {
		t.Fatalf("空密钥的指纹应是「（未设置）」")
	}
	if Fingerprint("key-a") != Fingerprint(" key-a ") {
		t.Fatalf("指纹应忽略首尾空白")
	}
	if Fingerprint("key-a") == Fingerprint("key-b") {
		t.Fatalf("不同密钥的指纹不应相同")
	}
	if len(Fingerprint("key-a")) != 8 {
		t.Fatalf("指纹应为 8 位十六进制，实际 %q", Fingerprint("key-a"))
	}
	// 指纹不能暴露密钥本身
	if strings.Contains(Fingerprint("key-a"), "key") {
		t.Fatalf("指纹不应包含密钥内容")
	}
}

// GuardEncrypt 必须与来源一致性判定一致：不一致→拦下，一致→放行。
func TestGuardEncryptFollowsSourceConsistency(t *testing.T) {
	t.Setenv(envName, "definitely-not-the-persisted-key")
	src := ReadKeySources()
	diverged := src.PersistedSet() && src.Env != src.Persisted

	err := GuardEncrypt()
	if diverged {
		if err == nil {
			t.Fatal("两处密钥不一致时应拦下加密")
		}
		for _, want := range []string{"已阻止加密", "出路", Fingerprint(src.Env), Fingerprint(src.Persisted)} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("拦截提示缺少 %q：\n%s", want, err)
			}
		}
		return
	}
	if err != nil {
		t.Fatalf("两处一致时不应拦下：%v", err)
	}
}

// 自检状态映射与提示：状态随本机实际环境而定，故按实际来源分支断言（不用跳过）。
func TestInspectKeyAndReports(t *testing.T) {
	const fakeKey = "selftest-env-only-key"

	t.Run("环境变量与持久值不同→不一致", func(t *testing.T) {
		t.Setenv(envName, fakeKey)
		state, src := InspectKey()
		if !src.PersistedSet() {
			t.Log("本机没有持久主密钥，跳过「不一致」分支")
			return
		}
		if state != KeyStateDiverged {
			t.Fatalf("应为 diverged，实际 %s", state)
		}
		note := PrecheckKey()
		for _, want := range []string{"不一致", "影响", "让两处一致", Fingerprint(fakeKey)} {
			if !strings.Contains(note, want) {
				t.Fatalf("预检查提示缺少 %q：\n%s", want, note)
			}
		}
	})

	t.Run("持久值存在但终端为空→已保存未读到", func(t *testing.T) {
		t.Setenv(envName, "")
		state, src := InspectKey()
		if !src.PersistedSet() {
			t.Log("本机没有持久主密钥，跳过「未读到」分支")
			return
		}
		if state != KeyStateStaleShell {
			t.Fatalf("应为 stale_shell，实际 %s", state)
		}
		note := PrecheckKey()
		for _, want := range []string{"没读到已保存的主密钥", "不需要重新生成", "让它生效"} {
			if !strings.Contains(note, want) {
				t.Fatalf("预检查提示缺少 %q：\n%s", want, note)
			}
		}
		// 这条提示是「改了密钥反而报缺少主密钥」场景的解药，必须出现在错误里
		if _, err := GetMasterKey(); err == nil || !strings.Contains(err.Error(), "没读到已保存的主密钥") {
			t.Fatalf("缺密钥的错误应点明「已保存但没读到」，实际：%v", err)
		}
	})

	t.Run("两处都空→未设置", func(t *testing.T) {
		t.Setenv(envName, "")
		if _, src := InspectKey(); src.PersistedSet() {
			t.Log("本机有持久主密钥，跳过「未设置」分支")
			return
		}
		if state, _ := InspectKey(); state != KeyStateUnset {
			t.Fatalf("应为 unset，实际 %s", state)
		}
		if !strings.Contains(PrecheckKey(), "") || PrecheckKey() != "" {
			t.Fatalf("未设置状态不该有开工警告（交由具体报错点处理）")
		}
	})
}

// 报告四种状态都要给出「现状 + 下一步」，且不得泄露密钥本体。
func TestKeyStatusReportNeverLeaksKey(t *testing.T) {
	t.Setenv(envName, "status-report-secret-key")
	report := KeyStatusReport()
	if !strings.Contains(report, "主密钥状态") || !strings.Contains(report, "下一步") {
		t.Fatalf("报告缺少状态或下一步：\n%s", report)
	}
	if strings.Contains(report, "status-report-secret-key") {
		t.Fatalf("报告泄露了密钥本体：\n%s", report)
	}
}
