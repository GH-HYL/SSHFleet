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
	a := &Args{ChangePasswordRaw: "NewPass123"}
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
	a := &Args{ChangePasswordRaw: path} // credentialEncrypted = false
	if err := checkPasswdChange(a); err != nil {
		t.Fatalf("不开加密时它就是一个密码，不该去读文件：%v", err)
	}
	if a.NewPassword != path {
		t.Fatalf("应原样当密码，实际：%q", a.NewPassword)
	}
}

// 开了加密时值是凭据文件路径：文件不在就报错，不静默当成明文。
func TestCheckPasswdChangeEncryptedMissingFile(t *testing.T) {
	a := &Args{ChangePasswordRaw: filepath.Join(t.TempDir(), "pw.bin"), credentialEncrypted: true}
	if err := checkPasswdChange(a); err == nil {
		t.Fatal("加密开关打开而文件不存在时应报错")
	}
}

func TestCheckPasswdChangeExclusive(t *testing.T) {
	cases := map[string]*Args{
		"-a":         {ChangePasswordRaw: "pw", Answer: "1,架构"},
		"-p":         {ChangePasswordRaw: "pw", Path: "/x/"},
		"-k":         {ChangePasswordRaw: "pw", Key: true},
		"--no-shell": {ChangePasswordRaw: "pw", NoShell: true},
		"--sudo":     {ChangePasswordRaw: "pw", sudoFlag: true},
		"--no-sudo":  {ChangePasswordRaw: "pw", noSudoFlag: true},
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
	both := &Args{ChangePasswordRaw: "pw", Command: "uptime"}
	if err := CheckArguments(both); err == nil || !strings.Contains(err.Error(), "互斥") {
		t.Fatalf("改密与命令模式同给应报互斥，实际：%v", err)
	}
	if err := CheckArguments(&Args{ChangePasswordRaw: "pw"}); err != nil {
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
	a := &Args{ChangePasswordRaw: "NewPass123"}
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

// 首尾空白不修值，直接拦：程序不替使用者去空白（ADR-0027）。
//
// 去空白会把他设的密码悄悄改掉，而屏幕上看到的还是带空格那串；拦下来由他自己换一个，
// 屏幕与实际设置值才不会对不上。
func TestCheckPasswdChangeRejectsOuterSpace(t *testing.T) {
	for _, value := range []string{" NewPass123", "NewPass123 ", " NewPass123 ", "\tNewPass123"} {
		a := &Args{ChangePasswordRaw: value}
		err := checkPasswdChange(a)
		if err == nil || !strings.Contains(err.Error(), "开头或结尾有空白") {
			t.Fatalf("%q 应报首尾空白，实际：%v", value, err)
		}
		if a.NewPassword != "" {
			t.Fatalf("被拦下时不该有解析结果，实际：%q", a.NewPassword)
		}
	}
}

// 换行与回车任意位置都拦：改密是把字节喂进远端终端，换行等于回车，
// 提交上去的是半截密码，远端重问或失败，报错指不到原因。
func TestCheckPasswdChangeRejectsNewline(t *testing.T) {
	for _, value := range []string{"New\nPass123", "New\rPass123", "NewPass123\n"} {
		a := &Args{ChangePasswordRaw: value}
		err := checkPasswdChange(a)
		if err == nil || !strings.Contains(err.Error(), "换行符或回车") {
			t.Fatalf("%q 应报换行，实际：%v", value, err)
		}
	}
}

// 中间的空格是正常写法，放行，且原样当密码用。
func TestCheckPasswdChangeKeepsInnerSpace(t *testing.T) {
	a := &Args{ChangePasswordRaw: "New Pass 2026"}
	if err := checkPasswdChange(a); err != nil {
		t.Fatalf("中间空格应放行：%v", err)
	}
	if a.NewPassword != "New Pass 2026" {
		t.Fatalf("应原样取出，实际：%q", a.NewPassword)
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

	a := &Args{ChangePasswordRaw: path, credentialEncrypted: true}
	if err := checkPasswdChange(a); err != nil {
		t.Fatalf("加密模式下应解出新密码：%v", err)
	}
	if a.NewPassword != "NewPass#2026" {
		t.Fatalf("应解出明文，实际：%q", a.NewPassword)
	}
}

// 加密模式解出来的明文同口径拦：密文文件里存着带首尾空格的密码，一样不许设上去。
//
// 这条钉的是「两种给法一套规矩」——只拦明文的话，把带空格的密码写进密文文件还能绕过。
func TestCheckPasswdChangeRejectsOuterSpaceFromCipherFile(t *testing.T) {
	const key = "test-master-key-2026"
	path := filepath.Join(t.TempDir(), "new_pw.cred")

	token, err := credential.EncryptV2(" NewPass#2026 ", key)
	if err != nil {
		t.Fatalf("造密文失败：%v", err)
	}
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		t.Fatalf("写凭据文件失败：%v", err)
	}
	t.Setenv("SSHFLEET_KEY", key)

	a := &Args{ChangePasswordRaw: path, credentialEncrypted: true}
	err = checkPasswdChange(a)
	if err == nil || !strings.Contains(err.Error(), "开头或结尾有空白") {
		t.Fatalf("解出的明文带首尾空白时应拦下，实际：%v", err)
	}
	if a.NewPassword != "" {
		t.Fatalf("被拦下时不该有解析结果，实际：%q", a.NewPassword)
	}
}

// 密文内容被当成字符串贴进这一格：先告诉它该写什么，而不是拿这串去 Stat。
// 走公开入口 Parse + CheckArguments，不碰包内私有函数。
func TestCheckArgumentsRejectsCiphertextPastedAsValue(t *testing.T) {
	cipherText, err := credential.EncryptV2("NewPass123", "master-key-for-test")
	if err != nil {
		t.Fatalf("造密文失败：%v", err)
	}

	cases := map[string]struct {
		encrypted bool
		want      string
	}{
		"没开加密": {encrypted: false, want: "写的是密文内容，不是新密码"},
		"开了加密": {encrypted: true, want: "写的是密文内容，不是文件路径"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := helpTestCfg()
			cfg.Credential.Encrypt = c.encrypted
			a, err := Parse(cfg, "test", []string{
				"-f", "1.1.1.1,22,root,pw", "--change-password", cipherText,
			})
			if err != nil {
				t.Fatalf("Parse 不该在这里报错：%v", err)
			}
			err = CheckArguments(a)
			if err == nil {
				t.Fatal("把密文内容当值用，应被拒绝")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("文案应含 %q，实际：%q", c.want, err.Error())
			}
			if !strings.Contains(err.Error(), "\n提示：") {
				t.Fatalf("应带提示行，实际：%q", err.Error())
			}
		})
	}
}
