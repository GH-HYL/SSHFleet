package credential

import (
	"errors"
	"testing"
)

// 这里用的两份样本都是**固定字节**：testdata/password.txt 是明文，testdata/password.enc 是
// 老工程加密引擎对同一串生成的密文，主密钥写死在 testMaster。不碰开发者自己的环境
// （`t.Setenv` 只在本用例进程里生效，环境里原来有什么都不影响结论）。

func codes(problems []ValueProblem) []ValueCode {
	out := make([]ValueCode, 0, len(problems))
	for _, p := range problems {
		out = append(out, p.Code)
	}
	return out
}

// 关加密时这一格就是密码本身：原样交回，一个字符都不动。
func TestPlainValuePassesPlaintextUntouched(t *testing.T) {
	for _, v := range []string{"pw", " pw ", "带 空格 的口令"} {
		got, problems, err := PlainValue(v, false, true)
		if err != nil || len(problems) > 0 {
			t.Fatalf("关加密时不该有问题：err=%v problems=%v", err, problems)
		}
		if got != v {
			t.Fatalf("原值被改动了：want %q, got %q", v, got)
		}
	}
}

// 开加密时读的是密文文件；文件末尾那个换行不该影响结果。
func TestPlainValueDecodesCipherFile(t *testing.T) {
	t.Setenv(envName, testMaster)
	got, problems, err := PlainValue("testdata/password.enc", true, true)
	if err != nil || len(problems) > 0 {
		t.Fatalf("这份密文该解得开：err=%v problems=%v", err, problems)
	}
	if got != testPlain {
		t.Fatalf("解出来的明文不对：%q", got)
	}
}

// 开了加密、文件不在：这是一条「值这一侧」的问题，不是致命错误——
// 使用者的下一步是去看那个路径，不是去看主密钥。
func TestPlainValueReportsMissingFile(t *testing.T) {
	t.Setenv(envName, testMaster)
	_, problems, err := PlainValue("testdata/根本没有这个文件.enc", true, true)
	if err != nil {
		t.Fatalf("文件不在不该归到致命错误：%v", err)
	}
	if len(problems) != 1 || problems[0].Code != ValuePathMissing {
		t.Fatalf("应当是「文件不在」这一条：%v", codes(problems))
	}
	if problems[0].Shown == "" || problems[0].Detail == "" {
		t.Fatalf("回显的路径与系统那句原因都要带上：%+v", problems[0])
	}
}

// 开了加密、内容却是明文密码：形状不是密文，报 ValueNotCiphertext。
func TestPlainValueReportsNotCiphertext(t *testing.T) {
	t.Setenv(envName, testMaster)
	_, problems, err := PlainValue("testdata/password.txt", true, true)
	if err != nil {
		t.Fatalf("不该归到致命错误：%v", err)
	}
	if len(problems) != 1 || problems[0].Code != ValueNotCiphertext {
		t.Fatalf("应当是「不是密文」这一条：%v", codes(problems))
	}
}

// 没开加密、值却是密文：开关刚被改过，照原样当密码用会登不上。
func TestPlainValueReportsSwitchMismatch(t *testing.T) {
	_, problems, err := PlainValue(testToken, false, true)
	if err != nil {
		t.Fatalf("不该归到致命错误：%v", err)
	}
	if len(problems) != 1 || problems[0].Code != ValueSwitchMismatch {
		t.Fatalf("应当是「开关不匹配」这一条：%v", codes(problems))
	}
}

// 主密钥没读到、密文校验不过：两条都归致命错误，各自可认——
// 它们与「值有问题」是两件事，混成一句会让使用者去修错的那一头。
func TestPlainValueFatalErrorsStayDistinct(t *testing.T) {
	t.Setenv(envName, "")
	if _, _, err := PlainValue("testdata/password.enc", true, true); !errors.Is(err, ErrKeyMissing) {
		t.Fatalf("没有主密钥时应当是 ErrKeyMissing，got %v", err)
	}

	t.Setenv(envName, "另一把主密钥")
	if _, _, err := PlainValue("testdata/password.enc", true, true); !errors.Is(err, ErrBadCipher) {
		t.Fatalf("主密钥不对时应当是 ErrBadCipher，got %v", err)
	}
}

// 形状判定只看形状：密文的形态认得出来，明文的形态认不出来。
func TestIsEncrypted(t *testing.T) {
	if !IsEncrypted(testToken) {
		t.Fatal("这份老工程生成的密文应当判成密文")
	}
	for _, v := range []string{"", "pw", "Sshfleet#2026", "这不是密文"} {
		if IsEncrypted(v) {
			t.Fatalf("%q 不该判成密文", v)
		}
	}
}
