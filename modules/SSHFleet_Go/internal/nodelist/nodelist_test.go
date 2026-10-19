package nodelist

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet/internal/cli"
	"sshfleet/internal/common"
	"sshfleet/internal/config"
	"sshfleet/internal/credential"
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
	cfg.Credential.Encrypt = false
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

// 表头识别提示走注入的输出流（交互模式）。
// 非交互模式下这类提示会被静默闸门挡掉，那一路由 common 的测试覆盖。
func TestCSVHeaderNoticeGoesToInteractor(t *testing.T) {
	csv := "ip,port,user,password\n10.0.0.1,22,root,pw.txt\n"
	in, out := newTestInteractor("", false)

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

// UTF-8 BOM 落在首行首列，不清掉会连累 IP 校验：多行清单把首行当表头扔掉
// （静默丢一台），单行清单整份解析为空。中文 Windows 上 Excel 另存的
// "CSV UTF-8" 默认就带这个头。
func TestReadCSVRowsStripsBOM(t *testing.T) {
	in, _ := newTestInteractor("", true)

	rows, err := readCSVRows("\ufeff10.0.0.1,22,root,pw\n10.0.0.2,22,root,pw\n", true, in)
	if err != nil {
		t.Fatalf("带 BOM 的清单不应报错：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("带 BOM 不应丢行，应剩 2 行，实际 %d 行：%v", len(rows), rows)
	}
	if rows[0][0] != "10.0.0.1" {
		t.Fatalf("首行首列的 BOM 应被剥掉，实际 %q", rows[0][0])
	}

	// 单行带 BOM：同样要能解析出节点
	if rows, err := readCSVRows("\ufeff10.0.0.1,22,root,pw\n", true, in); err != nil || len(rows) != 1 {
		t.Fatalf("单行带 BOM 应解析出 1 台，实际 %v（%v）", rows, err)
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
}

// ---------------------------------------------------------------------------
// 以下为补齐的用例。
//
// 分工：readCSVRows 直接测读取与清洗；凡是要看「清单行 → 节点信息」的，一律经
// Read 这个公开入口断言——它是 main 唯一的调用口子，precheckCredentials 也在它之下，
// 这样凭据预检与字段补全的产出都能在同一个可观察面上核对，不必对着内部下标写断言。
// ---------------------------------------------------------------------------

// mustIn 造一个只收输出、非交互的交互器（大多数用例只关心有没有输出，不关心内容）。
func mustIn() *common.Interactor {
	return &common.Interactor{In: strings.NewReader(""), Out: &strings.Builder{}, Disinteractive: true}
}

// readCSVRows 的两条来源：文件按原样读，内联先把 `;;` 换成分行符。
// 这条分叉只在本函数里，补完之后两者走同一条解析路径（common.ReadCSVRows）。
func TestReadCSVRowsSources(t *testing.T) {
	// 文件：内容原样进来，`\n` 是两个普通字符（不做转义展开）
	dir := t.TempDir()
	file := filepath.Join(dir, "nodes.csv")
	if err := os.WriteFile(file, []byte("10.0.0.1,22,root,pw\n10.0.0.2,22,root,pw\n"), 0o600); err != nil {
		t.Fatalf("写测试清单失败：%v", err)
	}
	rows, err := readCSVRows(file, false, mustIn())
	if err != nil {
		t.Fatalf("读文件清单失败：%v", err)
	}
	if len(rows) != 2 || rows[0][0] != "10.0.0.1" || rows[1][0] != "10.0.0.2" {
		t.Fatalf("文件清单应读出 2 行，实际 %v", rows)
	}

	// 内联：命令行传不进换行，只能写 `;;`，由装载阶段换成分行符
	rows, err = readCSVRows(`10.0.0.1,22,root,pw;;10.0.0.2,22,root,pw`, true, mustIn())
	if err != nil {
		t.Fatalf("读内联清单失败：%v", err)
	}
	if len(rows) != 2 || rows[0][0] != "10.0.0.1" || rows[1][0] != "10.0.0.2" {
		t.Fatalf("内联的 ;; 应分成两行、读出 2 行，实际 %v", rows)
	}

	// 内联单行：最常见的写法，不带任何 `;;`
	rows, err = readCSVRows("10.0.0.1,22,root,pw", true, mustIn())
	if err != nil || len(rows) != 1 || rows[0][3] != "pw" {
		t.Fatalf("内联单行应读出 1 行，实际 %v（%v）", rows, err)
	}
}

// 内联清单里的反斜杠是普通字符：Windows 路径常含 `\n`，切开就会变成两个节点
// （ADR-0020）。这里用 `;;` 把两台机器连起来，顺带钉住「路径不被当分行」。
func TestReadCSVRowsInlineKeepsBackslashPath(t *testing.T) {
	rows, err := readCSVRows(`10.0.0.1,22,root,D:\new\pw.bin,D:\new\id.pem,D:\nodes\kp.bin;;10.0.0.2,22,root,pw`, true, mustIn())
	if err != nil {
		t.Fatalf("读内联清单失败：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应是 2 台机器，实际 %d 行：%v", len(rows), rows)
	}
	if got := rows[0][3]; got != `D:\new\pw.bin` {
		t.Fatalf("第 4 列的路径应原样保留，实际 %q", got)
	}
	// 第 5、6 列同样是本地路径，一样不能被切
	if got := rows[0][4]; got != `D:\new\id.pem` {
		t.Fatalf("第 5 列的路径应原样保留，实际 %q", got)
	}
	if got := rows[0][5]; got != `D:\nodes\kp.bin` {
		t.Fatalf("第 6 列的路径应原样保留，实际 %q", got)
	}
}

// 文件读不到：报错要带上路径，且不是「未解析出节点」那句（两者病因不同，不能混）。
func TestReadCSVRowsFileMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.csv")
	_, err := readCSVRows(missing, false, mustIn())
	if err == nil {
		t.Fatal("清单文件不存在时应报错")
	}
	msg := err.Error()
	if !strings.Contains(msg, "读取内容时发生错误") || !strings.Contains(msg, "nope.csv") {
		t.Fatalf("应报读取错误并带上路径，实际：%s", msg)
	}
}

// CSV 语法错（双引号不成对）：不把 Go 的 ParseError 行话抛给用户，
// 改成「哪一行 + 引号要成对」两句，来源（文件 / 内联）分别指出。
func TestReadCSVRowsSyntaxError(t *testing.T) {
	dir := t.TempDir()
	badFile := filepath.Join(dir, "bad.csv")
	// 第 2 行引号没闭合：ParseError 报到文件里那一行
	if err := os.WriteFile(badFile, []byte("10.0.0.1,22,root,pw\n10.0.0.2,22,root,\"没闭合\n"), 0o600); err != nil {
		t.Fatalf("写测试清单失败：%v", err)
	}

	_, err := readCSVRows(badFile, false, mustIn())
	if err == nil {
		t.Fatal("引号不成对时应报错")
	}
	msg := err.Error()
	for _, want := range []string{"读取节点清单失败", "bad.csv", "第 2 行", "双引号要成对出现"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("文件来源的语法错文案缺少 %q：\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "ParseError") || strings.Contains(msg, "bare \"") {
		t.Fatalf("不应把 Go 的行话抛给用户：\n%s", msg)
	}

	// 内联来源：报的是「内联节点信息的第 N 行」，不是某个文件路径
	_, err = readCSVRows("10.0.0.1,22,root,\"没闭合\n", true, mustIn())
	if err == nil {
		t.Fatal("内联引号不成对时应报错")
	}
	if msg := err.Error(); !strings.Contains(msg, "内联节点信息的第 1 行") {
		t.Fatalf("内联来源的语法错应说「内联节点信息的第 N 行」，实际：%s", msg)
	}
}

// 表头识别只认「首行首列不是 IPv4 且含逗号」这一条：
// 含逗号 → 当表头移除；不含逗号 → 判为无有效节点，不静默丢那一行。
func TestReadCSVRowsHeaderDetection(t *testing.T) {
	// 形态一：标准表头
	rows, err := readCSVRows("ip,port,user,password\n10.0.0.1,22,root,pw\n", true, mustIn())
	if err != nil || len(rows) != 1 || rows[0][0] != "10.0.0.1" {
		t.Fatalf("标准表头应被移除，实际 %v（%v）", rows, err)
	}

	// 形态二：只有表头，去掉之后没有有效行了 —— 报「未解析出节点」而不是返回空清单
	if _, err := readCSVRows("ip,port,user,password\n", true, mustIn()); err == nil ||
		!strings.Contains(err.Error(), "未解析出任何有效节点") {
		t.Fatalf("只剩表头时应报无有效节点，实际：%v", err)
	}

	// 形态三：首行不是 IP 又不含逗号（一列垃圾）—— 不能当表头吃掉，直接报错
	// （单列内容若被当表头，用户看到的是「文件是空的」，与真因对不上）
	if _, err := readCSVRows("垃圾内容\n10.0.0.1,22,root,pw\n", true, mustIn()); err == nil ||
		!strings.Contains(err.Error(), "未解析出任何有效节点") {
		t.Fatalf("单列非 IP 首行应报无有效节点，实际：%v", err)
	}

	// 形态四：首行就是合法 IP —— 不是表头，照常参与解析（多行清单一台都不能丢）
	rows, err = readCSVRows("10.0.0.1,22,root,pw\n10.0.0.2,22,root,pw\n", true, mustIn())
	if err != nil || len(rows) != 2 {
		t.Fatalf("首行是合法 IP 时不该当表头，实际 %v（%v）", rows, err)
	}
}

// 表头提示是教学类提示：交互模式上屏，--yes 下由静默闸门挡掉。
func TestReadCSVRowsHeaderNoticeRespectsGate(t *testing.T) {
	const text = "ip,port,user,password\n10.0.0.1,22,root,pw\n"

	in := mustIn() // Disinteractive = true
	if _, err := readCSVRows(text, true, in); err != nil {
		t.Fatalf("非交互下读表头清单失败：%v", err)
	}
	if got := in.Out.(*strings.Builder).String(); got != "" {
		t.Fatalf("非交互（--yes）下不该有表头提示，实际：%q", got)
	}
}

// 空内容与「只有注释 / 空行」：都归到同一句「未解析出任何有效节点」。
func TestReadCSVRowsEmptyContent(t *testing.T) {
	for _, text := range []string{"", "\n\n", "# 只有注释\n", "   \n"} {
		if _, err := readCSVRows(text, true, mustIn()); err == nil ||
			!strings.Contains(err.Error(), "未解析出任何有效节点") {
			t.Fatalf("内容 %q 应报无有效节点，实际：%v", text, err)
		}
	}
}

// CSV 口径（双引号包住逗号、变长列、CRLF）在本模块与代填侧一致——
// 这里钉住「读进来的行不会被二次加工」，列内容原样交给字段补全。
func TestReadCSVRowsKeepsColumnContent(t *testing.T) {
	// 引号里包住逗号：值里的逗号不能当分列符
	rows, err := readCSVRows(`10.0.0.1,22,root,"pw,with,comma"`, true, mustIn())
	if err != nil {
		t.Fatalf("引号含逗号应能解析：%v", err)
	}
	if len(rows) != 1 || rows[0][3] != "pw,with,comma" {
		t.Fatalf("引号里的逗号应属于同一个值，实际 %v", rows)
	}

	// 变长列：列数不限，缺的列由 padRow 补、多的列由 padRow 截（此处只确认读取层不报错）
	rows, err = readCSVRows("10.0.0.1,22\n10.0.0.2,22,root,pw,key,pass,extra\n", true, mustIn())
	if err != nil || len(rows) != 2 || len(rows[0]) != 2 || len(rows[1]) != 7 {
		t.Fatalf("变长列应原样读出，实际 %v（%v）", rows, err)
	}

	// CRLF：Windows 上另存的 CSV，行尾 \r 不能留在最后一个字段里
	rows, err = readCSVRows("10.0.0.1,22,root,pw\r\n10.0.0.2,22,root,pw\r\n", true, mustIn())
	if err != nil || len(rows) != 2 {
		t.Fatalf("CRLF 清单应能解析出 2 行，实际 %v（%v）", rows, err)
	}
	if rows[0][3] != "pw" {
		t.Fatalf("行尾 \\r 应被剥掉，实际末列 %q", rows[0][3])
	}
}

// ---------------------------------------------------------------------------
// Read：主干第 6 步的公开入口（main 唯一调用口子）
//
// 凭据预检没有独立的公开口子，它是 Read 的必经环节；这里一律经 Read 断言它产出的
// 可观察结果（节点里的明文密码、错误汇总文案、提示上屏与否），不另开测内部的旁路。
// ---------------------------------------------------------------------------

// testKey 测试用的主密钥：任何非空串都可用，取一个长度够的固定值。
func testKey() string {
	return base64.RawURLEncoding.EncodeToString([]byte("012345678901234567890123456789012345"))
}

// testRead 造 Read 的入参：参数与配置、一个可断言的交互器。
// 非交互（--yes）默认打开——大多数用例只关心结果，提示由专测提示的用例去开。
func testRead(t *testing.T, disinteractive bool, argv ...string) (*cli.Args, *config.Config, *common.Interactor, *strings.Builder) {
	t.Helper()
	args, cfg := testArgsCfg(t, argv...)
	out := &strings.Builder{}
	in := &common.Interactor{
		In:             strings.NewReader(""),
		Out:            out,
		Disinteractive: disinteractive,
	}
	return args, cfg, in, out
}

// writeSecret 把一段密文写进临时凭据文件，返回路径。
// 打开加密时，清单的密码 / 口令列填的就是这样一个文件的绝对路径。
func writeSecret(t *testing.T, dir, name, plain, key string) string {
	t.Helper()
	token, err := credential.EncryptV2(plain, key)
	if err != nil {
		t.Fatalf("造密文失败：%v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		t.Fatalf("写凭据文件失败：%v", err)
	}
	return path
}

// writePEM 造一个能过 PEM 校验（只查 -----BEGIN 头）的私钥文件，返回路径与内容。
func writePEM(t *testing.T, dir, name string) (string, string) {
	t.Helper()
	content := "-----BEGIN OPENSSH PRIVATE KEY-----\nAAAABBBBCCCC\n-----END OPENSSH PRIVATE KEY-----"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写私钥文件失败：%v", err)
	}
	return path, content
}

// 端到端纵切：加密模式下，行内凭据 / 配置默认密码 / 配置默认账号端口三条补全同时生效。
// 这是使用者实际走的那条路——清单列有值用清单的，没值落配置，密码经密文文件解密取明文。
func TestReadEndToEndEncrypted(t *testing.T) {
	key := testKey()
	t.Setenv("SSHFLEET_KEY", key)
	dir := t.TempDir()

	pwFile := writeSecret(t, dir, "pw.bin", "行内明文密码", key)
	defFile := writeSecret(t, dir, "def.bin", "配置默认密码", key)

	// 第 1 行四项齐全；第 2 行只写 IP，其余落配置
	inline := "10.0.0.1,2200,deploy," + pwFile + "\n10.0.0.2,,,"
	args, cfg, in, out := testRead(t, true, "-f", inline, "-c", "echo hi")
	cfg.Credential.Encrypt = true
	cfg.Credential.SecretDir = dir
	cfg.Account.Password = defFile
	cfg.Account.Port = 10022
	cfg.Account.User = "jx_zyc"

	nodes, err := Read(args, cfg, in)
	if err != nil {
		t.Fatalf("读取应成功：%v", err)
	}
	if nodes.Len() != 2 {
		t.Fatalf("应读出 2 台，实际 %d", nodes.Len())
	}

	// 第 1 行：清单列的值原样生效，密码是解密后的明文（不是文件路径）
	first := nodes.Items[0]
	if first.IP != "10.0.0.1" || first.Port != 2200 || first.User != "deploy" {
		t.Fatalf("第 1 行应取清单列的值，实际 %+v", first)
	}
	if first.Password != "行内明文密码" {
		t.Fatalf("第 1 行密码应是解密后的明文，实际 %q", first.Password)
	}

	// 第 2 行：缺的列一律落配置
	second := nodes.Items[1]
	if second.IP != "10.0.0.2" || second.Port != 10022 || second.User != "jx_zyc" {
		t.Fatalf("第 2 行缺列应落配置，实际 %+v", second)
	}
	if second.Password != "配置默认密码" {
		t.Fatalf("第 2 行密码应落配置默认密码，实际 %q", second.Password)
	}

	if out.Len() != 0 {
		t.Fatalf("非交互下不该有提示上屏，实际：%q", out.String())
	}
}

// 同一份密码文件被多行共用：每行都要拿到同一份明文（预检内的去重不影响结果）。
func TestReadSharedPasswordFilePerRow(t *testing.T) {
	key := testKey()
	t.Setenv("SSHFLEET_KEY", key)
	dir := t.TempDir()
	shared := writeSecret(t, dir, "shared.bin", "共用的密码", key)

	inline := "10.0.0.1,22,root," + shared + "\n10.0.0.2,22,root," + shared + "\n10.0.0.3,22,root," + shared
	args, cfg, in, _ := testRead(t, true, "-f", inline, "-c", "echo hi")
	cfg.Credential.Encrypt = true

	nodes, err := Read(args, cfg, in)
	if err != nil {
		t.Fatalf("读取应成功：%v", err)
	}
	if nodes.Len() != 3 {
		t.Fatalf("应读出 3 台，实际 %d", nodes.Len())
	}
	for i, n := range nodes.Items {
		if n.Password != "共用的密码" {
			t.Fatalf("第 %d 台应取到共用密码，实际 %q", i+1, n.Password)
		}
	}
}

// 密钥登录全链路：清单第 5 列的私钥原文进 KeyContent、第 6 列的口令明文进 KeyPassphrase，
// 且持有私钥的节点密码恒为空（不落配置默认密码）。
//
// 注意 -k 下每行的第 5 列都必须写私钥：留空会回落配置 credential.key，两处都空则直接报错——
// 所以混合清单不能靠「第 5 列留空」表示密码行。
func TestReadKeyLoginFromList(t *testing.T) {
	key := testKey()
	t.Setenv("SSHFLEET_KEY", key)
	dir := t.TempDir()
	pwFile := writeSecret(t, dir, "pw.bin", "密码登录的密码", key)
	pemPath, pemContent := writePEM(t, dir, "id.pem")
	passFile := writeSecret(t, dir, "kp.bin", "私钥口令", key)
	defFile := writeSecret(t, dir, "def.bin", "配置默认密码", key)

	// 第 1 行：密码列与私钥列都写了（-k 下按密钥登录）；第 2 行：纯密钥行，无密码列
	inline := "10.0.0.1,22,root," + pwFile + "," + pemPath + "\n" +
		"10.0.0.2,22,root,," + pemPath + "," + passFile
	args, cfg, in, _ := testRead(t, true, "-f", inline, "-c", "echo hi", "-k")
	cfg.Credential.Encrypt = true
	cfg.Credential.SecretDir = dir
	cfg.Account.Password = defFile

	nodes, err := Read(args, cfg, in)
	if err != nil {
		t.Fatalf("读取应成功：%v", err)
	}
	if nodes.Len() != 2 {
		t.Fatalf("应读出 2 台，实际 %d", nodes.Len())
	}

	// 第 1 行：两列都写了，各取各的；第 6 列没写口令，故口令为空
	if nodes.Items[0].Password != "密码登录的密码" {
		t.Fatalf("第 1 行写了密码列，应如实取用，实际 %q", nodes.Items[0].Password)
	}
	if nodes.Items[0].KeyContent != pemContent {
		t.Fatalf("第 1 行应带上私钥，实际 %q", nodes.Items[0].KeyContent)
	}
	if nodes.Items[0].KeyPassphrase != "" {
		t.Fatalf("第 1 行没写第 6 列，口令应留空，实际 %q", nodes.Items[0].KeyPassphrase)
	}

	// 第 2 行是密钥行：私钥原文与口令成对取自同一行，密码恒为空
	second := nodes.Items[1]
	if second.KeyContent != pemContent {
		t.Fatalf("第 2 行应带上私钥原文，实际 %q", second.KeyContent)
	}
	if second.KeyPassphrase != "私钥口令" {
		t.Fatalf("第 2 行口令应取第 6 列的密文文件，实际 %q", second.KeyPassphrase)
	}
	if second.Password != "" {
		t.Fatalf("持有私钥的节点密码必须为空，实际 %q（配置默认密码不该落到这一行）", second.Password)
	}
}

// 统一私钥走配置（清单第 5 列留空）：私钥与口令都取自配置，配置口令只配给这一类节点。
func TestReadKeyLoginFromConfig(t *testing.T) {
	key := testKey()
	t.Setenv("SSHFLEET_KEY", key)
	dir := t.TempDir()
	pwFile := writeSecret(t, dir, "pw.bin", "密码", key)
	pemPath, pemContent := writePEM(t, dir, "id.pem")
	passFile := writeSecret(t, dir, "kp.bin", "配置里的私钥口令", key)

	inline := "10.0.0.1,22,root," + pwFile + "\n10.0.0.2,22,root," + pwFile
	args, cfg, in, _ := testRead(t, true, "-f", inline, "-c", "echo hi", "-k")
	cfg.Credential.Encrypt = true
	cfg.Credential.SecretDir = dir
	cfg.Credential.Key = pemPath
	cfg.Credential.KeyPassword = passFile

	nodes, err := Read(args, cfg, in)
	if err != nil {
		t.Fatalf("读取应成功：%v", err)
	}
	for i, n := range nodes.Items {
		if n.KeyContent != pemContent {
			t.Fatalf("第 %d 台应取配置里的统一私钥，实际 %q", i+1, n.KeyContent)
		}
		if n.KeyPassphrase != "配置里的私钥口令" {
			t.Fatalf("第 %d 台口令应取配置里的值，实际 %q", i+1, n.KeyPassphrase)
		}
	}
}

// 私钥取自配置时，清单第 6 列不参与——口令只与「清单私钥」成对。
// 这是有意为之的归属规则：配置私钥配配置口令，避免两处口令互相串。
func TestReadConfigKeyIgnoresRowPassphrase(t *testing.T) {
	key := testKey()
	t.Setenv("SSHFLEET_KEY", key)
	dir := t.TempDir()
	pwFile := writeSecret(t, dir, "pw.bin", "密码", key)
	pemPath, _ := writePEM(t, dir, "id.pem")
	cfgPass := writeSecret(t, dir, "cfgpw.bin", "配置口令", key)
	rowPass := writeSecret(t, dir, "rowpw.bin", "行内口令", key)

	// 第 6 列写了口令，但私钥来自配置——这一列应被忽略
	inline := "10.0.0.1,22,root," + pwFile + ",," + rowPass
	args, cfg, in, _ := testRead(t, true, "-f", inline, "-c", "echo hi", "-k")
	cfg.Credential.Encrypt = true
	cfg.Credential.SecretDir = dir
	cfg.Credential.Key = pemPath
	cfg.Credential.KeyPassword = cfgPass

	nodes, err := Read(args, cfg, in)
	if err != nil {
		t.Fatalf("读取应成功：%v", err)
	}
	// 两个口令取不同的字面量：只断言「等于配置口令」的话，第 6 列先被读进去、
	// 随后又被配置口令覆盖，两种实现都能通过——必须同时钉住「不等于行内口令」。
	if got := nodes.Items[0].KeyPassphrase; got != "配置口令" {
		t.Fatalf("私钥取自配置时口令应取配置值，实际 %q", got)
	}
	if got := nodes.Items[0].KeyPassphrase; got == "行内口令" {
		t.Fatalf("私钥取自配置时第 6 列不该参与，实际取到了行内口令")
	}

	// 反向确认：配置里没配口令时，第 6 列同样不参与（口令为空，而不是行内口令）
	args, cfg, in, _ = testRead(t, true, "-f", inline, "-c", "echo hi", "-k")
	cfg.Credential.Encrypt = true
	cfg.Credential.SecretDir = dir
	cfg.Credential.Key = pemPath
	cfg.Credential.KeyPassword = ""

	nodes, err = Read(args, cfg, in)
	if err != nil {
		t.Fatalf("配置没配口令时也应读成功：%v", err)
	}
	if got := nodes.Items[0].KeyPassphrase; got != "" {
		t.Fatalf("配置没配口令时口令应为空，实际 %q（第 6 列不该参与）", got)
	}
}

// 第 6 列的口令明文在清单私钥下生效；且该列**永不检查**——
// 工具无法判定私钥是否加密，校验了就是误报，取不到就当没给（真正的缺失由 ssh 侧说清）。
func TestReadRowPassphraseNeverValidated(t *testing.T) {
	key := testKey()
	t.Setenv("SSHFLEET_KEY", key)
	dir := t.TempDir()
	pwFile := writeSecret(t, dir, "pw.bin", "密码", key)
	pemPath, _ := writePEM(t, dir, "id.pem")
	passFile := writeSecret(t, dir, "kp.bin", "行内口令", key)
	missingPass := filepath.Join(dir, "not-exist.bin")

	// 情形一：口令文件存在 → 取到明文
	inline := "10.0.0.1,22,root," + pwFile + "," + pemPath + "," + passFile
	args, cfg, in, _ := testRead(t, true, "-f", inline, "-c", "echo hi", "-k")
	cfg.Credential.Encrypt = true
	cfg.Credential.SecretDir = dir

	nodes, err := Read(args, cfg, in)
	if err != nil {
		t.Fatalf("读取应成功：%v", err)
	}
	if got := nodes.Items[0].KeyPassphrase; got != "行内口令" {
		t.Fatalf("第 6 列的口令应生效，实际 %q", got)
	}

	// 情形二：口令文件根本不存在 → 仍然成功，口令当没给（不报错）
	inline = "10.0.0.1,22,root," + pwFile + "," + pemPath + "," + missingPass
	args, cfg, in, _ = testRead(t, true, "-f", inline, "-c", "echo hi", "-k")
	cfg.Credential.Encrypt = true
	cfg.Credential.SecretDir = dir

	nodes, err = Read(args, cfg, in)
	if err != nil {
		t.Fatalf("第 6 列取不到时不该报错，实际：%v", err)
	}
	if got := nodes.Items[0].KeyPassphrase; got != "" {
		t.Fatalf("取不到口令时应留空，实际 %q", got)
	}
}

// 凭据预检的错误汇总：逐行带行号与 IP，末尾给出统一的「修好再跑」。
// 这里挑三种最常撞上的病因各造一行，确认它们**同时**汇总在一份报告里，不半路退出。
func TestReadPrecheckErrorsAggregated(t *testing.T) {
	key := testKey()
	t.Setenv("SSHFLEET_KEY", key)
	dir := t.TempDir()
	missing := filepath.Join(dir, "not-exist.bin")
	plainFile := filepath.Join(dir, "plain.bin")
	if err := os.WriteFile(plainFile, []byte("就是个明文"), 0o600); err != nil {
		t.Fatalf("写测试文件失败：%v", err)
	}
	badPEM := filepath.Join(dir, "bad.pem")
	if err := os.WriteFile(badPEM, []byte("这不是 PEM"), 0o600); err != nil {
		t.Fatalf("写测试文件失败：%v", err)
	}

	// 行 1：密码文件不存在；行 2：开了加密却不是密文；行 3：私钥不是 PEM
	// （-k 下每行都得有私钥，行 1、2 补一份合规的，免得报的是「找不到私钥」而不是想验的那条）
	goodPEM, _ := writePEM(t, dir, "ok.pem")
	inline := "10.0.0.1,22,root," + missing + "," + goodPEM + "\n" +
		"10.0.0.2,22,root," + plainFile + "," + goodPEM + "\n" +
		"10.0.0.3,22,root," + writeSecret(t, dir, "ok.bin", "密码", key) + "," + badPEM
	args, cfg, in, _ := testRead(t, true, "-f", inline, "-c", "echo hi", "-k")
	cfg.Credential.Encrypt = true

	_, err := Read(args, cfg, in)
	if err == nil {
		t.Fatal("凭据有问题时应报错")
	}
	msg := err.Error()
	for _, want := range []string{
		"CSV凭据预检查失败，共 3 处错误",
		"行 1 (IP: 10.0.0.1): 密码文件不存在",
		"行 2 (IP: 10.0.0.2): 密码文件不是本工具的密文",
		"行 3 (IP: 10.0.0.3): 密钥文件不是有效的PEM格式",
		"请修复上述问题后重新执行",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("错误汇总缺少 %q：\n%s", want, msg)
		}
	}
}

// 密码列为空但配置也没配默认密码：单独一句，不带行号（它不是某一行的问题）。
func TestReadPrecheckDefaultPasswordMissing(t *testing.T) {
	args, cfg, in, _ := testRead(t, true, "-f", "10.0.0.1,22,root,", "-c", "echo hi")
	cfg.Account.Password = ""

	_, err := Read(args, cfg, in)
	if err == nil {
		t.Fatal("密码列空且无默认密码时应报错")
	}
	if !strings.Contains(err.Error(), "密码列有空值，但 config 未配置默认密码") {
		t.Fatalf("应报缺默认密码，实际：%s", err)
	}
}

// 用了 -k 却两处都没有私钥：报错要把出路（第 5 列 / 配置 credential.key）一起说清。
func TestReadPrecheckKeyMissing(t *testing.T) {
	args, cfg, in, _ := testRead(t, true, "-f", "10.0.0.1,22,root,pw", "-c", "echo hi", "-k")

	_, err := Read(args, cfg, in)
	if err == nil {
		t.Fatal("用了 -k 但没有私钥时应报错")
	}
	msg := err.Error()
	for _, want := range []string{"用了 -k，但没有找到可用的私钥", "第 5 列", "credential.key"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("缺私钥的报错应指出路 %q：\n%s", want, msg)
		}
	}
}

// 加密开关刚被改过是常见事故：两个方向的错配各给一句能照做的说明。
func TestReadPrecheckEncryptSwitchMismatch(t *testing.T) {
	key := testKey()
	t.Setenv("SSHFLEET_KEY", key)
	dir := t.TempDir()
	token, err := credential.EncryptV2("密码", key)
	if err != nil {
		t.Fatalf("造密文失败：%v", err)
	}

	// 方向一：没开加密，却读到密文
	args, cfg, in, _ := testRead(t, true, "-f", "10.0.0.1,22,root,"+token, "-c", "echo hi")
	cfg.Credential.Encrypt = false
	if _, err := Read(args, cfg, in); err == nil ||
		!strings.Contains(err.Error(), "读到的是密文，但当前没开加密") {
		t.Fatalf("明文开关读到密文应给出改法，实际：%v", err)
	}

	// 方向二：开了加密，却不是密文
	plainFile := filepath.Join(dir, "plain.bin")
	if err := os.WriteFile(plainFile, []byte("明文密码"), 0o600); err != nil {
		t.Fatalf("写测试文件失败：%v", err)
	}
	args, cfg, in, _ = testRead(t, true, "-f", "10.0.0.1,22,root,"+plainFile, "-c", "echo hi")
	cfg.Credential.Encrypt = true
	if _, err := Read(args, cfg, in); err == nil ||
		!strings.Contains(err.Error(), "不是本工具的密文") {
		t.Fatalf("加密开关读到明文应给出改法，实际：%v", err)
	}
}

// 清单里的相对路径拼 secret_dir，但 secret_dir 没配。
//
// 两列的落点不同，值得分开钉住：
//   - 密码 / 口令列经 credentialValueSpec，「拼不了就原样交给读取层」，最后报的是「文件不存在」；
//   - 私钥列直接调 resolveCredentialPath，拼不了就**当场**报「secret_dir 未配置」——
//     这句才是给得出路的那句，不能只让用户看到「密钥文件不存在 → id.pem」而去猜目录配在哪。
func TestReadPrecheckRelativePathNeedsSecretDir(t *testing.T) {
	// 密码列：拼不上时按原样当路径找，落到「密码文件不存在」
	args, cfg, in, _ := testRead(t, true, "-f", "10.0.0.1,22,root,pw.bin", "-c", "echo hi")
	cfg.Credential.Encrypt = true
	cfg.Credential.SecretDir = ""

	_, err := Read(args, cfg, in)
	if err == nil {
		t.Fatal("相对路径且未配 secret_dir 时应报错")
	}
	if !strings.Contains(err.Error(), "密码文件不存在") {
		t.Fatalf("密码列应报文件不存在，实际：%s", err)
	}

	// 私钥列：当场报「secret_dir 未配置」，把出路说清。
	// 密码列给一个**绝对路径**的合规密文文件，只是为了让预检走到私钥那一列——
	// 密码列先报错会把它挡住，那样验到的还是密码列。
	key := testKey()
	t.Setenv("SSHFLEET_KEY", key)
	absPw := writeSecret(t, t.TempDir(), "abs.bin", "密码", key)
	args, cfg, in, _ = testRead(t, true, "-f", "10.0.0.1,22,root,"+absPw+",id.pem", "-c", "echo hi", "-k")
	cfg.Credential.Encrypt = true
	cfg.Credential.SecretDir = ""

	_, err = Read(args, cfg, in)
	if err == nil {
		t.Fatal("私钥列为相对路径且未配 secret_dir 时应报错")
	}
	if !strings.Contains(err.Error(), "凭据列包含相对路径，但 secret_dir 未配置") {
		t.Fatalf("私钥列应报 secret_dir 未配置，实际：%s", err)
	}

	// 配了 secret_dir：相对路径拼上去，文件都在，应能读通
	dir := t.TempDir()
	pwName := "pw.bin"
	pemName := "id.pem"
	writeSecret(t, dir, pwName, "拼出来的密码", key)
	writePEM(t, dir, pemName)

	args, cfg, in, _ = testRead(t, true, "-f", "10.0.0.1,22,root,"+pwName+","+pemName, "-c", "echo hi", "-k")
	cfg.Credential.Encrypt = true
	cfg.Credential.SecretDir = dir

	nodes, err := Read(args, cfg, in)
	if err != nil {
		t.Fatalf("配了 secret_dir 后相对路径应能拼出来：%v", err)
	}
	if nodes.Items[0].Password != "拼出来的密码" {
		t.Fatalf("相对路径应拼到 secret_dir 下，实际密码 %q", nodes.Items[0].Password)
	}
}

// 明文模式（encrypt = false）：密码列就是密码本身，不走文件、不需要主密钥。
// 这是出厂之外最常用的一种配法，必须能独立跑通。
func TestReadPlaintextModeNeedsNoMasterKey(t *testing.T) {
	t.Setenv("SSHFLEET_KEY", "") // 明文模式不该碰主密钥
	args, cfg, in, _ := testRead(t, true, "-f", "10.0.0.1,2200,deploy,直接就是密码", "-c", "echo hi")
	cfg.Credential.Encrypt = false

	nodes, err := Read(args, cfg, in)
	if err != nil {
		t.Fatalf("明文模式读取应成功（不需要主密钥）：%v", err)
	}
	if nodes.Items[0].Password != "直接就是密码" {
		t.Fatalf("明文模式的密码应原样取用，实际 %q", nodes.Items[0].Password)
	}
}

// 读取失败时不得返回半个清单：调用方（main）按「拿到非 nil 才继续」处理，
// 返回部分结果会让报错之后的流程多跑一段。
func TestReadReturnsNoNodesOnError(t *testing.T) {
	args, cfg, in, _ := testRead(t, true, "-f", "10.0.0.1,22,root,pw\n这不是IP,22,root,pw", "-c", "echo hi")
	cfg.Credential.Encrypt = false

	nodes, err := Read(args, cfg, in)
	if err == nil {
		t.Fatal("有非法 IP 时应报错")
	}
	if nodes != nil {
		t.Fatalf("报错时不该返回节点集合，实际 %+v", nodes)
	}
}

// Nodes.Len 与 Items 同源：调用方各处都用它取台数。
func TestNodesLen(t *testing.T) {
	if n := (&Nodes{}).Len(); n != 0 {
		t.Fatalf("空集合应为 0，实际 %d", n)
	}
	n := &Nodes{Items: []NodeInfo{{IP: "10.0.0.1"}, {IP: "10.0.0.2"}}}
	if n.Len() != 2 {
		t.Fatalf("应报 2 台，实际 %d", n.Len())
	}
}

// ---------------------------------------------------------------------------
// unusedKeyNotice：清单里配了私钥却没加 -k 时的提示文案
// ---------------------------------------------------------------------------

// 提示文案的三个分支：一台、几台（列全部）、超过 5 台（只列前 5 并报总数）。
func TestUnusedKeyNoticeWording(t *testing.T) {
	cases := []struct {
		name string
		rows []int
		want string
	}{
		{"没有这种情况：空串，调用方据此不上屏", nil, ""},
		{"一台：用「这一台」", []int{3},
			"[提示] 清单第 3 行配了私钥，本次没加 -k，这一台会用密码登录\n"},
		{"两台：用「这几台」", []int{3, 7},
			"[提示] 清单第 3、7 行配了私钥，本次没加 -k，这几台会用密码登录\n"},
		{"正好 5 台：全部列出", []int{1, 2, 3, 4, 5},
			"[提示] 清单第 1、2、3、4、5 行配了私钥，本次没加 -k，这几台会用密码登录\n"},
		{"6 台：只列前 5 并报总数", []int{1, 2, 3, 4, 5, 6},
			"[提示] 清单共 6 行配了私钥（如第 1、2、3、4、5 行），本次没加 -k，这几台会用密码登录\n"},
		{"上千台：文案长度不随行数增长", []int{2, 4, 6, 8, 10, 12, 14, 16, 18, 20},
			"[提示] 清单共 10 行配了私钥（如第 2、4、6、8、10 行），本次没加 -k，这几台会用密码登录\n"},
	}
	for _, c := range cases {
		pre := &precheckResult{unusedKeyRows: c.rows}
		if got := pre.unusedKeyNotice(); got != c.want {
			t.Fatalf("%s：\n应得 %q\n实际 %q", c.name, c.want, got)
		}
	}
}

// 提示与 -k 是同一件事的两面：加 -k 之后不该再提醒；没加才提醒，且只在交互模式上屏。
func TestUnusedKeyNoticeThroughRead(t *testing.T) {
	key := testKey()
	t.Setenv("SSHFLEET_KEY", key)
	dir := t.TempDir()
	pemPath, _ := writePEM(t, dir, "id.pem")
	pwFile := writeSecret(t, dir, "pw.bin", "密码", key)

	// 没加 -k，交互模式：提示上屏
	inline := "10.0.0.1,22,root," + pwFile + "," + pemPath
	args, cfg, in, out := testRead(t, false, "-f", inline, "-c", "echo hi")
	cfg.Credential.Encrypt = true
	if _, err := Read(args, cfg, in); err != nil {
		t.Fatalf("没加 -k 时应照常按密码登录读下去：%v", err)
	}
	if !strings.Contains(out.String(), "配了私钥，本次没加 -k") {
		t.Fatalf("交互模式下应提示「配了没用上」，实际：%q", out.String())
	}
	if !strings.Contains(out.String(), "第 1 行") {
		t.Fatalf("提示应指出具体行号，实际：%q", out.String())
	}

	// 没加 -k，非交互（--yes）：提示由静默闸门挡掉
	args, cfg, in, out = testRead(t, true, "-f", inline, "-c", "echo hi")
	cfg.Credential.Encrypt = true
	if _, err := Read(args, cfg, in); err != nil {
		t.Fatalf("非交互下也应读成功：%v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("非交互下不该有提示上屏，实际：%q", out.String())
	}

	// 加了 -k：私钥用上了，不再提醒
	args, cfg, in, out = testRead(t, false, "-f", inline, "-c", "echo hi", "-k")
	cfg.Credential.Encrypt = true
	if _, err := Read(args, cfg, in); err != nil {
		t.Fatalf("加了 -k 应读成功：%v", err)
	}
	if strings.Contains(out.String(), "配了私钥，本次没加 -k") {
		t.Fatalf("加了 -k 之后不该再提醒，实际：%q", out.String())
	}
}
