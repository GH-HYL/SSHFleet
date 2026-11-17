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
	AbortLine    string `toml:"abort_line"`
	Output       string `toml:"output"`
	Mode         string `toml:"mode"`
	SuccessFiles int    `toml:"success_files"`
	FailedFiles  int    `toml:"failed_files"`
	Expected     string `toml:"expected"`

	// 结构事实。报文时代它们不再用来"给分类名"，只是把执行侧真会写出的字段组合如实造出来：
	// 少了这些，语料就只能靠 error 里的文本碰巧命中，测不出形态本身。
	SessionBegun     bool `toml:"session_begun"`
	TimedOut         bool `toml:"timed_out"`
	Canceled         bool `toml:"canceled"`
	AnswersExhausted bool `toml:"answers_exhausted"`
	PasswdFedAny     bool `toml:"passwd_fed_any"`
	PasswdEarlyClose bool `toml:"passwd_early_close"`
	EnterSignalSeen  bool `toml:"enter_signal_seen"`
	PasswdExitCode   *int `toml:"passwd_exit_code"`
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
		ConnectSuccess:   true,
		Error:            strPtrOf(c.Error),
		ServerBanner:     c.Banner,
		AbortLine:        c.AbortLine,
		Output:           c.Output,
		SuccessFiles:     c.SuccessFiles,
		FailedFiles:      c.FailedFiles,
		CommandExitCode:  c.ExitCode,
		SessionBegun:     c.ExitCode != nil || c.SessionBegun,
		TimedOut:         c.TimedOut,
		Canceled:         c.Canceled,
		AnswersExhausted: c.AnswersExhausted,
		PasswdFedAny:     c.PasswdFedAny,
		PasswdEarlyClose: c.PasswdEarlyClose,
		EnterSignalSeen:  c.EnterSignalSeen,
	}
	if c.PasswdExitCode != nil {
		res.Steps = []ssh.StepResult{{Name: ssh.StepPasswd, ExitCode: c.PasswdExitCode}}
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
		{"目标机写入失败", false},
		{"执行失败(退出码2)", false},
		{"本地文件错误", false},
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

// TestFallbackCategoryFollowsConfig 兜底判定只看配置登记：由程序按结构事实给出的分类名，
// 登记了就是已知分类，没登记就与任何一段认不出来的报错原文一样，落到兜底那一行。
// 判定包里不再有"这些名字豁免"的名单——那张名单就是"改配置不生效"的来源。
func TestFallbackCategoryFollowsConfig(t *testing.T) {
	cases := []struct {
		category   string
		registered []string
		want       bool
	}{
		{"远端无响应", []string{"密码过期", "远端无响应"}, false},
		{"远端无响应", []string{"密码过期"}, true},
		{"提示词没认出", []string{"提示词没认出"}, false},
		{"本地文件错误", []string{"本地文件错误"}, false},
	}
	for _, c := range cases {
		kw := &Keywords{}
		kw.PrependCategories(categoriesOf(c.registered...))
		if got := IsFallbackCategory(c.category, kw); got != c.want {
			t.Errorf("登记 %v 时 IsFallbackCategory(%q) = %v，期望 %v", c.registered, c.category, got, c.want)
		}
	}
}

// categoriesOf 按名字造一组分类（只有名字、没有关键词：它们永远匹配不上，但登记了就是已知分类）。
func categoriesOf(names ...string) []Category {
	out := make([]Category, 0, len(names))
	for _, n := range names {
		out = append(out, Category{Name: n})
	}
	return out
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

// 判定的几个钉子（result-verdict spec 9.5 的纸面替身）。
//
// 报文时代这里只剩**选组与退出码**这些仍然住在判定层的东西：
// 「给一组布尔 → 期望一个中文分类名」那批已经搬到执行侧的纯函数用例（internal/ssh 的
// shape_test.go），判定层不再按结构字段起名。所以下面凡是要分类名的用例，
// 都按执行侧真会写出的形态**把报文放进原文**再去查表。
func TestJudgeStructuralFacts(t *testing.T) {
	kw := loadKeywords(t)
	zero := 0
	one := 1

	// D1 活证：退出码 0 却计入失败（代填没送完）。失败原因来自报文，不来自退出码。
	res := ssh.Result{
		ConnectSuccess: true, SessionBegun: true, CommandExitCode: &zero,
		Error: strPtrOf("sshfleet.answer.missed: 1 answer not delivered"),
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

	// 上传失败后回滚：报文追加在逐文件失败原因之后，回滚那句说明目标机已清干净；
	// 分类由逐文件那行的报文给（不再是「部分成功」，ADR-0022 让它退役了）；定论退出码空
	res = ssh.Result{
		ConnectSuccess: true, SuccessFiles: 2, FailedFiles: 1,
		Error: strPtrOf("a.log: 上传失败 - sshfleet.transfer.remote_file_failed: disk quota exceeded\n" +
			"sshfleet.transfer.rolled_back: 2 of 2 written files removed"),
	}
	Judge(&res, cli.ModeUpload, kw)
	if res.Category != "上传失败已回滚" || res.ExitCode != nil || res.Verdict != Other {
		t.Fatalf("回滚后的上传失败判定不对，实为 %+v", res)
	}

	// 改密未见过期信号 → 「密码未过期」；`:` 在 Steps、CommandExitCode 恒空
	res = ssh.Result{
		ConnectSuccess: true, SessionBegun: true,
		Error: strPtrOf("sshfleet.passwd.not_expired: no password-expiry prompt seen"),
		Steps: []ssh.StepResult{{Name: ssh.StepPasswd, ExitCode: &zero}},
	}
	Judge(&res, cli.ModePasswd, kw)
	if res.Category != "密码未过期" || res.ExitCode != nil {
		t.Fatalf("改密未过期判定不对，实为 %+v", res)
	}

	// 干净收场：喂过、没异常、passwd 退出码 0、原文里没有报文 → 成功
	res = ssh.Result{
		ConnectSuccess: true, SessionBegun: true, EnterSignalSeen: true, PasswdFedAny: true,
		Steps: []ssh.StepResult{{Name: ssh.StepPasswd, ExitCode: &zero}},
	}
	Judge(&res, cli.ModePasswd, kw)
	if res.Verdict != Success {
		t.Fatalf("改密干净收场应判成功，实为 %+v", res)
	}

	// 干净收场但报错原文里有报文 → 不算成功（报文只在非成功形态下写，两条口径等价）
	res.Error = strPtrOf("sshfleet.passwd.not_expired: no password-expiry prompt seen")
	res.EnterSignalSeen = false
	Judge(&res, cli.ModePasswd, kw)
	if res.Verdict == Success {
		t.Fatalf("原文里有报文时不该判成功，实为 %+v", res)
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

	// 退出码 0 但判失败、第二组又没命中：失败原因不在退出码里，
	// 不许拼出「执行失败(退出码0)」，该按第一组显示原文。
	res = ssh.Result{
		ConnectSuccess: true, SessionBegun: true, CommandExitCode: &zero,
		Error: strPtrOf("sshfleet.answer.missed: 1 answer not delivered"),
	}
	Judge(&res, cli.ModeCommand, kw)
	if res.Category != "触发词未命中" {
		t.Fatalf("退出码 0 的失败不该落到退出码分类，实为 %q", res.Category)
	}

	// 预检退出码进 Steps 的下载失败 → 第一组按报错原文归类（D27 消掉的第二组补丁）
	step := 1
	res = ssh.Result{
		ConnectSuccess: true, Steps: []ssh.StepResult{{Name: "test -e", ExitCode: &step}},
		Error: strPtrOf("sshfleet.transfer.remote_missing: remote path not found: /var/log/x"),
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
