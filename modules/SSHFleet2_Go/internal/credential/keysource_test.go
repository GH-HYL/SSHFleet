package credential

import (
	"errors"
	"os"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// 主密钥那一摊（自定义区）：五状态、指纹、封与解、持久值的写。
//
// 期望值一律是字面量：指纹取自 sha256 的已知值，rc 文件的期望内容是手写的那几行——
// 不拿被测代码再算一遍，那样按构造必然通过。

// fake 造一个注进来的来源：环境变量与持久值都由用例给，写持久值只记账不落盘。
func fake(env string, persisted Sources) (*KeySource, *string) {
	written := new(string)
	return &KeySource{
		EnvLookup:       func() string { return env },
		PersistedLookup: func() Sources { return persisted },
		Persist: func(key string) (contract.PersistResult, error) {
			*written = key
			return contract.PersistResult{Where: "/home/u/.bashrc"}, nil
		},
		RCFile: func() string { return "/home/u/.bashrc" },
		Host:   contract.KeyHostPOSIX,
	}, written
}

// 五种状态各判一次，判定次序与老版 InspectKey 相同。
func TestInspectFiveStates(t *testing.T) {
	persisted := Sources{Persisted: "PERSISTED", PersistedWhere: "/home/u/.bashrc"}
	cases := []struct {
		name string
		env  string
		src  Sources
		want contract.KeyState
	}{
		{"两处都有且不同", "ENVONLY", persisted, contract.KeyStateDiverged},
		{"两处都有且相同", "SAME", Sources{Persisted: "SAME", PersistedWhere: "/home/u/.bashrc"}, contract.KeyStateOK},
		{"只剩持久值", "", persisted, contract.KeyStateStaleShell},
		{"只剩环境变量", "ENVONLY", Sources{}, contract.KeyStateUnpersisted},
		{"两处都空", "", Sources{}, contract.KeyStateUnset},
		// 两侧 rc 文件互相不同也算不一致：换 shell 之后才会爆发的那种分叉。
		{"两侧 rc 不同", "PERSISTED", Sources{
			Persisted: "PERSISTED", PersistedWhere: "/home/u/.bashrc",
			OtherKey: "OTHER", OtherWhere: "/home/u/.zshrc",
		}, contract.KeyStateDiverged},
	}
	for _, c := range cases {
		src, _ := fake(c.env, c.src)
		if got := src.Inspect().State; got != c.want {
			t.Errorf("%s：want %q，got %q", c.name, c.want, got)
		}
	}
}

// 指纹是 sha256 前 4 字节的十六进制：拿一个已知的 sha256 值对手写的那八位。
func TestFingerprintIsEightHexChars(t *testing.T) {
	// sha256("abc") = ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad
	if got := Fingerprint("abc"); got != "ba7816bf" {
		t.Fatalf(`指纹不对：want "ba7816bf"，got %q`, got)
	}
	if got := Fingerprint("   "); got != "（未设置）" {
		t.Fatalf("空密钥要印「（未设置）」，got %q", got)
	}
	// 首尾空白不算另一把：rc 里 export 出来的值常带空白。
	if Fingerprint(" abc ") != Fingerprint("abc") {
		t.Fatalf("首尾空白不该改变指纹")
	}
}

// 封了能解回：字面量往返，不拿解密算期望值。
func TestEncryptDecryptRoundTrip(t *testing.T) {
	const plain = "P@ssw0rd-中文"
	token, err := Encrypt(plain, "master-1")
	if err != nil {
		t.Fatalf("封不起来：%v", err)
	}
	if token == plain || strings.Contains(token, plain) {
		t.Fatalf("密文里不该出现明文")
	}
	back, err := Decrypt(token, "master-1")
	if err != nil {
		t.Fatalf("解不开：%v", err)
	}
	if back != plain {
		t.Fatalf("解回来不是原样：%q", back)
	}
	if _, err := Decrypt(token, "master-2"); !errors.Is(err, ErrBadCipher) {
		t.Fatalf("换一把密钥应当解不开，got %v", err)
	}
}

// 生成的密钥：约 48 字符、两次不同。
func TestGenerateMasterKeyShape(t *testing.T) {
	a, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("生成失败：%v", err)
	}
	b, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("生成失败：%v", err)
	}
	if len(a) != 48 {
		t.Fatalf("长度应当是 48（36 字节 base64url 无填充），got %d：%q", len(a), a)
	}
	if a == b {
		t.Fatalf("两次生成不该相同")
	}
}

// 预检查：只在加密开着、且状态是那三种之一时才说话。
func TestPrecheckSpeaksOnlyWhenNeeded(t *testing.T) {
	diverged := Sources{Persisted: "P", PersistedWhere: "/home/u/.bashrc"}

	src, _ := fake("E", diverged)
	if _, need := src.Precheck(true); !need {
		t.Fatalf("不一致时该说话")
	}
	if _, need := src.Precheck(false); need {
		t.Fatalf("加密关着时一个字都不该说")
	}

	src, _ = fake("SAME", Sources{Persisted: "SAME", PersistedWhere: "/home/u/.bashrc"})
	if _, need := src.Precheck(true); need {
		t.Fatalf("两处一致时不该说话")
	}

	src, _ = fake("", Sources{})
	if _, need := src.Precheck(true); need {
		t.Fatalf("两处都空时不该说话（老版 PrecheckKey 只有三支）")
	}
}

// Write 交回的就是来源写的那份结果。
func TestWriteDelegates(t *testing.T) {
	src, written := fake("", Sources{})
	res, err := src.Write("K1")
	if err != nil {
		t.Fatalf("写失败：%v", err)
	}
	if *written != "K1" || res.Where != "/home/u/.bashrc" {
		t.Fatalf("写持久值没有走到注入的那个函数：key=%q res=%+v", *written, res)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("写 %s 失败：%v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败：%v", path, err)
	}
	return string(data)
}
