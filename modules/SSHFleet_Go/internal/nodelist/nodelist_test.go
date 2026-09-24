package nodelist

import (
	"strings"
	"testing"

	"sshfleet/internal/cli"
	"sshfleet/internal/common"
	"sshfleet/internal/config"
)

// 清单模块回归。
//
// 这个包此前没有任何测试，却是改动最频繁的地方之一。以下几条是踩过坑的点：
//   - 预检产出与清单行的**行号对齐**（历史坑：预检写 0 基、解析读 1 基，两套下标并行）
//   - 提示走注入的输出流（历史坑：直写 os.Stdout，交互路径无法断言）
//   - CSV 清洗（注释行 / 空行 / 表头识别）

// newTestInteractor 造一个可断言的交互器：输入用给定文本，输出收进缓冲区。
func newTestInteractor(input string, disinteractive bool) (*common.Interactor, *strings.Builder) {
	out := &strings.Builder{}
	return &common.Interactor{
		In:             strings.NewReader(input),
		Out:            out,
		Disinteractive: disinteractive,
	}, out
}

// testArgsCfg 造一份可用的参数与配置。
func testArgsCfg(t *testing.T, argv ...string) (*cli.Args, *config.Config) {
	t.Helper()
	cfg := &config.Config{}
	cfg.Account.Port = 22
	cfg.Account.User = "root"
	cfg.Account.Encrypt = false
	cfg.Execution.Sudo = false
	cfg.Execution.TimeoutConnect = 10
	cfg.Execution.TimeoutExecute = 60
	cfg.Execution.TimeoutTransfer = 300

	args, err := cli.Parse(cfg, "test", argv)
	if err != nil {
		t.Fatalf("解析参数失败（%v）：%v", argv, err)
	}
	return args, cfg
}

// 行号对齐：precheckResult 里第 i 项对应清单第 i 行（0 基存储），
// 而对外取用一律用 1 基行号——rowCreds 必须按这个口径取到**同一行**的凭据。
func TestRowCredsAlignsByLineNumber(t *testing.T) {
	pre := &precheckResult{rows: []rowCreds{
		{passwordPlain: "第一行的密码"}, // 0 基 0 → 行 1
		{passwordPlain: "第二行的密码"}, // 0 基 1 → 行 2
		{passwordPlain: "第三行的密码"}, // 0 基 2 → 行 3
	}}

	for lineNo, want := range map[int]string{
		1: "第一行的密码",
		2: "第二行的密码",
		3: "第三行的密码",
	} {
		if got := pre.rowCreds(lineNo).passwordPlain; got != want {
			t.Fatalf("第 %d 行凭据应取到 %q，实际 %q", lineNo, want, got)
		}
	}

	// 越界只返回零值，不 panic、也不误取邻行
	for _, lineNo := range []int{0, -1, 4, 999} {
		if got := pre.rowCreds(lineNo); got != (rowCreds{}) {
			t.Fatalf("行号 %d 越界应返回零值，实际 %+v", lineNo, got)
		}
	}
}

// 多行清单：每行凭据与解析结果必须一一对应，不得错位。
func TestResolveNodesCredentialsMatchRows(t *testing.T) {
	rows := [][]string{
		{"10.0.0.1", "22", "root", "pw1.txt"},
		{"10.0.0.2", "22", "root", "pw2.txt"},
		{"10.0.0.3", "22", "root", "pw3.txt"},
	}
	pre := &precheckResult{
		rows: []rowCreds{
			{passwordPlain: "密码一"},
			{passwordPlain: "密码二"},
			{passwordPlain: "密码三"},
		},
	}
	_, cfg := testArgsCfg(t, "-f", "x.csv", "-c", "echo hi")

	nodes, err := resolveNodes(rows, pre, cfg)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(nodes) != 3 {
		t.Fatalf("应解析出 3 个节点，实际 %d", len(nodes))
	}
	for i, want := range []struct{ ip, pw string }{
		{"10.0.0.1", "密码一"},
		{"10.0.0.2", "密码二"},
		{"10.0.0.3", "密码三"},
	} {
		if nodes[i].IP != want.ip {
			t.Fatalf("第 %d 个节点 IP 应为 %q，实际 %q", i+1, want.ip, nodes[i].IP)
		}
		if nodes[i].Password != want.pw {
			t.Fatalf("%s 的密码应为 %q，实际 %q（行号错位会表现成这里串行）",
				want.ip, want.pw, nodes[i].Password)
		}
	}
}

// 私钥节点：密码恒为空，且私钥与口令成对取自同一行。
func TestResolveNodesKeyBinding(t *testing.T) {
	rows := [][]string{
		{"10.0.0.1", "22", "root", "pw1.txt"}, // 密码行
		{"10.0.0.2", "22", "root", ""},        // 密钥行（无密码列）
	}
	pre := &precheckResult{rows: []rowCreds{
		{passwordPlain: "密码一"},
		{hasKey: true, keyContent: "PEM-第二行", keyPassRaw: "口令二"},
	}}
	args, cfg := testArgsCfg(t, "-f", "x.csv", "-c", "echo hi", "-k")
	if !args.Key {
		t.Fatal("给了 -k 就该是密钥登录")
	}

	nodes, err := resolveNodes(rows, pre, cfg)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if nodes[0].Password != "密码一" || nodes[0].KeyContent != "" {
		t.Fatalf("第一行应是纯密码节点，实际 %+v", nodes[0])
	}
	if nodes[1].Password != "" {
		t.Fatalf("持有私钥的节点密码必须为空，实际 %q", nodes[1].Password)
	}
	if nodes[1].KeyContent != "PEM-第二行" || nodes[1].KeyPassphrase != "口令二" {
		t.Fatalf("第二行的私钥与口令应同源取用，实际 %+v", nodes[1])
	}
}

// 端口补全不再有交互这条路径：清单列有值就用它（非法直接报错），没值就用配置默认值。
func TestResolvePortNoInteraction(t *testing.T) {
	_, cfg := testArgsCfg(t, "-f", "x.csv", "-c", "echo hi")
	cfg.Account.Port = 22

	rows := [][]string{{"10.0.0.1", "", "root", "pw.txt"}}
	pre := &precheckResult{rows: []rowCreds{{passwordPlain: "pw"}}}
	_, out := newTestInteractor("", true)

	nodes, err := resolveNodes(rows, pre, cfg)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if nodes[0].Port != 22 {
		t.Fatalf("端口为空应落到配置默认值 22，实际 %d", nodes[0].Port)
	}
	if out.Len() != 0 {
		t.Fatalf("不该有任何交互输出，实际：%q", out.String())
	}

	// 清单列里的非法端口直接进错误汇总，不重试
	rows2 := [][]string{{"10.0.0.1", "99999", "root", "pw.txt"}}
	var buf strings.Builder
	if _, err := resolveNodes(rows2, pre, cfg); err == nil ||
		!strings.Contains(err.Error(), "当前值为：99999") {
		t.Fatalf("清单里的非法端口应直接报错，实际：%v（输出：%q）", err, buf.String())
	}
}

// 表头识别提示同样走注入的输出流。
func TestCSVHeaderNoticeGoesToInteractor(t *testing.T) {
	csv := "ip,port,user,password\n10.0.0.1,22,root,pw.txt\n"
	in, out := newTestInteractor("", true)

	rows, err := readCSVRows(csv, true, in)
	if err != nil {
		t.Fatalf("读取内联清单失败：%v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("表头应被移除，剩 1 行，实际 %d 行：%v", len(rows), rows)
	}
	if !strings.Contains(out.String(), "已移除表头行") {
		t.Fatalf("表头提示应写进注入的输出流，实际输出：%q", out.String())
	}
}

// CSV 清洗：注释行与**完全空行**跳过；全无有效行时报错。
//
// 注意只含空白的行（如 "   "）不在此处过滤——它等价于「首列是空白」的一行，
// 留给后续 IP 校验拦下并报「IP必须存在」。这是既有行为，此处用测试把口径钉住，
// 免得后来者以为它该在读取阶段就被丢弃。
func TestReadCSVRowsSkipsCommentsAndBlanks(t *testing.T) {
	csv := "# 这是注释\n\n10.0.0.1,22,root,pw.txt\n10.0.0.2,22,root,pw.txt\n"
	in, _ := newTestInteractor("", true)

	rows, err := readCSVRows(csv, true, in)
	if err != nil {
		t.Fatalf("读取失败：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("注释行与空行应跳过，应剩 2 行，实际 %d 行：%v", len(rows), rows)
	}
	if rows[0][0] != "10.0.0.1" || rows[1][0] != "10.0.0.2" {
		t.Fatalf("清洗后行序不符：%v", rows)
	}

	// 只含空白的行不被读取阶段丢弃，原样保留（交由 IP 校验处理）
	if rows, err := readCSVRows("10.0.0.1,22,root,pw.txt\n   \n", true, in); err != nil || len(rows) != 2 {
		t.Fatalf("只含空白的行应原样保留，实际 %v（%v）", rows, err)
	}

	// 只有注释：无有效节点，明确报错
	if _, err := readCSVRows("# 只有注释\n", true, in); err == nil {
		t.Fatal("无有效节点时应报错")
	}
}

// 端口校验：只接受纯数字，越界与带符号都要拒。
func TestResolvePortValidation(t *testing.T) {
	cases := []struct {
		raw   string
		want  int
		valid bool
	}{
		{"22", 22, true},
		{"65535", 65535, true},
		{"1", 1, true},
		{"0", 0, false},
		{"65536", 0, false},
		{"-1", 0, false},
		{"+22", 0, false},
		{"22.0", 0, false},
		{"abc", 0, false},
	}
	for _, c := range cases {
		got, errs := resolvePort(c.raw, 0)
		if c.valid {
			if len(errs) > 0 || got != c.want {
				t.Fatalf("端口 %q 应通过并得 %d，实际 %d（%v）", c.raw, c.want, got, errs)
			}
			continue
		}
		if len(errs) == 0 {
			t.Fatalf("端口 %q 应被拒，实际通过得 %d", c.raw, got)
		}
	}
}

// 配置默认值参与补全：清单缺列时落到配置值，无需交互。
func TestResolveNodesFallsBackToConfig(t *testing.T) {
	rows := [][]string{{"10.0.0.1", "", "", ""}}
	// 密码列也为空：由预检阶段备好的默认密码兜底（与主流程一致）
	pre := &precheckResult{rows: []rowCreds{{}}, defaultPasswordPlain: "配置默认密码"}
	_, cfg := testArgsCfg(t, "-f", "x.csv", "-c", "echo hi")
	cfg.Account.Port = 2200
	cfg.Account.User = "deploy"
	_, out := newTestInteractor("", true)

	nodes, err := resolveNodes(rows, pre, cfg)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if nodes[0].Port != 2200 || nodes[0].User != "deploy" {
		t.Fatalf("缺列应落到配置默认值，实际 %+v", nodes[0])
	}
	if nodes[0].Password != "配置默认密码" {
		t.Fatalf("密码应落到默认密码，实际 %q", nodes[0].Password)
	}
	if out.Len() != 0 {
		t.Fatalf("有配置默认值时不该有任何交互输出，实际：%q", out.String())
	}
}

// IP 严格校验（D13）：严格 IPv4 字面量，非法行进错误汇总。
func TestParseNodeStrictIPv4(t *testing.T) {
	rows := [][]string{
		{"10.0.0.1", "22", "root", "pw.txt"},
		{"999.1.1.1", "22", "root", "pw.txt"},
		{"", "22", "root", "pw.txt"},
		{"::1", "22", "root", "pw.txt"},
	}
	pre := &precheckResult{rows: []rowCreds{
		{passwordPlain: "pw"}, {passwordPlain: "pw"},
		{passwordPlain: "pw"}, {passwordPlain: "pw"},
	}}
	_, cfg := testArgsCfg(t, "-f", "x.csv", "-c", "echo hi")

	_, err := resolveNodes(rows, pre, cfg)
	if err == nil {
		t.Fatal("存在非法 IP 时应报错")
	}
	msg := err.Error()
	for _, want := range []string{"行 2", "IP格式不正确", "行 3", "IP必须存在"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("错误汇总缺少 %q：\n%s", want, msg)
		}
	}
}

// 缺字段一律报错，不再交互补输入：同样的命令加同样的文件，永远跑出同样的结果。
func TestResolveNodesMissingFieldErrors(t *testing.T) {
	rows := [][]string{
		{"10.0.0.1", "22", "", "pw.txt"}, // 用户名缺，配置也空
		{"10.0.0.2", "22", "root", ""},   // 密码缺，配置也空
	}
	pre := &precheckResult{rows: []rowCreds{
		{passwordPlain: "pw"}, {},
	}}
	_, cfg := testArgsCfg(t, "-f", "x.csv", "-c", "echo hi")
	cfg.Account.User = ""
	cfg.Account.Password = ""

	_, err := resolveNodes(rows, pre, cfg)
	if err == nil {
		t.Fatal("用户名与密码都无处可补，应报错")
	}
	msg := err.Error()
	for _, want := range []string{"行 1", "用户名为空", "行 2", "密码为空"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("错误汇总缺少 %q：\n%s", want, msg)
		}
	}
}

// resolvePassword 的三级回落：行内密码 > 有私钥则空 > 配置默认密码。
func TestResolvePasswordFallbacks(t *testing.T) {
	pre := &precheckResult{
		rows:                 []rowCreds{{passwordPlain: "行内密码"}, {hasKey: true}, {}},
		defaultPasswordPlain: "配置默认密码",
	}
	if pw, err := resolvePassword(pre.rowCreds(1), pre); err != nil || pw != "行内密码" {
		t.Fatalf("第 1 行应取行内密码，实际 %q（%v）", pw, err)
	}
	if pw, err := resolvePassword(pre.rowCreds(2), pre); err != nil || pw != "" {
		t.Fatalf("第 2 行持有私钥，密码应为空，实际 %q（%v）", pw, err)
	}
	if pw, err := resolvePassword(pre.rowCreds(3), pre); err != nil || pw != "配置默认密码" {
		t.Fatalf("第 3 行应取配置默认密码，实际 %q（%v）", pw, err)
	}
	// 配置默认密码也没有：报错，不询问
	if _, err := resolvePassword(rowCreds{}, &precheckResult{}); err == nil {
		t.Fatal("三级都空时应报错")
	}
}

// 工具函数：短行补齐与判定。
func TestHelpers(t *testing.T) {
	if got := padRow([]string{"a", "b"}); len(got) != 6 || got[0] != "a" || got[2] != "" {
		t.Fatalf("padRow 应补齐到 6 列，实际 %v", got)
	}
	if got := padRow([]string{"a", "b", "c", "d", "e", "f", "g"}); len(got) != 6 {
		t.Fatalf("padRow 应截到 6 列，实际 %v", got)
	}

	if !isAllDigits("0123") || isAllDigits("") || isAllDigits("1a") || isAllDigits("-1") {
		t.Fatal("isAllDigits 判定不合预期")
	}
	if !isStrictIPv4(" 192.168.1.1 ") || isStrictIPv4("256.1.1.1") || isStrictIPv4("::1") {
		t.Fatal("isStrictIPv4 判定不合预期")
	}
}
