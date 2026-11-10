package parse

import (
	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"

	"strings"
	"testing"
)

// 读清单那一段：文件与内联两种形态同一条口径（BOM、`#` 注释、空行、变长列、双引号）。

func TestReadManifestFromFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "nodes.csv", "\ufeff# 注释行\n\n10.0.0.1,22,root,pw\n10.0.0.2\n")

	m, fail := readManifest(path)
	if fail != nil {
		t.Fatalf("读不成器：%s", fail.Body)
	}
	if len(m.rows) != 2 {
		t.Fatalf("应当读到两行，拿到 %d 行：%v", len(m.rows), m.rows)
	}
	if strings.TrimSpace(m.rows[0][0]) != "10.0.0.1" {
		t.Fatalf("BOM 或注释行没处理干净：%q", m.rows[0][0])
	}
	if m.inline {
		t.Fatal("给的是文件，不该判成内联")
	}
}

func TestReadManifestFromInlineRecord(t *testing.T) {
	m, fail := readManifest("10.0.0.9,22,root,pw")
	if fail != nil {
		t.Fatalf("读不成器：%s", fail.Body)
	}
	if !m.inline {
		t.Fatal("内联记录应当判成 inline")
	}
	if len(m.rows) != 1 || m.rows[0][0] != "10.0.0.9" {
		t.Fatalf("内联记录读出来的行不对：%v", m.rows)
	}
}

// 首行首列不是 IP 且这一行有逗号：当表头移除，并留一句提示（老版会打出来）。
func TestReadManifestDropsHeader(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "nodes.csv", "ip,port,user,password\n10.0.0.1,22,root,pw\n")

	m, fail := readManifest(path)
	if fail != nil {
		t.Fatalf("读不成器：%s", fail.Body)
	}
	if len(m.rows) != 1 {
		t.Fatalf("表头没被移除：%v", m.rows)
	}
	if !m.headerDropped {
		t.Fatal("移除表头要留一句提示，否则使用者不知道少的那台去哪了")
	}
}

func TestReadManifestRejectsEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "empty.csv", "")

	_, fail := readManifest(path)
	if fail == nil {
		t.Fatal("空文件应当起不来")
	}
	if !strings.Contains(fail.Body, "CSV 文件为空") {
		t.Fatalf("话不对：%s", fail.Body)
	}
}

func TestReadManifestRejectsMissingF(t *testing.T) {
	_, fail := readManifest("")
	if fail == nil {
		t.Fatal("没有 -f 应当起不来")
	}
	if fail.Where != "nodelist" {
		t.Fatalf("环节标签应当是 nodelist，拿到 %q", fail.Where)
	}
	if say := message.CannotStartText(*fail); !strings.Contains(say, "读取内容时发生错误:") {
		t.Fatalf("话不对：%s", say)
	}
}

// 既不是文件、也不像一条记录：报「文件读不到」，并给一条去路。
func TestReadManifestRejectsNeitherFileNorRecord(t *testing.T) {
	_, fail := readManifest("这不是文件也不是记录")
	if fail == nil {
		t.Fatal("应当起不来")
	}
	if !strings.Contains(fail.Body, "-f 参数指定的文件读不到：这不是文件也不是记录") {
		t.Fatalf("话不对：%s", fail.Body)
	}
	if !strings.Contains(fail.Body, "提示：") {
		t.Fatalf("没给去路：%s", fail.Body)
	}
}

func TestReadManifestRejectsNewlineInInline(t *testing.T) {
	_, fail := readManifest("10.0.0.1,22,root,pw\n10.0.0.2")
	if fail == nil {
		t.Fatal("内联值里有换行应当起不来")
	}
	if !strings.Contains(fail.Body, "内联值里不能有换行") {
		t.Fatalf("话不对：%s", fail.Body)
	}
}

// 双引号不配对：说清第几行、引号要成对。
func TestReadManifestReportsSyntaxError(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "bad.csv", "10.0.0.1,\"22,root\n")

	_, fail := readManifest(path)
	if fail == nil {
		t.Fatal("引号不配对应当起不来")
	}
	if !strings.Contains(fail.Body, "CSV 的双引号要成对出现") {
		t.Fatalf("话不对：%s", fail.Body)
	}
}

// —— 逐台补全 ——

func TestBuildNodesFallsBackToConfig(t *testing.T) {
	cfg := &settings{account: account{Port: 10022, User: "jx", Password: "cfg-pw"}}

	nodes, _, fail := buildNodes([][]string{{"10.0.0.1"}}, cfg, false)
	if fail != nil {
		t.Fatalf("不该起不来：%s", fail.Body)
	}
	if len(nodes) != 1 {
		t.Fatalf("应当补出一台：%v", nodes)
	}
	got := nodes[0]
	if got.Port != "10022" || got.User != "jx" || got.Password != "cfg-pw" {
		t.Fatalf("没按配置兜底：%+v", got)
	}
}

func TestBuildNodesPrefersListColumns(t *testing.T) {
	cfg := &settings{account: account{Port: 22, User: "cfg", Password: "cfg-pw"}}

	nodes, _, fail := buildNodes([][]string{{"10.0.0.2", "2222", "root", "list-pw"}}, cfg, false)
	if fail != nil {
		t.Fatalf("不该起不来：%s", fail.Body)
	}
	got := nodes[0]
	if got.Port != "2222" || got.User != "root" || got.Password != "list-pw" {
		t.Fatalf("清单列没优先：%+v", got)
	}
}

// 所有行的毛病一次收齐，不让人改一行跑一次。
func TestBuildNodesCollectsAllErrors(t *testing.T) {
	cfg := &settings{account: account{Port: 22, User: "root", Password: "pw"}}

	_, _, fail := buildNodes([][]string{
		{"10.0.0.1", "0", "root", "pw"},
		{"10.0.0.300", "22", "root", "pw"},
		{"", "22", "root", "pw"},
	}, cfg, false)
	if fail == nil {
		t.Fatal("三行都坏，应当起不来")
	}
	for _, want := range []string{
		"发现以下错误:",
		"行 1 (IP: 10.0.0.1): - 端口必须是1-65535之间的整数",
		"行 2 (IP: 10.0.0.300): - IP格式不正确",
		"行 3 (IP: 空值): - IP必须存在",
	} {
		if !strings.Contains(fail.Body, want) {
			t.Fatalf("少了这一段：%q\n实际：%s", want, fail.Body)
		}
	}
}

// 用了 -k 却没有私钥：按行报出来，并给出两条去路。
func TestBuildNodesNeedsKeyWhenKeyMode(t *testing.T) {
	cfg := &settings{account: account{Port: 22, User: "root", Password: "pw"}}

	_, _, fail := buildNodes([][]string{{"10.0.0.1", "22", "root", "pw"}}, cfg, true)
	if fail == nil {
		t.Fatal("用了 -k 没私钥应当起不来")
	}
	if !strings.Contains(fail.Body, "用了 -k，但没有找到可用的私钥") {
		t.Fatalf("话不对：%s", fail.Body)
	}
}

// 清单里配了私钥却没加 -k：不报错，但留一句提示。
func TestBuildNodesWarnsUnusedKey(t *testing.T) {
	cfg := &settings{account: account{Port: 22, User: "root", Password: "pw"}}

	_, notices, fail := buildNodes([][]string{{"10.0.0.1", "22", "root", "pw", "/k/id_rsa"}}, cfg, false)
	if fail != nil {
		t.Fatalf("加 -k 之外不该起不来：%s", fail.Body)
	}
	// 纸上只装种类与事实：句子由 message 照 Kind 成形（`TestUnusedKeyNotice` 管那句话）。
	if len(notices) != 1 || notices[0].Kind != contract.NoticeUnusedKey {
		t.Fatalf("没提醒「配了私钥没用上」：%v", notices)
	}
	if len(notices[0].Rows) != 1 || notices[0].Rows[0] != 1 {
		t.Fatalf("行号要给对：%v", notices[0].Rows)
	}
}

// 整条解析路径：读配置、读判据表、读清单，最后纸上「对谁做」那一格有值、
// 提示挂在纸上（由塔尖打）、运行上下文封好了。01 号单留下的那条路径级断点在这里合上。
func TestRunFillsNodesAndNotices(t *testing.T) {
	dir := writeTempConfig(t)
	writeFile(t, dir, "nodes.csv", "ip,port,user,password\n10.0.0.1,22,root,pw\n")

	_, plan, _, fail := Run([]string{"sshfleet2", "-f", "nodes.csv", "-c", "uptime"}, "9.9.9")
	if fail != nil {
		t.Fatalf("不该起不来：%s", fail.Body)
	}
	if plan == nil {
		t.Fatal("没交回纸")
	}
	if len(plan.Nodes) != 1 || plan.Nodes[0].IP != "10.0.0.1" {
		t.Fatalf("「对谁做」那一格不对：%+v", plan.Nodes)
	}
	if len(plan.Notices) != 1 {
		t.Fatalf("表头提示没挂到纸上：%+v", plan.Notices)
	}
	if len(plan.RunMeta) == 0 {
		t.Fatal("运行上下文没封")
	}
	if plan.Resources.Manifest != "nodes.csv" {
		t.Fatalf("「执行资源」那一格不对：%q", plan.Resources.Manifest)
	}
}
