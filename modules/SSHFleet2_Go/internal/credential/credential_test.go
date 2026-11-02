package credential

import (
	"errors"
	"strings"
	"testing"
)

// 这一份密文是**用老工程那份加密引擎**生成的（HKDF-SHA256 + AES-256-GCM，0x02 布局），
// 不是本包自己加出来再自己解——期望值独立于被测代码，见《个人开发规范》三.3。
const (
	testMaster = "sshfleet-test-master-key-0123456789"
	testPlain  = "Sshfleet#2026"
	testToken  = "AtKvT37VuLHwfIb8LkmbXSmfFepr17YYvRI1txT+WFdV/kVFvEuG4FUP"
)

func TestDecryptOldToolCiphertext(t *testing.T) {
	got, err := Decrypt(testToken, testMaster)
	if err != nil {
		t.Fatalf("解不开老工具生成的密文：%v", err)
	}
	if got != testPlain {
		t.Fatalf("解出来的明文不对：%q", got)
	}
}

func TestDecryptRejectsWrongMasterKey(t *testing.T) {
	if _, err := Decrypt(testToken, "另一把主密钥"); !errors.Is(err, ErrBadCipher) {
		t.Fatalf("换一把主密钥应当归到 ErrBadCipher，got %v", err)
	}
}

func TestDecryptRejectsTruncated(t *testing.T) {
	if _, err := Decrypt(testToken[:20], testMaster); !errors.Is(err, ErrBadCipher) {
		t.Fatalf("截断的密文应当归到 ErrBadCipher，got %v", err)
	}
}

func TestDecryptRejectsGarbage(t *testing.T) {
	if _, err := Decrypt("这不是密文", testMaster); !errors.Is(err, ErrBadCipher) {
		t.Fatalf("不是密文的内容应当归到 ErrBadCipher，got %v", err)
	}
}

// 加密关着时那一格就是密码本身：原样交回，一个字符都不动（包括首尾空白）。
func TestSecretPlainPassesThroughUntouched(t *testing.T) {
	for _, v := range []string{"pw", " pw ", "带 空格 的 口令", ""} {
		got, err := Secret(v, false)
		if err != nil {
			t.Fatalf("加密关着不该报错：%v", err)
		}
		if got != v {
			t.Fatalf("原值被改动了：want %q, got %q", v, got)
		}
	}
}

// 加密开着时读的是**装了密文的文件**，文件末尾那个换行不该影响结果。
func TestSecretReadsCipherFile(t *testing.T) {
	t.Setenv(envName, testMaster)
	got, err := Secret("testdata/password.enc", true)
	if err != nil {
		t.Fatalf("读密文文件失败：%v", err)
	}
	if got != testPlain {
		t.Fatalf("解出来的明文不对：%q", got)
	}
}

func TestSecretNeedsMasterKeyWhenEncrypted(t *testing.T) {
	t.Setenv(envName, "")
	if _, err := Secret("testdata/password.enc", true); !errors.Is(err, ErrKeyMissing) {
		t.Fatalf("没有主密钥时应当交 ErrKeyMissing，got %v", err)
	}
}

func TestSecretReportsMissingFile(t *testing.T) {
	t.Setenv(envName, testMaster)
	_, err := Secret("testdata/根本没有这个文件.enc", true)
	if err == nil || errors.Is(err, ErrKeyMissing) {
		t.Fatalf("文件读不到应当报「读不到」，不是「主密钥没读到」：%v", err)
	}
}

func TestStatusTracksEnv(t *testing.T) {
	t.Setenv(envName, "")
	if Status() != KeyMissing {
		t.Fatal("没有主密钥时状态应当是 missing")
	}
	t.Setenv(envName, "  "+testMaster+"  ")
	if Status() != KeyReady {
		t.Fatal("有主密钥时状态应当是 ready（首尾空白不算）")
	}
}

func TestMasterKeyTrimsWhitespace(t *testing.T) {
	t.Setenv(envName, "  "+testMaster+"\n")
	got, err := MasterKey()
	if err != nil {
		t.Fatalf("应当读得到：%v", err)
	}
	if strings.TrimSpace(got) != testMaster {
		t.Fatalf("首尾空白没去掉：%q", got)
	}
}
