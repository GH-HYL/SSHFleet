package verdict

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"sshfleet/internal/cli"
	"sshfleet/internal/config"
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
	// 与 main 同一条装法：改密分类插在判据表最前面，改密模式命中的是它。
	// 少了这一步，改密路径的用例会误用通用表——测出来的绿不代表改密那条链是通的。
	pc, err := config.LoadPasswdConfig(filepath.Join("..", "..", "config", "keywords_passwd.conf"))
	if err != nil {
		t.Fatalf("改密配置应可加载: %v", err)
	}
	cats := make([]Category, 0, len(pc.Categories))
	for _, x := range pc.Categories {
		cats = append(cats, Category{Name: x.Name, Keywords: x.Keywords, Tip: x.Tip})
	}
	kw.PrependCategories(cats)
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
	case "passwd":
		mode = cli.ModePasswd
	}
	Judge(&res, mode, kw)
	if res.Verdict == Success {
		// 成功行的展示分类由呈现侧按模式给（D2）。这里写**字面量**、不读 cli 的名字表：
		// 期望值不跟着实现走，实现改错了这一行才会响——这是本断言存在的意义。
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

	// 改密卡住的几种形态，由 PasswdFedAny / TimedOut / PasswdEarlyClose / passwd 退出码
	// 这几个结构事实定性（2026-10-14）。此前靠执行侧编的四句「改密没有完成：…」文案 +
	// 判据表关键词认回来，改措辞即失配。用例按"执行侧真会写出的字段组合"给。
	nonzero := 1
	for _, c := range []struct {
		name      string
		fedAny    bool
		timedOut  bool
		earlyClse bool
		passwdBad bool
		want      string
	}{
		{"没喂出去、到点 → 没等到提示", false, true, false, false, "没等到提示"},
		{"喂出去了、到点 → 远端无响应", true, true, false, false, "远端无响应"},
		{"早收场 → 改密未完成", false, false, true, false, "改密未完成"},
		{"没喂出去、passwd 非 0 收场 → 提示词没认出", false, false, false, true, "提示词没认出"},
		{"喂出去了、passwd 非 0 收场 → 改密未完成", true, false, false, true, "改密未完成"},
	} {
		res = ssh.Result{
			ConnectSuccess: true, SessionBegun: true, EnterSignalSeen: true,
			PasswdFedAny: c.fedAny, TimedOut: c.timedOut, PasswdEarlyClose: c.earlyClse,
		}
		if c.passwdBad {
			res.Steps = []ssh.StepResult{{Name: ssh.StepPasswd, ExitCode: &nonzero}}
		}
		Judge(&res, cli.ModePasswd, kw)
		if res.Category != c.want {
			t.Fatalf("%s：应得 %q，实为 %q", c.name, c.want, res.Category)
		}
		if res.Verdict != Other {
			t.Fatalf("%s：应判失败，实为 %q", c.name, res.Verdict)
		}
	}

	// 干净收场：喂过、没异常、passwd 退出码 0 → 成功（不被上面那些兜底形态误伤）
	res = ssh.Result{
		ConnectSuccess: true, SessionBegun: true, EnterSignalSeen: true, PasswdFedAny: true,
		Steps: []ssh.StepResult{{Name: ssh.StepPasswd, ExitCode: &zero}},
	}
	Judge(&res, cli.ModePasswd, kw)
	if res.Verdict != Success {
		t.Fatalf("改密干净收场应判成功，实为 %+v", res)
	}

	// 次序：**超时优先于远端原话**。超时时 err 上带的是「命令执行超时(Ns)」这类通用描述，
	// 远端在此之前说的半句原因不能拿来做定论——原执行侧也是这个次序（endFactsOf 注释有记）。
	res = ssh.Result{
		ConnectSuccess: true, SessionBegun: true, EnterSignalSeen: true,
		PasswdFedAny: true, TimedOut: true,
		Error: strPtrOf("passwd：密码未更改。"),
	}
	Judge(&res, cli.ModePasswd, kw)
	if res.Category != "远端无响应" {
		t.Fatalf("超时应盖过远端原话，得 %q", res.Category)
	}

	// 没有超时/早收场时，远端当场说了原因才让位给判据表：分类按它的措辞走，不被兜底形态盖掉。
	res = ssh.Result{
		ConnectSuccess: true, SessionBegun: true, EnterSignalSeen: true,
		PasswdFedAny: true,
		Error:        strPtrOf("passwd：密码未更改。"),
		Steps:        []ssh.StepResult{{Name: ssh.StepPasswd, ExitCode: &one}},
	}
	Judge(&res, cli.ModePasswd, kw)
	if res.Category == "改密未完成" || res.Category == "提示词没认出" {
		t.Fatalf("远端说了原因时不该被兜底形态盖掉，实为 %q", res.Category)
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

// 外部中断按"中断在哪一步"分两档（用户 2026-10-08 真机：Ctrl+C 时日志里混进两种结果——
// 还在建连的报 `建立连接失败 - dial tcp …: operation was canceled`，不在关键词表里，
// 每台一个分类把汇总刷屏；已连上的报 `context canceled`，归了「任务已取消」）。
// 这里走的是结构事实（ssh 侧写下的 Canceled + 连没连上），不看文案。
func TestCanceledSplitsByPhase(t *testing.T) {
	kw := loadKeywords(t)

	// 还没连上就被中断 → 「建连被取消」，而且必须是登记过的分类
	// （否则汇总里会按"报错原文"单排一行——正是这次的病）
	connErr := "建立连接失败 - dial tcp 10.0.0.1:22: operation was canceled"
	res := ssh.Result{ConnectSuccess: false, Canceled: true, Error: &connErr}
	Judge(&res, cli.ModeCommand, kw)
	if res.Category != "建连被取消" {
		t.Fatalf("建连阶段被中断应归「建连被取消」，实为 %q", res.Category)
	}
	if IsFallbackCategory(res.Category, kw) {
		t.Fatalf("「建连被取消」必须登记在关键词表里，实为兜底原文：%q", res.Category)
	}

	// 已连上、跑到中途被中断 → 「任务已取消」
	canceled := "context canceled"
	res = ssh.Result{ConnectSuccess: true, SessionBegun: true, Canceled: true, Error: &canceled}
	Judge(&res, cli.ModeCommand, kw)
	if res.Category != "任务已取消" {
		t.Fatalf("执行阶段被中断应归「任务已取消」，实为 %q", res.Category)
	}

	// 中止词（密码过期）比"被中断"更具体，不许被盖掉
	res = ssh.Result{
		ConnectSuccess: true, SessionBegun: true, Canceled: true,
		AbortLine: "WARNING: Your password has expired.",
	}
	Judge(&res, cli.ModeCommand, kw)
	if res.Category != "密码过期" {
		t.Fatalf("中止词命中时应保留「密码过期」，实为 %q", res.Category)
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
