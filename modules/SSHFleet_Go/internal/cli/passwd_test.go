package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet/internal/credential"
)

// --change-password 的取值与互斥。取值形态由 credential.encrypt 开关唯一决定，
// **不做**"文件还是文本"的自动判定（明文密码可能恰好等于一个存在的文件名）。

func TestCheckPasswdChangePlainText(t *testing.T) {
	a := &Args{ChangePassword: "NewPass123"}
	if err := checkPasswdChange(a); err != nil {
		t.Fatalf("明文新密码应通过：%v", err)
	}
	if a.NewPassword != "NewPass123" {
		t.Fatalf("明文应原样取出，实际：%q", a.NewPassword)
	}
}

// 不开加密时，哪怕这个值恰好是一个存在的文件路径，也照样当密码解释。
func TestCheckPasswdChangeValueIsNeverAutoDetected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.csv")
	if err := os.WriteFile(path, []byte("1.1.1.1,22,root,pw\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &Args{ChangePassword: path} // credentialEncrypted = false
	if err := checkPasswdChange(a); err != nil {
		t.Fatalf("不开加密时它就是一个密码，不该去读文件：%v", err)
	}
	if a.NewPassword != path {
		t.Fatalf("应原样当密码，实际：%q", a.NewPassword)
	}
}

// 开了加密时值是凭据文件路径：文件不在就报错，不静默当成明文。
func TestCheckPasswdChangeEncryptedMissingFile(t *testing.T) {
	a := &Args{ChangePassword: filepath.Join(t.TempDir(), "pw.bin"), credentialEncrypted: true}
	if err := checkPasswdChange(a); err == nil {
		t.Fatal("加密开关打开而文件不存在时应报错")
	}
}

func TestCheckPasswdChangeExclusive(t *testing.T) {
	cases := map[string]*Args{
		"-a":         {ChangePassword: "pw", Answer: "1,架构"},
		"-p":         {ChangePassword: "pw", Path: "/x/"},
		"-k":         {ChangePassword: "pw", Key: true},
		"--no-shell": {ChangePassword: "pw", NoShell: true},
		"--sudo":     {ChangePassword: "pw", sudoFlag: true},
		"--no-sudo":  {ChangePassword: "pw", noSudoFlag: true},
	}
	for name, a := range cases {
		if err := checkPasswdChange(a); err == nil || !strings.Contains(err.Error(), "--change-password 不能和") {
			t.Fatalf("%s 时应明确报错，实际：%v", name, err)
		}
	}
}

// 没给 --change-password 时这几条互斥一律不掺和。
func TestCheckPasswdChangeQuietWhenAbsent(t *testing.T) {
	a := &Args{Command: "uptime", Answer: "1,架构", sudoFlag: true}
	if err := checkPasswdChange(a); err != nil {
		t.Fatalf("没给改密参数时不该报错：%v", err)
	}
}

// 改密是执行模式之一：五选一。
func TestCheckArgumentsModeIsFiveWay(t *testing.T) {
	both := &Args{ChangePassword: "pw", Command: "uptime"}
	if err := CheckArguments(both); err == nil || !strings.Contains(err.Error(), "互斥") {
		t.Fatalf("改密与命令模式同给应报互斥，实际：%v", err)
	}
	if err := CheckArguments(&Args{ChangePassword: "pw"}); err != nil {
		t.Fatalf("只给改密应通过到后面的清单检查：%v", err)
	}
	none := &Args{}
	err := CheckArguments(none)
	if err == nil || !strings.Contains(err.Error(), "五个") {
		t.Fatalf("一个模式都没给时应列出五个模式，实际：%v", err)
	}
}

// 模式名进 ModeName，日志形态里的新密码必须是脱敏形态。
//
// 日志报的是**解析后**的新密码（加密模式下旗标原值是一份文件路径，报它等于什么都没报），
// 所以夹具走一遍真实的解析，而不是只塞一个旗标值。
func TestPasswdModeNameAndMaskedSummary(t *testing.T) {
	a := &Args{ChangePassword: "NewPass123"}
	if err := checkPasswdChange(a); err != nil {
		t.Fatalf("明文新密码应通过：%v", err)
	}
	if got := a.ModeName(); got != "passwd" {
		t.Fatalf("模式名应为 passwd，实际：%q", got)
	}
	summary := a.Summary()
	if strings.Contains(summary, "NewPass123") {
		t.Fatalf("新密码不该明文进日志：%s", summary)
	}
	if !strings.Contains(summary, "change_password='Ne****23'") {
		t.Fatalf("应报脱敏形态，实际：%s", summary)
	}
}

// 首尾空白在解析阶段就被去掉：喂给远端的是去掉之后的值。
//
// 确认屏与报告显示的是 NewPassword 的形态（见 confirm / output 的测试），所以「屏上看到的
// 就是实际设进去的那个值」这件事，前提正是这里——不在这里钉住，那个断言只是拿夹具自证。
func TestCheckPasswdChangeTrimsOuterSpace(t *testing.T) {
	a := &Args{ChangePassword: " NewPass123 "}
	if err := checkPasswdChange(a); err != nil {
		t.Fatalf("明文新密码应通过：%v", err)
	}
	if a.NewPassword != "NewPass123" {
		t.Fatalf("首尾空白应去掉，实际：%q", a.NewPassword)
	}
}

// 加密链路：旗标值是密文文件路径，解出来的才是新密码。
//
// 这是「三处显示点取 NewPassword」这件事的**前提**。显示那几条测试都手塞 NewPassword，
// 所以这条链一断（不再解密、或解成空），它们照样全绿，而线上三处会显示 ****。
// 造密文走真实加密，主密钥经环境变量注入（GetMasterKey 只认 SSHFLEET_KEY）。
func TestCheckPasswdChangeDecryptsCipherFile(t *testing.T) {
	const key = "test-master-key-2026"
	path := filepath.Join(t.TempDir(), "new_pw.cred")

	token, err := credential.EncryptV2("NewPass#2026", key)
	if err != nil {
		t.Fatalf("造密文失败：%v", err)
	}
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		t.Fatalf("写凭据文件失败：%v", err)
	}
	t.Setenv("SSHFLEET_KEY", key)

	a := &Args{ChangePassword: path, credentialEncrypted: true}
	if err := checkPasswdChange(a); err != nil {
		t.Fatalf("加密模式下应解出新密码：%v", err)
	}
	if a.NewPassword != "NewPass#2026" {
		t.Fatalf("应解出明文，实际：%q", a.NewPassword)
	}
}
