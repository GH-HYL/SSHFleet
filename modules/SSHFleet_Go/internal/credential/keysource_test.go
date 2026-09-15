package credential

import (
	"strings"
	"testing"
)

// KeySource 是可注入的主密钥来源：「只提醒一次」这件事现在记在这个值上，
// 不再是包级布尔。于是「同一进程内提醒恰好一次」可以在不碰真实环境的前提下验证。

// fakeSource 造一个来源：环境变量与持久值由参数给定，输出进缓冲区。
func fakeSource(env, persisted, persistedWhere string) (*KeySource, *strings.Builder) {
	out := &strings.Builder{}
	return &KeySource{
		EnvLookup:       func() string { return env },
		PersistedLookup: func() KeySources { return KeySources{Persisted: persisted, PersistedWhere: persistedWhere} },
		WarnOut:         out,
	}, out
}

// Read 要把两侧来源合到一个视图里，且环境变量侧做首尾去空白。
func TestKeySourceReadMergesBothOrigins(t *testing.T) {
	s, _ := fakeSource("  env-key  ", "persisted-key", "~/.bashrc")
	src := s.Read()

	if src.Env != "env-key" {
		t.Fatalf("环境变量侧应做首尾去空白，实际 %q", src.Env)
	}
	if src.Persisted != "persisted-key" || src.PersistedWhere != "~/.bashrc" {
		t.Fatalf("持久值侧应原样保留，实际 %q / %q", src.Persisted, src.PersistedWhere)
	}
}

// 两侧独立读取：环境变量为空但持久值有值，这个状态必须能被如实分辨——
// 它正是「生成密钥后忘了让它生效」的典型症状。
func TestKeySourceReadKeepsBothSidesIndependent(t *testing.T) {
	s, _ := fakeSource("", "persisted-key", "注册表 HKCU\\Environment")
	src := s.Read()

	if src.EnvSet() {
		t.Fatalf("环境变量为空时 EnvSet 应为假，实际 Env=%q", src.Env)
	}
	if !src.PersistedSet() {
		t.Fatal("持久值有值时 PersistedSet 应为真")
	}
	if !strings.Contains(divergenceNote(KeySources{Env: "a", Persisted: "b"}), "不一致") {
		t.Fatal("两侧取值不同应判为不一致")
	}
}

// 「只提醒一次」是这条路径的核心契约：读凭据是按节点走的，几万个节点不能刷几万次。
func TestKeySourceWarnsDivergenceOnlyOnce(t *testing.T) {
	s, out := fakeSource("env-key", "persisted-key", "~/.bashrc")

	for i := 0; i < 5; i++ {
		s.warnDivergenceOnce()
	}

	warned := strings.Count(out.String(), "[警告]")
	if warned != 1 {
		t.Fatalf("来源不一致应恰好提醒一次，实际 %d 次：\n%s", warned, out.String())
	}
	for _, want := range []string{Fingerprint("env-key"), Fingerprint("persisted-key")} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("提醒缺少指纹 %q：\n%s", want, out.String())
		}
	}
	// 密钥本体不得出现在提醒里
	if strings.Contains(out.String(), "env-key") || strings.Contains(out.String(), "persisted-key") {
		t.Fatalf("提醒泄露了密钥本体：\n%s", out.String())
	}
}

// 两处一致时不该有警告——但「已经说过」仍要置位，
// 否则后来的不一致会在同一次运行里冒出来（同一进程只应有一次表态）。
func TestKeySourceConsistentSourceStaysSilent(t *testing.T) {
	s, out := fakeSource("same-key", "same-key", "~/.bashrc")
	s.warnDivergenceOnce()

	if out.Len() != 0 {
		t.Fatalf("来源一致时不该有输出，实际：\n%s", out.String())
	}
	if !s.warned {
		t.Fatal("无论是否真的打印，处理过一次就应置位，避免同一进程再次表态")
	}
}

// 预检自己会打更完整的一段（PrecheckKey），故它先把「已经说过」置位，
// 稍后逐节点读凭据时不再重复同一件事。
func TestKeySourceMarkWarnedSuppressesLaterWarning(t *testing.T) {
	s, out := fakeSource("env-key", "persisted-key", "~/.bashrc")

	s.markWarned()
	s.warnDivergenceOnce()

	if out.Len() != 0 {
		t.Fatalf("预检已说过则后续不该重复，实际：\n%s", out.String())
	}
}

// 多个来源值互不影响：提醒状态是值上的字段，不是全局的。
func TestKeySourceWarnStateIsPerValue(t *testing.T) {
	first, firstOut := fakeSource("a", "b", "~/.bashrc")
	second, secondOut := fakeSource("a", "b", "~/.bashrc")

	first.warnDivergenceOnce()
	second.warnDivergenceOnce()

	if firstOut.Len() == 0 || secondOut.Len() == 0 {
		t.Fatal("两个来源值各自都该提醒一次，说明状态没有串味")
	}
}

// key 取环境变量侧的值（当前应当使用的密钥）。
func TestKeySourceKeyReadsEnv(t *testing.T) {
	s, _ := fakeSource("  the-key  ", "", "")
	if got := s.key(); got != "the-key" {
		t.Fatalf("key() 应返回去空白后的环境变量，实际 %q", got)
	}
	if got := (&KeySource{EnvLookup: func() string { return "" }}).key(); got != "" {
		t.Fatalf("环境变量为空时 key() 应为空串，实际 %q", got)
	}
}
