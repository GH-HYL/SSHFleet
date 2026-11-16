package message

import "testing"

// 帮助那一屏的字（自定义区）：期望值照迁移前 `parse/options.go` 里的字面量抄，
// 不拿被测函数再算一遍——这批的判据就是「屏上字节不变」。

// 说明按长选项名取：十九条在表里，`--no-sudo` 单独两版；认不得的名字要报「没有」。
func TestHelpDescByLongName(t *testing.T) {
	cases := map[string]string{
		"--command":         "在多台服务器上执行一条命令",
		"--change-password": "批量改密：把清单里密码过期的账号改成同一个新密码",
		"--csv-file":        "节点清单：CSV 文件路径，或直接写一行节点信息（必填）",
		"--number":          "并发数：同时操作几台，不填=全部并行",
		"--timeout":         "命令跑完、文件传完的最长等待（秒）",
		"--convert-secret":  "转换凭据文件（后面跟要转的文件路径）：按配置里的加密开关，转成明文或密文",
	}
	for long, want := range cases {
		got, ok := HelpDesc(long, false)
		if !ok {
			t.Fatalf("%s 取不到说明", long)
		}
		if got != want {
			t.Fatalf("%s 说明：\n got  %q\n want %q", long, got, want)
		}
	}

	if _, ok := HelpDesc("--no-such-option", false); ok {
		t.Fatal("不认识的选项不该给说明，屏上那一行该被跳过")
	}
}

// 十九条说明各归其位：表里漏一条，`parse` 侧那条判据就会红在「取不到」上。
func TestHelpDescCoversEveryOption(t *testing.T) {
	if len(helpDescs) != 19 {
		t.Fatalf("说明 want 19 条（二十个选项里 --no-sudo 单独两版），got %d", len(helpDescs))
	}
}

// `--no-sudo` 那句随配置两版：sudo 关着是平铺直叙，开着要多一句去路。
func TestHelpDescNoSudoFollowsConfig(t *testing.T) {
	if got, _ := HelpDesc("--no-sudo", false); got != "这次以登录用户身份执行" {
		t.Fatalf("配置关时 --no-sudo：%q", got)
	}
	if got, _ := HelpDesc("--no-sudo", true); got != "这次以登录用户身份执行（配置是开时用它改回来）" {
		t.Fatalf("配置开时 --no-sudo：%q", got)
	}
}

// 分组标题五条，顺序与屏上一致。
func TestHelpGroupTitles(t *testing.T) {
	want := []string{"模式（五选一）", "清单与目标路径", "执行选项", "密钥与凭据", "密钥管理（不执行批量任务）"}
	got := []string{HelpGroupMode, HelpGroupTarget, HelpGroupExec, HelpGroupKey, HelpGroupKeyTool}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 个分组：\n got  %q\n want %q", i+1, got[i], want[i])
		}
	}
}

// 标记列四个方括号：默认值与当前配置。
func TestHelpTags(t *testing.T) {
	if got := HelpTagDefaultAll(); got != "[默认: 全部]" {
		t.Fatalf("--number 标记：%q", got)
	}
	if got := HelpTagDefaultPair(60, 300); got != "[默认: 60/300]" {
		t.Fatalf("--timeout 标记：%q", got)
	}
	if got := HelpTagDefaultSeconds(10); got != "[默认: 10]" {
		t.Fatalf("--connect-timeout 标记：%q", got)
	}
	if got := HelpTagCurrentConfig(true); got != "[当前配置: 开]" {
		t.Fatalf("sudo 开着：%q", got)
	}
	if got := HelpTagCurrentConfig(false); got != "[当前配置: 关]" {
		t.Fatalf("sudo 关着：%q", got)
	}
}
