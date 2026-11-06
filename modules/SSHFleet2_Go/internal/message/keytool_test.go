package message

import (
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// 密钥那几条命令的话（自定义区）：期望串逐字来自老版源码，不拿被测代码的拼法反推。
//
// 色码照 message 的老规矩带着——去不去由 console 按流与终端能力决定。

const (
	keyEscYellow = "\x1b[33m"
	keyEscReset  = "\x1b[0m"
)

// 两处来源那三行：本次运行、本机已保存、另一处持久值。
const wantSourceLines = "  本次运行实际使用：指纹 11111111（进程环境变量 SSHFLEET_KEY）\n" +
	"  本机已保存：      指纹 22222222（/home/u/.bashrc）\n" +
	"  另一处持久值：    指纹 33333333（/home/u/.zshrc）\n"

func posixFacts(state contract.KeyState) contract.KeyFacts {
	return contract.KeyFacts{
		Host: contract.KeyHostPOSIX, State: state, EnvName: "SSHFLEET_KEY",
		EnvSet: true, EnvPrint: "11111111",
		Persisted: true, PersistPrint: "22222222", PersistWhere: "/home/u/.bashrc",
		OtherPrint: "33333333", OtherWhere: "/home/u/.zshrc",
		RCPath: "/home/u/.bashrc",
	}
}

// 不一致那一屏：标题上色、三行来源、影响与两条出路。
func TestKeyStatusReportDiverged(t *testing.T) {
	got := KeyStatusReport(posixFacts(contract.KeyStateDiverged)).Text()
	want := keyEscYellow + "主密钥状态：不一致" + keyEscReset + "\n" + wantSourceLines +
		"  影响：用「本次运行这把」加密出的文件，重开终端后就解不开；凭据解密也可能报「主密钥不匹配」\n" +
		"  下一步：执行 source /home/u/.bashrc 或重新打开终端，让本次运行改用已保存的那把\n" +
		"         若确定要用本次运行这把：把 /home/u/.bashrc 里的 export 行改成当前值（先 echo $SSHFLEET_KEY 取值）：\n" +
		"       sed -i \"s|^export SSHFLEET_KEY=.*|export SSHFLEET_KEY='$SSHFLEET_KEY'|\" /home/u/.bashrc && source /home/u/.bashrc\n"
	if got != want {
		t.Fatalf("不一致那一屏不对：\n got %q\nwant %q", got, want)
	}
}

// 未设置那一屏：不上色，末句给生成指引。
func TestKeyStatusReportUnset(t *testing.T) {
	facts := contract.KeyFacts{
		Host: contract.KeyHostPOSIX, State: contract.KeyStateUnset, EnvName: "SSHFLEET_KEY",
		RCPath: "/home/u/.bashrc",
	}
	got := KeyStatusReport(facts).Text()
	want := "主密钥状态：未设置\n" +
		"  本次运行实际使用：未读到（进程环境变量 SSHFLEET_KEY 为空）\n" +
		"  本机已保存：      无\n" +
		"  下一步：先生成主密钥——SSHFleet --gen-key（加密凭据需要它）\n"
	if got != want {
		t.Fatalf("未设置那一屏不对：\n got %q\nwant %q", got, want)
	}
}

// 预检查那段走标准错误、前面空一行、带 [警告]，且没有密钥本身。
func TestKeyPrecheckNote(t *testing.T) {
	line := KeyPrecheckNote(posixFacts(contract.KeyStateDiverged))
	if !line.Stderr() {
		t.Fatalf("预检查要写标准错误")
	}
	got := line.Text()
	if !strings.HasPrefix(got, "\n"+keyEscYellow+"[警告]"+keyEscReset+" 主密钥两处来源不一致：\n") {
		t.Fatalf("开头不对：%q", got)
	}
	for _, want := range []string{
		"  让两处一致：执行 source /home/u/.bashrc 或重新打开终端\n",
		"（加密已被工具拦下）",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("缺「%s」：%q", want, got)
		}
	}

	// 两处一致、两处都空：一个字都不说。
	if text := KeyPrecheckNote(posixFacts(contract.KeyStateOK)).Text(); text != "" {
		t.Fatalf("正常时不该说话：%q", text)
	}
}

// 日志那行：括号里是老版的状态名。
func TestKeyPrecheckTrace(t *testing.T) {
	if got, want := KeyPrecheckTrace(contract.KeyStateStaleShell).Plain(), "主密钥预检查未通过（stale_shell）"; got != want {
		t.Fatalf("日志那行不对：got %q want %q", got, want)
	}
}

// 命令不合规那两句：与老版逐字相同。
func TestKeyToolComplaints(t *testing.T) {
	if got, want := KeyToolNeedAlone("--gen-key、--key-status"),
		"--gen-key、--key-status 只能给一个——它们是三个独立命令，一次做一件事"; got != want {
		t.Fatalf("互斥那句不对：\n got %q\nwant %q", got, want)
	}
	if got, want := KeyToolWithBatch("--key-status", "-c、-r"),
		"--key-status 不能和批量执行选项一起用：同时给了 -c、-r\n"+
			"提示：分成两次执行——先跑密钥管理命令，再跑批量执行"; got != want {
		t.Fatalf("与批量参数同给那句不对：\n got %q\nwant %q", got, want)
	}
}

// 覆盖确认那几句：问句、保留、覆盖、非交互拒绝。
func TestKeyGenScreens(t *testing.T) {
	if got, want := KeyGenConfirm().Text(), "是否确认覆盖？"; got != want {
		t.Fatalf("问句不对：got %q want %q", got, want)
	}
	if got, want := KeyGenKept().Text(), "已保留原密钥，未做修改\n"; got != want {
		t.Fatalf("保留那句不对：got %q want %q", got, want)
	}
	if got, want := KeyGenOverwrite().Text(), "已确认覆盖，正在重新生成主密钥...\n"; got != want {
		t.Fatalf("覆盖那句不对：got %q want %q", got, want)
	}
	want := "检测到已存在主密钥，非交互模式不自动覆盖（覆盖后旧密钥加密的凭据文件将无法解密）。\n" +
		"如需覆盖：去掉 --yes 后重新执行 --gen-key"
	if got := KeyGenNoOverwrite(); got != want {
		t.Fatalf("非交互拒绝那句不对：\n got %q\nwant %q", got, want)
	}
}

// 已有密钥时开场那段：不一致那一行在，色码在箭头前面（与老版同一处）。
func TestKeyGenExistingDiverged(t *testing.T) {
	got := KeyGenExisting(posixFacts(contract.KeyStateDiverged)).Text()
	want := "检测到已存在主密钥，当前未做任何修改\n" +
		"  本次运行实际使用：指纹 11111111（进程环境变量 SSHFLEET_KEY）\n" +
		"  本机已保存：      指纹 22222222（/home/u/.bashrc）\n" +
		"  " + keyEscYellow + "→ 两处不一致" + keyEscReset + "——这正是凭据解密报「主密钥不匹配」的常见原因\n" +
		"注意：如果覆盖，用旧密钥加密的凭据文件将永久无法解密\n"
	if got != want {
		t.Fatalf("开场那段不对：\n got %q\nwant %q", got, want)
	}
}

// 写完之后那一段：POSIX 说 rc 路径与 source，Windows 说注册表与重开终端。
func TestKeyGenSavedBothPlatforms(t *testing.T) {
	facts := posixFacts(contract.KeyStateUnset)
	posix := KeyGenSaved(false, facts, contract.PersistResult{Where: "/home/u/.bashrc"}, "44444444").Text()
	wantPosix := "主密钥已生成，并自动保存到 /home/u/.bashrc\n" +
		"请执行 source /home/u/.bashrc 或重新打开终端后生效\n" +
		keyEscYellow + "注意：在此之前，本次运行读到的仍是旧密钥，执行 --convert-secret 会被拦下（避免加密出将来看不开的文件）" + keyEscReset + "\n" +
		"  新密钥指纹：44444444\n"
	if posix != wantPosix {
		t.Fatalf("POSIX 收尾不对：\n got %q\nwant %q", posix, wantPosix)
	}

	winFacts := facts
	winFacts.Host = contract.KeyHostWindows
	winFacts.RCPath = ""
	win := KeyGenSaved(true, winFacts, contract.PersistResult{Where: `注册表 HKCU\Environment`}, "55555555").Text()
	wantWin := "主密钥已重新生成，并写入本机（HKCU\\Environment）\n" +
		"请重新打开终端后再使用：当前终端的环境变量仍是旧密钥\n" +
		keyEscYellow + "注意：在此之前，本次运行读到的仍是旧密钥，执行 --convert-secret 会被拦下（避免加密出将来看不开的文件）" + keyEscReset + "\n" +
		"  新密钥指纹：55555555\n"
	if win != wantWin {
		t.Fatalf("Windows 收尾不对：\n got %q\nwant %q", win, wantWin)
	}
}

// 转换那几句：两种「无需转换」、成功那两行、三种读不到的毛病。
func TestKeyConvertScreens(t *testing.T) {
	if got, want := KeyConvertAlreadyCipher("/x/sec").Text(), "当前已是密文，无需转换：/x/sec\n"; got != want {
		t.Fatalf("已是密文那句不对：got %q want %q", got, want)
	}
	if got, want := KeyConvertAlreadyPlain("/x/sec").Text(), "当前已是明文（文件内容即密码本身），无需转换：/x/sec\n"; got != want {
		t.Fatalf("已是明文那句不对：got %q want %q", got, want)
	}
	if got, want := KeyConvertDone("明文", "密文", "/x/sec", 9).Text(),
		"转换成功：明文 → 密文：/x/sec\n内容长度：9 字符，请用编辑器打开该文件确认结果\n"; got != want {
		t.Fatalf("成功那两行不对：\n got %q\nwant %q", got, want)
	}
	cases := map[contract.CredFileProblem]string{
		contract.CredFileEmpty:     "凭据文件是空的，请先填入密码再转换：/x/sec",
		contract.CredFileBinary:    "凭据文件含二进制数据，不是文本格式，无法转换：/x/sec",
		contract.CredFileMultiline: "凭据文件有多行内容，但密码应为单行，请检查是否误粘贴：/x/sec",
	}
	for code, want := range cases {
		if got := KeyConvertReadProblem(code, "/x/sec", ""); got != want {
			t.Fatalf("%s 那句不对：\n got %q\nwant %q", code, got, want)
		}
	}
	if got, want := KeyConvertReadProblem(contract.CredFileUnreadable, "/x/sec", "no such file"),
		"读取凭据文件失败：/x/sec\n原因：no such file"; got != want {
		t.Fatalf("读不到那句不对：\n got %q\nwant %q", got, want)
	}
}

// 加密被拦那一屏：先给结论，再给两条出路。
func TestKeyConvertBlocked(t *testing.T) {
	got := KeyConvertBlocked(posixFacts(contract.KeyStateDiverged))
	for _, want := range []string{
		"已阻止加密。\n",
		"主密钥来源不一致——这会让凭据解密报「主密钥不匹配」",
		"出路：\n  ① 执行 source /home/u/.bashrc 或重新打开终端，然后重试\n",
		"  ② 若确定要用本次运行这把密钥：",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("缺「%s」：%q", want, got)
		}
	}
}

// 三条命令进日志那一行。
func TestKeyModeEntered(t *testing.T) {
	for which, want := range map[contract.KeyTool]string{
		contract.KeyToolGen:     "进入密钥管理模式（--gen-key）",
		contract.KeyToolStatus:  "进入主密钥状态查看模式（--key-status）",
		contract.KeyToolConvert: "进入凭据转换模式（--convert-secret）",
	} {
		if got := KeyModeEntered(which).Plain(); got != want {
			t.Fatalf("%s 那行不对：got %q want %q", which, got, want)
		}
	}
}
