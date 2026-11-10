package execute

// 分类判据的自定义区测试：喂手搓的原文，断言分类名。
//
// 期望值一律写死字面量，不用被测代码算——这些名字是随包 `config/keywords_error.conf` 里
// 那两类规则与老版 `verdict` 的口径给出来的，不是「跑一遍看看出来什么」。

import (
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// classifyTables 手搓一份判据表，形状照随包配置：两组、块内自上而下、关键词里带一个星号。
func classifyTables() contract.Tables {
	return contract.Tables{
		Categories: []contract.Category{
			{Name: "连接失败", Keywords: []string{"sshfleet.connect."}},
			{Name: "认证失败", Keywords: []string{"permission denied"}},
			{Name: "执行超时", Keywords: []string{"sshfleet.run.timeout"}},
			{Name: "命令失败", Keywords: []string{"ssh: command * failed"}},
			{Name: "代填中止", Keywords: []string{"sshfleet.answer."}},
		},
		ExitCategories: []contract.Category{
			{Name: "密码过期", Keywords: []string{"password change required", "your account has expired"}},
			{Name: "会话被拒", Keywords: []string{"sshfleet.run.rejected"}},
			// 和第一组同名不同判据：用来验「同一段文本，有没有退出码决定去哪一组」。
			{Name: "连接失败", Keywords: []string{"connection reset by peer"}},
		},
	}
}

func intPtr(v int) *int { return &v }

// TestCategoryOfCommandMode 每类一条原文，分类名逐条写死。
func TestCategoryOfCommandMode(t *testing.T) {
	tables := classifyTables()

	cases := []struct {
		name string
		res  contract.NodeResult
		want string
	}{
		{
			name: "干净走完是成功那一类",
			res:  contract.NodeResult{Connected: true, ExitCode: intPtr(0)},
			want: "执行成功",
		},
		{
			name: "连不上：没有退出码，走第一组",
			res:  contract.NodeResult{ErrorText: "sshfleet.connect.dial_refused: dial tcp 10.0.0.1:22: connect: connection refused"},
			want: "连接失败",
		},
		{
			name: "远端原话认认证失败",
			res:  contract.NodeResult{ErrorText: "Permission denied (publickey,password)."},
			want: "认证失败",
		},
		{
			name: "超时：报文进第一组",
			res:  contract.NodeResult{TimedOut: true, ErrorText: "sshfleet.run.timeout: exceeded 30s"},
			want: "执行超时",
		},
		{
			name: "关键词里带星号：按片段顺序查找",
			res:  contract.NodeResult{ErrorText: "ssh: command uptime failed: broken pipe"},
			want: "命令失败",
		},
		{
			name: "大小写不敏感",
			res:  contract.NodeResult{ErrorText: "PERMISSION DENIED (publickey)"},
			want: "认证失败",
		},
		{
			name: "一条都没命中：分类名就是原文本身",
			res:  contract.NodeResult{ErrorText: "something odd happened"},
			want: "something odd happened",
		},
		{
			name: "两段都空：兜底原因未知",
			res:  contract.NodeResult{},
			want: "原因未知",
		},
		{
			name: "有退出码、原文命中第二组：密码过期顶掉退出码兜底",
			res: contract.NodeResult{ExitCode: intPtr(1),
				ErrorText: "Password change required but no TTY available"},
			want: "密码过期",
		},
		{
			name: "有退出码、一组都不命中：按退出码给那一类",
			res:  contract.NodeResult{ExitCode: intPtr(3), ErrorText: "boom"},
			want: "执行失败(退出码3)",
		},
		{
			// 退出码 0 却不算成功（报文说明了另有一件事没成）：原因不在退出码里，
			// 按第一组的规矩走——不许拼出「执行失败(退出码0)」把原文盖掉。
			name: "退出码 0 但有报文：落回第一组",
			res:  contract.NodeResult{ExitCode: intPtr(0), ErrorText: "sshfleet.answer.aborted: eof"},
			want: "代填中止",
		},
		{
			name: "成功那一路也要求没超时",
			res:  contract.NodeResult{Connected: true, ExitCode: intPtr(0), TimedOut: true},
			want: "原因未知",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := categoryOf(contract.ModeCommand, tables, &c.res, ""); got != c.want {
				t.Fatalf("分类名该是 %q，实际 %q", c.want, got)
			}
		})
	}
}

// TestCategoryGroupDependsOnExitCode 两组的分工只看一件事：目的一条命令有没有给出退出码。
//
// 同一段原文（`connection reset by peer`）两组都收，选哪一组由退出码在不在决定。
func TestCategoryGroupDependsOnExitCode(t *testing.T) {
	tables := classifyTables()

	// 没有退出码：第一组里没有这条关键词，认不出 → 分类名是原文本身。
	noCode := contract.NodeResult{ErrorText: "connection reset by peer"}
	if got := categoryOf(contract.ModeCommand, tables, &noCode, ""); got != "connection reset by peer" {
		t.Fatalf("没有退出码该走第一组、认不出就显示原文，实际 %q", got)
	}

	// 有退出码：第二组命中，拿的是第二组那个同名分类。
	withCode := contract.NodeResult{ExitCode: intPtr(1), ErrorText: "connection reset by peer"}
	if got := categoryOf(contract.ModeCommand, tables, &withCode, ""); got != "连接失败" {
		t.Fatalf("有退出码该走第二组，实际 %q", got)
	}
}

// TestCategoryFallbackIsTruncated 一段长报文当分类名时要截断，不然统计块与报告会被撑爆。
func TestCategoryFallbackIsTruncated(t *testing.T) {
	long := strings.Repeat("a", 250)
	res := contract.NodeResult{ErrorText: long}
	got := categoryOf(contract.ModeCommand, classifyTables(), &res, "")

	want := strings.Repeat("a", 200) + "…"
	if got != want {
		t.Fatalf("兜底分类名该截到 200 个字符加一个省略号（%d 个字符），实际 %d 个字符：%q",
			len([]rune(want)), len([]rune(got)), got)
	}
}

// TestCategoryEmptyTables 判据表一句都没配时不 panic：有退出码的按退出码兜底，其余的退回原文。
func TestCategoryEmptyTables(t *testing.T) {
	res := contract.NodeResult{ExitCode: intPtr(1), ErrorText: "sshfleet.run.rejected: rejected"}
	if got := categoryOf(contract.ModeCommand, contract.Tables{}, &res, ""); got != "执行失败(退出码1)" {
		t.Fatalf("判据表空时该按退出码兜底，实际 %q", got)
	}
}

// passwdTables 在通用表之上加一张改密专用表，形状照随包 `keywords_passwd.conf`。
func passwdTables() contract.Tables {
	t := classifyTables()
	t.PasswdCategories = []contract.Category{
		{Name: "密码未过期", Keywords: []string{"sshfleet.passwd.not_expired"}},
		{Name: "改密未完成", Keywords: []string{"sshfleet.passwd.give_up"}},
		{Name: "写不进密码文件", Keywords: []string{"Authentication token manipulation error"}},
	}
	return t
}

// TestCategoryOfPasswdUsesOwnTableFirst 改密：先认自己那张 15 条，认不出再退通用。
//
// 顺序有两处硬要求：专用表里那条「改密未完成」必须抢在通用表前面（早收场时工具自己那句
// 报文就是在说这件事）；而连不上那类远程原文通用表照样接得住（专用表里没有那些措辞）。
func TestCategoryOfPasswdUsesOwnTableFirst(t *testing.T) {
	tables := passwdTables()

	cases := []struct {
		name string
		text string
		want string
	}{
		{"专用表命中", "sshfleet.passwd.not_expired: no password-expiry prompt seen", "密码未过期"},
		{"工具报文落在专用表那一条", "sshfleet.passwd.give_up: no further prompts after answers used up", "改密未完成"},
		{"远端原话落在专用表那一条", "passwd: Authentication token manipulation error", "写不进密码文件"},
		{"专用表认不出就退通用", "sshfleet.connect.refused: dial tcp 127.0.0.1:2201: connect: connection refused", "连接失败"},
	}
	for _, c := range cases {
		res := contract.NodeResult{Connected: true, ErrorText: c.text}
		if got := categoryOf(contract.ModePasswd, tables, &res, ""); got != c.want {
			t.Fatalf("%s：期望 %q，实际 %q", c.name, c.want, got)
		}
	}
}

// TestCategoryOfCommandIgnoresPasswdTable 同一条原文，命令模式不该被改密那张表抢走。
//
// 那 15 条说的都是改密场景的话；命令模式下认它，等于把「改密没做成」的理由安到一次
// 普通命令上。
func TestCategoryOfCommandIgnoresPasswdTable(t *testing.T) {
	const text = "sshfleet.passwd.not_expired: no password-expiry prompt seen"
	res := contract.NodeResult{Connected: true, ErrorText: text}

	if got := categoryOf(contract.ModeCommand, passwdTables(), &res, ""); got != text {
		t.Fatalf("命令模式该认不出、回落原文，实际 %q", got)
	}
}

// TestCategoryOfPasswdSkipsServerHint 改密不拿服务端提示认分类。
//
// 「密码必须马上改」那句在改密里是**前提**（它就是入场信号），不是失败原因：算进去，
// 跑到一半被 Ctrl+C 那一档会被「密码过期」抢走——那一档排在它后面。
func TestCategoryOfPasswdSkipsServerHint(t *testing.T) {
	const hint = "You are required to change your password immediately (administrator enforced)."
	tables := contract.Tables{Categories: []contract.Category{
		{Name: "密码过期", Keywords: []string{"You are required to change your password immediately"}},
		{Name: "任务已取消", Keywords: []string{"sshfleet.abort.canceled"}},
	}}
	res := contract.NodeResult{Connected: true, Cancelled: true,
		ErrorText: "context canceled\nsshfleet.abort.canceled: interrupted after session established\n" + hint}

	if got := categoryOf(contract.ModePasswd, tables, &res, hint); got != "任务已取消" {
		t.Fatalf("改密被中断该归「任务已取消」，实际 %q", got)
	}
	// 没给提示时照旧按原文认：这条只管「提示不参与」，不放宽别的。
	if got := categoryOf(contract.ModePasswd, tables, &res, ""); got != "密码过期" {
		t.Fatalf("没给提示时该照旧按原文认，实际 %q", got)
	}
	// 命令模式不吃这条豁免：提示照旧参与分类。
	if got := categoryOf(contract.ModeCommand, tables, &res, hint); got != "密码过期" {
		t.Fatalf("命令模式该照旧连着提示查表，实际 %q", got)
	}
}

// TestCategoryOfAbort 中断两档：各认自己那句报文，名字写死（照随包配置那两类的次序）。
//
// 判据表的次序就是随包的次序——「建连被取消」排在表头，「任务已取消」在后面一档；两档
// 只认工具自己写的那句报文，所以别的报文串不进来。
func TestCategoryOfAbort(t *testing.T) {
	tables := contract.Tables{Categories: []contract.Category{
		{Name: "建连被取消", Keywords: []string{"sshfleet.abort.connect_canceled"}},
		{Name: "任务已取消", Keywords: []string{"sshfleet.abort.canceled", "context canceled"}},
	}}

	cases := []struct {
		name string
		res  contract.NodeResult
		want string
	}{
		{
			name: "还没连上就被打断",
			res: contract.NodeResult{ErrorText: "dial tcp 10.0.0.1:22: i/o timeout\n" +
				"sshfleet.abort.connect_canceled: interrupted while connecting"},
			want: "建连被取消",
		},
		{
			name: "已连上、跑在命令中途",
			res: contract.NodeResult{Connected: true, ErrorText: "context canceled\n" +
				"sshfleet.abort.canceled: interrupted after session established"},
			want: "任务已取消",
		},
		{
			// Windows 上命令中途取消，原文里会多带一句 operation was canceled；
			// 「建连被取消」排在表头，若它认这个词，这一档就被抢走了。
			name: "Windows 上跑在命令中途",
			res: contract.NodeResult{Connected: true, ErrorText: "The operation was canceled by the user.\n" +
				"sshfleet.abort.canceled: interrupted after session established"},
			want: "任务已取消",
		},
	}

	for _, c := range cases {
		res := c.res
		if got := categoryOf(contract.ModeCommand, tables, &res, ""); got != c.want {
			t.Fatalf("%s：期望 %q，实际 %q", c.name, c.want, got)
		}
	}
}
