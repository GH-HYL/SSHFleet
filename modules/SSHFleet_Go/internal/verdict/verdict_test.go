package verdict

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"sshfleet/internal/cli"
	"sshfleet/internal/ssh"
)

// 语料回归（常驻）：旧工程 test/test_error_classification.py 的 30 条用例 + 配置归类断言。
// 字段语义：corpusCase 描述的是**执行侧写下的事实**（Result 的事实区子集 + 模式），
// 经 Judge 判定后与 expected 比对；成功行按统计侧口径转中文（D2）。

type corpusCase struct {
	Name         string `toml:"name"`
	ExitCode     *int   `toml:"exit_code"`
	Error        string `toml:"error"`
	Banner       string `toml:"banner"`
	Output       string `toml:"output"`
	Mode         string `toml:"mode"`
	SuccessFiles int    `toml:"success_files"`
	FailedFiles  int    `toml:"failed_files"`
	Expected     string `toml:"expected"`
}

type corpusFile struct {
	Cases []corpusCase `toml:"cases"`
}

func loadKeywords(t *testing.T) *Keywords {
	t.Helper()
	kw, err := LoadKeywords(filepath.Join("..", "..", "config", "keywords_error.conf"))
	if err != nil {
		t.Fatalf("关键词文件应可加载: %v", err)
	}
	return kw
}

// judgeCorpus 按语料字段造事实、跑判定，返回**展示用**分类名（成功行转中文）。
func judgeCorpus(c corpusCase, kw *Keywords) string {
	res := ssh.Result{
		ConnectSuccess:  true,
		Error:           strPtrOf(c.Error),
		ServerBanner:    c.Banner,
		Output:          c.Output,
		SuccessFiles:    c.SuccessFiles,
		FailedFiles:     c.FailedFiles,
		CommandExitCode: c.ExitCode,
		SessionBegun:    c.ExitCode != nil,
	}
	mode := cli.ModeCommand
	switch c.Mode {
	case "upload":
		mode = cli.ModeUpload
	case "download":
		mode = cli.ModeDownload
	}
	Judge(&res, mode, kw)
	if res.Verdict == Success {
		// 成功行的展示分类由统计/呈现侧按模式转中文（D2）；result 包引 verdict，
		// 这里只能写字面量、不能回引它的常量
		if mode == cli.ModeUpload || mode == cli.ModeDownload {
			return "传输成功"
		}
		return "执行成功"
	}
	return res.Category
}

func strPtrOf(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func TestErrorClassificationCorpus(t *testing.T) {
	kw := loadKeywords(t)

	var file corpusFile
	if _, err := toml.DecodeFile(filepath.Join("..", "..", "testdata", "error_classification_cases.toml"), &file); err != nil {
		t.Fatal(err)
	}

	passed := 0
	for _, c := range file.Cases {
		if got := judgeCorpus(c, kw); got == c.Expected {
			passed++
			continue
		} else {
			t.Errorf("语料不通过：%s\n  期望 %q\n  实际 %q", c.Name, c.Expected, judgeCorpus(c, kw))
		}
	}
	t.Logf("错误分类语料：%d/%d 通过", passed, len(file.Cases))
	if passed != len(file.Cases) {
		t.Fatalf("分类行为与基线不一致：%d/%d", passed, len(file.Cases))
	}
}

// TestKeywordPlacement 配置归类断言（对位旧语料的三个布尔检查）。
func TestKeywordPlacement(t *testing.T) {
	kw := loadKeywords(t)

	authLower := lowerAll(kw.KeywordsOf("身份验证失败"))
	if contains(authLower, "permission denied") {
		t.Error("permission denied 不应在「身份验证失败」分类里")
	}
	if !contains(kw.KeywordsOf("权限不足"), "permission denied") {
		t.Error("permission denied 应在「权限不足」分类里")
	}
	if !contains(authLower, "permission denied (publickey") {
		t.Error("认证后缀串应在「身份验证失败」分类里")
	}
}

// TestFallbackCategory 兜底判定（终端单行显示用）。
func TestFallbackCategory(t *testing.T) {
	kw := loadKeywords(t)
	unknownErr := "disk quota exceeded on /var/log"

	cases := []struct {
		category string
		want     bool
	}{
		{"SFTP失败", false},
		{"执行失败(退出码2)", false},
		{"部分成功", false},
		{"原因未知", false},
		{"触发词未命中", false},
		{unknownErr, true},
		{"", false},
	}
	for _, c := range cases {
		if got := IsFallbackCategory(c.category, kw); got != c.want {
			t.Errorf("IsFallbackCategory(%q) = %v，期望 %v", c.category, got, c.want)
		}
	}
}

// TestAuthFailureKeywordPath 认证失败分类优先于关键词推断（spec D44）。
// 仅公钥被拒 → 归「密钥不匹配」（关键词推断，不依赖任何显式分类字段）。
func TestAuthFailureKeywordPath(t *testing.T) {
	kw := loadKeywords(t)
	noPwd := "ssh: handshake failed: ssh: unable to authenticate, attempted methods [none publickey], no supported methods remain"

	res := ssh.Result{ConnectSuccess: false, Error: &noPwd}
	Judge(&res, cli.ModeCommand, kw)
	if res.Category != "密钥不匹配" {
		t.Fatalf("仅公钥被拒应归「密钥不匹配」，实为 %q", res.Category)
	}
}

// 判定与新结构事实的几个钉子（result-verdict spec 9.5 的纸面替身）：
func TestJudgeStructuralFacts(t *testing.T) {
	kw := loadKeywords(t)
	zero := 0
	one := 1

	// D1 活证：退出码 0 却计入失败（代填未命中）
	res := ssh.Result{
		ConnectSuccess: true, SessionBegun: true, CommandExitCode: &zero,
		AnswersMissed: []int{1},
	}
	Judge(&res, cli.ModeScript, kw)
	if res.Verdict != Other || res.Category != "触发词未命中" || res.ExitCode == nil || *res.ExitCode != 0 {
		t.Fatalf("代填未命中应 other/触发词未命中/定论退出码 0，实为 %+v", res)
	}

	// 中止词 → 「密码过期」，定论退出码为空
	res = ssh.Result{ConnectSuccess: true, SessionBegun: true, AbortLine: "WARNING: Your password has expired."}
	Judge(&res, cli.ModeCommand, kw)
	if res.Category != "密码过期" || res.ExitCode != nil {
		t.Fatalf("中止词应密码过期/退出码空，实为 %+v", res)
	}

	// 传输部分成功不查表；定论退出码空
	res = ssh.Result{ConnectSuccess: true, SuccessFiles: 2, FailedFiles: 1}
	Judge(&res, cli.ModeUpload, kw)
	if res.Category != "部分成功" || res.ExitCode != nil || res.Verdict != Other {
		t.Fatalf("部分成功判定不对，实为 %+v", res)
	}

	// 改密未见过期信号 → 「密码未过期」；`:` 在 Steps、CommandExitCode 恒空
	res = ssh.Result{ConnectSuccess: true, SessionBegun: true, Steps: []ssh.StepResult{{Name: ssh.StepPasswd, ExitCode: &zero}}}
	Judge(&res, cli.ModePasswd, kw)
	if res.Category != "密码未过期" || res.ExitCode != nil {
		t.Fatalf("改密未过期判定不对，实为 %+v", res)
	}

	// 目的命令退非 0 → 第二组（选组纯字段判定，D33）；定论退出码保留
	res = ssh.Result{
		ConnectSuccess: true, SessionBegun: true, CommandExitCode: &one,
		Output: "WARNING: Your password has expired.\nPassword change required but no TTY available.",
	}
	Judge(&res, cli.ModeCommand, kw)
	if res.Category != "密码过期" || res.ExitCode == nil || *res.ExitCode != 1 {
		t.Fatalf("会话被拒应密码过期/退出码 1，实为 %+v", res)
	}

	// 预检退出码进 Steps 的下载失败 → 第一组按报错原文归类（D27 消掉的第二组补丁）
	step := 1
	res = ssh.Result{
		ConnectSuccess: true, Steps: []ssh.StepResult{{Name: "test -e", ExitCode: &step}},
		Error: strPtrOf("远程路径不存在: /var/log/x"),
	}
	Judge(&res, cli.ModeDownload, kw)
	if res.Category != "远程路径不存在" || res.ExitCode != nil {
		t.Fatalf("下载预检失败应第一组/远程路径不存在，实为 %+v", res)
	}
}

func lowerAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, strings.ToLower(s))
	}
	return out
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
