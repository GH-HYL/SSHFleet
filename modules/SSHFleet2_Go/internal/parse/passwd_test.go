package parse

// 改密值（建站 05 的 07 号单）：两种加密开关、四种坏值、形态两条。
//
// 密文那一支用凭据包 testdata 里那份**老工程生成的**密文与写死的主密钥：
// 期望值是固定的字面量，不由被测代码现算。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

const (
	testMasterKey = "sshfleet-test-master-key-0123456789"
	testCipher    = "../credential/testdata/password.enc"
	testPlain     = "Sshfleet#2026"
)

// cryptoConfig 在临时目录里造一份配置，把 encrypt 开关换成要的取值并切过去。
func cryptoConfig(t *testing.T, encrypt bool) string {
	t.Helper()
	dir := writeTempConfig(t)
	path := filepath.Join(dir, configPath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读临时配置失败：%v", err)
	}
	body := strings.Replace(string(data), "encrypt = false", "encrypt = "+boolLit(encrypt), 1)
	if encrypt {
		// 开了加密之后，配置里的凭据字段一律要绝对路径：这里指向凭据包那份密文样本。
		abs, err := filepath.Abs(testCipher)
		if err != nil {
			t.Fatalf("算绝对路径失败：%v", err)
		}
		body = strings.Replace(body, `password = "pw"`, `password = "`+abs+`"`, 1)
		body = strings.Replace(body, `secret_dir = ""`, `secret_dir = "`+dir+`"`, 1)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("写临时配置失败：%v", err)
	}
	return dir
}

func boolLit(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// passwdRaw 只跑改密那一段：造纸、调 passwdValue。
func passwdRaw(t *testing.T, encrypt bool, raw string) (*contract.Plan, *contract.CannotStart) {
	t.Helper()
	cryptoConfig(t, encrypt)
	cfg, err := load()
	if err != nil {
		t.Fatalf("临时配置读不通：%v", err)
	}
	p := &contract.Plan{
		Mode: contract.ModePasswd,
		Task: contract.Task{Passwd: &contract.PasswdTask{Raw: raw}},
	}
	return p, passwdValue(p, cfg.credentialCfg.Encrypt)
}

// 关加密：原值就是明文，原样进纸。
func TestPasswdPlainValue(t *testing.T) {
	p, fail := passwdRaw(t, false, "NewPass#1")
	if fail != nil {
		t.Fatalf("不该报错：%v", message.CannotStartText(*fail))
	}
	if p.Task.Passwd.NewPassword != "NewPass#1" {
		t.Fatalf("明文没进纸：%q", p.Task.Passwd.NewPassword)
	}
}

// 形态两条：换行、首尾空白。只报不改值。
func TestPasswdShapeRejected(t *testing.T) {
	cases := []struct{ name, raw, want string }{
		{"带换行", "abc\ndef", "换行符或回车"},
		{"首尾空白", " abc", "开头或结尾有空白"},
		{"全是空白", "   ", "开头或结尾有空白"},
	}
	for _, c := range cases {
		_, fail := passwdRaw(t, false, c.raw)
		if fail == nil {
			t.Errorf("%s：应当报错", c.name)
			continue
		}
		if !strings.Contains(message.CannotStartText(*fail), c.want) {
			t.Errorf("%s：话里没有 %q：%s", c.name, c.want, message.CannotStartText(*fail))
		}
	}
}

// 密文贴进这一格：两种开关各一句，说的是两条不同的去路。
func TestPasswdCipherTextInValue(t *testing.T) {
	const token = "AtKvT37VuLHwfIb8LkmbXSmfFepr17YYvRI1txT+WFdV/kVFvEuG4FUP"

	_, fail := passwdRaw(t, false, token)
	if fail == nil || !strings.Contains(message.CannotStartText(*fail), "不是新密码") {
		t.Fatalf("没开加密时要说「不是新密码」：%v", fail)
	}
	_, fail = passwdRaw(t, true, token)
	if fail == nil || !strings.Contains(message.CannotStartText(*fail), "不是文件路径") {
		t.Fatalf("开了加密时要说「不是文件路径」：%v", fail)
	}
}

// 开加密：值是密文文件的路径，解出明文进纸。
func TestPasswdEncryptedValue(t *testing.T) {
	abs, err := filepath.Abs(testCipher)
	if err != nil {
		t.Fatalf("算绝对路径失败：%v", err)
	}
	t.Setenv("SSHFLEET_KEY", testMasterKey)

	p, fail := passwdRaw(t, true, abs)
	if fail != nil {
		t.Fatalf("不该报错：%v", message.CannotStartText(*fail))
	}
	if p.Task.Passwd.NewPassword != testPlain {
		t.Fatalf("解出来的明文不对：%q", p.Task.Passwd.NewPassword)
	}
}

// 开加密但文件不在：三行套（问题 / 原因 / 提示）。
func TestPasswdEncryptedMissingFile(t *testing.T) {
	_, fail := passwdRaw(t, true, "/tmp/没有这个凭据文件.enc")
	if fail == nil {
		t.Fatal("应当报错")
	}
	for _, want := range []string{"的值读不到", "原因：", "提示："} {
		if !strings.Contains(message.CannotStartText(*fail), want) {
			t.Fatalf("三行套里少了 %q：%s", want, message.CannotStartText(*fail))
		}
	}
}

// 主密钥不在：与「值有问题」分开报——这一条要人去看主密钥。
func TestPasswdBadMasterKeySeparate(t *testing.T) {
	abs, err := filepath.Abs(testCipher)
	if err != nil {
		t.Fatalf("算绝对路径失败：%v", err)
	}
	t.Setenv("SSHFLEET_KEY", "")

	_, fail := passwdRaw(t, true, abs)
	if fail == nil {
		t.Fatal("没有主密钥时应当报错")
	}
	if !strings.Contains(message.CannotStartText(*fail), "读不出新密码") {
		t.Fatalf("这一条要说「读不出新密码」：%s", message.CannotStartText(*fail))
	}
	if strings.Contains(message.CannotStartText(*fail), "值有问题") {
		t.Fatalf("主密钥那一类不该混成「值有问题」：%s", message.CannotStartText(*fail))
	}
}
