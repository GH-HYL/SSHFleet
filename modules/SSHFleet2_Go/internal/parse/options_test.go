package parse

import (
	"strings"
	"testing"

	"sshfleet2/internal/message"
)

// 选项表：二十条、五组、十三条带值；标记列跟着配置走。期望串是从老工程
// cli/args.go 的 helpEntries 与 pflag 登记处抄的字面量——期望值独立于被测代码。

func TestOptionTableShape(t *testing.T) {
	if len(options) != 20 {
		t.Fatalf("选项 want 20 条，got %d", len(options))
	}
	takes := 0
	for _, o := range options {
		if o.takesValue {
			takes++
		}
	}
	if takes != 13 {
		t.Errorf("带值的选项 want 13 条，got %d", takes)
	}
}

func TestHelpEntriesGroups(t *testing.T) {
	entries := helpEntries(execution{Sudo: true, TimeoutConnect: 10, TimeoutExecute: 60, TimeoutTransfer: 300})

	var groups, opts, blanks int
	for _, e := range entries {
		switch e.Kind {
		case message.EntryGroup:
			groups++
		case message.EntryOption:
			opts++
		case message.EntryBlank:
			blanks++
		}
	}
	if groups != 5 || opts != 20 || blanks != 4 {
		t.Errorf("组/选项/空行 want 5/20/4，got %d/%d/%d", groups, opts, blanks)
	}

	// 组标题逐字
	wantGroups := []string{"模式（五选一）", "清单与目标路径", "执行选项", "密钥与凭据", "密钥管理（不执行批量任务）"}
	var got []string
	for _, e := range entries {
		if e.Kind == message.EntryGroup {
			got = append(got, e.Desc)
		}
	}
	if strings.Join(got, "|") != strings.Join(wantGroups, "|") {
		t.Errorf("组标题 want %v，got %v", wantGroups, got)
	}
}

func TestHelpEntriesTagsFollowConfig(t *testing.T) {
	on := helpEntries(execution{Sudo: true, TimeoutConnect: 10, TimeoutExecute: 60, TimeoutTransfer: 300})
	for _, e := range on {
		if e.Long == "--sudo" && e.Tag != "[当前配置: 开]" {
			t.Errorf("配置开时 sudo 标记：%q", e.Tag)
		}
		if e.Long == "--no-sudo" && e.Desc != "这次以登录用户身份执行（配置是开时用它改回来）" {
			t.Errorf("配置开时 no-sudo 说明：%q", e.Desc)
		}
		if e.Long == "--timeout" && e.Tag != "[默认: 60/300]" {
			t.Errorf("timeout 标记：%q", e.Tag)
		}
		if e.Long == "--connect-timeout" && e.Tag != "[默认: 10]" {
			t.Errorf("connect-timeout 标记：%q", e.Tag)
		}
	}
	off := helpEntries(execution{Sudo: false, TimeoutConnect: 10, TimeoutExecute: 60, TimeoutTransfer: 300})
	for _, e := range off {
		if e.Long == "--sudo" && e.Tag != "[当前配置: 关]" {
			t.Errorf("配置关时 sudo 标记：%q", e.Tag)
		}
		if e.Long == "--no-sudo" && e.Desc != "这次以登录用户身份执行" {
			t.Errorf("配置关时 no-sudo 说明：%q", e.Desc)
		}
	}
}

// 说明文字逐字抽查（全量对照表见工单 Comments）：每条都是从老工程抄来的字面量。
func TestHelpEntriesVerbatim(t *testing.T) {
	entries := helpEntries(execution{})
	want := map[string]string{
		"--command":         "在多台服务器上执行一条命令",
		"--change-password": "批量改密：把清单里密码过期的账号改成同一个新密码",
		"--csv-file":        "节点清单：CSV 文件路径，或直接写一行节点信息（必填）",
		"--number":          "并发数：同时操作几台，不填=全部并行",
		"--answer":          "代填：看到触发词就自动填内容（一条写成「代填内容,触发词,触发词」），多条写进 CSV 文件再指向它",
		"--key":             "密钥登录：不写=只用密码；写了=用清单或配置里的私钥",
		"--gen-key":         "生成主密钥（打开加密之后才会用到），密钥存进环境变量 SSHFLEET_KEY",
		"--convert-secret":  "转换凭据文件（后面跟要转的文件路径）：按配置里的加密开关，转成明文或密文",
	}
	got := map[string]string{}
	for _, e := range entries {
		if e.Kind == message.EntryOption {
			got[e.Long] = e.Desc
		}
	}
	for long, desc := range want {
		if got[long] != desc {
			t.Errorf("%s 说明 want %q，got %q", long, desc, got[long])
		}
	}
}

// 选项名对照（短名、长名、带值）逐条对老版 pflag 登记处。
func TestOptionNamesMatchLegacy(t *testing.T) {
	type trio struct {
		short, long string
		val         bool
	}
	wants := []trio{
		{"-c", "--command", true},
		{"-s", "--script", true},
		{"-u", "--upload", true},
		{"-d", "--download", true},
		{"-f", "--csv-file", true},
		{"-p", "--path", true},
		{"", "--sudo", false},
		{"", "--no-sudo", false},
		{"-t", "--timeout", true},
		{"-T", "--connect-timeout", true},
		{"-n", "--number", true},
		{"-r", "--remark", true},
		{"", "--no-shell", false},
		{"", "--yes", false},
		{"", "--change-password", true},
		{"-a", "--answer", true},
		{"-k", "--key", false},
		{"", "--gen-key", false},
		{"", "--key-status", false},
		{"", "--convert-secret", true},
	}
	if len(wants) != len(options) {
		t.Fatalf("条数对不上：want %d，got %d", len(wants), len(options))
	}
	for i, w := range wants {
		if options[i].short != w.short || options[i].long != w.long || options[i].takesValue != w.val {
			t.Errorf("第 %d 条 want %v，got %v", i, w, options[i])
		}
	}
}
