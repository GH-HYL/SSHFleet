package keytool

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/credential"
)

// 三条命令的流程（自定义区）：真跑一遍，看的是使用者看得见的结果——有没有写出去、
// 凭据文件转没转、起不来的那一屏说了什么。
//
// **主密钥来源一律注入**：持久值那一摊是假件，「写到哪」只是测试临时目录里的一个路径，
// 不碰本机 rc 文件、不碰注册表——`setx` 那条路在 Windows 上会真改使用者的环境变量
// （规格第十二节：自动化里一律是注进去的假件）。只有主密钥本身走真实环境变量
// （`credential.MasterKey` 读它），那是进程内的一次读，不落盘。

// fakeSource 造一个注进来的来源：环境变量、本机持久值、写出去的那把，全由用例给。
type fakeSource struct {
	env       string
	persisted string
	rcPath    string
	written   string // 写持久值时收到的密钥；空表示没写过
}

func (f *fakeSource) source() *credential.KeySource {
	return &credential.KeySource{
		EnvLookup: func() string { return f.env },
		PersistedLookup: func() credential.Sources {
			if f.persisted == "" {
				return credential.Sources{}
			}
			return credential.Sources{Persisted: f.persisted, PersistedWhere: f.rcPath}
		},
		Persist: func(key string) (contract.PersistResult, error) {
			f.written = key
			return contract.PersistResult{Where: f.rcPath}, nil
		},
		RCFile: func() string { return f.rcPath },
		Host:   contract.KeyHostPOSIX,
	}
}

// newFake 造一份假来源并把环境里的主密钥清空；rcPath 指到测试临时目录。
func newFake(t *testing.T) *fakeSource {
	t.Helper()
	t.Setenv(credential.EnvName, "")
	return &fakeSource{rcPath: filepath.Join(t.TempDir(), ".bashrc")}
}

// writeConfig 造一份只带 `[credential] encrypt` 那一格的配置，并把工作目录切过去。
func writeConfig(t *testing.T, encrypt bool) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o755); err != nil {
		t.Fatalf("建配置目录失败：%v", err)
	}
	body := "[credential]\nencrypt = " + strconv.FormatBool(encrypt) + "   # 加密开关\n"
	if err := os.WriteFile(filepath.Join(dir, "config", "SSHFleet.conf"), []byte(body), 0o644); err != nil {
		t.Fatalf("写配置失败：%v", err)
	}
	t.Chdir(dir)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败：%v", path, err)
	}
	return string(data)
}

// 空现场跑 --gen-key：写好就退，交出去的是一把 48 字符的新密钥。
func TestGenKeyWritesPersistedKey(t *testing.T) {
	fake := newFake(t)
	writeConfig(t, true)

	if cannot := Run(contract.KeyRun{Which: contract.KeyToolGen}, fake.source()); cannot != nil {
		t.Fatalf("空现场不该起不来：%+v", cannot)
	}
	if len(fake.written) != 48 {
		t.Fatalf("写出去的密钥应当是 48 字符，实际 %d：%q", len(fake.written), fake.written)
	}
}

// 已有密钥、加 --yes：拒绝自动覆盖，一个字节都不写出去。
func TestGenKeyYesRefusesOverwrite(t *testing.T) {
	fake := newFake(t)
	fake.persisted = "KEEP-ME"
	writeConfig(t, true)

	cannot := Run(contract.KeyRun{Which: contract.KeyToolGen, Yes: true}, fake.source())
	if cannot == nil {
		t.Fatalf("已有密钥时加 --yes 应当是起不来")
	}
	if !strings.Contains(cannot.Body, "非交互模式不自动覆盖") {
		t.Fatalf("那一屏要说清非交互不覆盖：%q", cannot.Body)
	}
	if cannot.Where != "credential" {
		t.Fatalf("环节名沿用老版 credential，got %q", cannot.Where)
	}
	if fake.written != "" {
		t.Fatalf("拒绝覆盖时不该写持久值，实际写了 %q", fake.written)
	}
}

// 明文转密文、再转回明文：转完能解回去。
func TestConvertBothDirections(t *testing.T) {
	fake := newFake(t)
	t.Setenv(credential.EnvName, "FIXED-MASTER-KEY")
	writeConfig(t, true)

	path := filepath.Join(t.TempDir(), "sec.txt")
	const plain = "P@ssw0rd-中文"
	if err := os.WriteFile(path, []byte(plain), 0o644); err != nil {
		t.Fatalf("写凭据文件失败：%v", err)
	}

	if cannot := Run(contract.KeyRun{Which: contract.KeyToolConvert, Path: path}, fake.source()); cannot != nil {
		t.Fatalf("加密方向不该起不来：%+v", cannot)
	}
	cipher := readFile(t, path)
	if !credential.IsEncrypted(cipher) {
		t.Fatalf("转完应当是密文：%q", cipher)
	}

	writeConfig(t, false)
	if cannot := Run(contract.KeyRun{Which: contract.KeyToolConvert, Path: path}, fake.source()); cannot != nil {
		t.Fatalf("解密方向不该起不来：%+v", cannot)
	}
	if got := readFile(t, path); got != plain {
		t.Fatalf("转回来不是原文：%q", got)
	}
}

// 已经是目标形态：不覆盖、不报错。
func TestConvertLeavesTargetFormAlone(t *testing.T) {
	fake := newFake(t)
	t.Setenv(credential.EnvName, "FIXED-MASTER-KEY")
	writeConfig(t, true)

	path := filepath.Join(t.TempDir(), "sec.txt")
	token, err := credential.Encrypt("P@ssw0rd", "FIXED-MASTER-KEY")
	if err != nil {
		t.Fatalf("造密文失败：%v", err)
	}
	if err := os.WriteFile(path, []byte(token), 0o644); err != nil {
		t.Fatalf("写凭据文件失败：%v", err)
	}

	if cannot := Run(contract.KeyRun{Which: contract.KeyToolConvert, Path: path}, fake.source()); cannot != nil {
		t.Fatalf("已是密文不该起不来：%+v", cannot)
	}
	if got := readFile(t, path); got != token {
		t.Fatalf("已是密文时不该动它：%q", got)
	}
}

// 两处主密钥不一致时加密方向被拦：文件不改，屏上说清两条出路。
func TestConvertBlocksEncryptWhenDiverged(t *testing.T) {
	fake := newFake(t)
	fake.persisted = "PERSISTED-KEY"
	fake.env = "OTHER-KEY"
	writeConfig(t, true)

	path := filepath.Join(t.TempDir(), "sec.txt")
	const plain = "P@ssw0rd"
	if err := os.WriteFile(path, []byte(plain), 0o644); err != nil {
		t.Fatalf("写凭据文件失败：%v", err)
	}

	cannot := Run(contract.KeyRun{Which: contract.KeyToolConvert, Path: path}, fake.source())
	if cannot == nil {
		t.Fatalf("不一致时加密方向应当被拦")
	}
	for _, want := range []string{"已阻止加密", "出路", "① 执行 source"} {
		if !strings.Contains(cannot.Body, want) {
			t.Fatalf("那一屏缺「%s」：%q", want, cannot.Body)
		}
	}
	if got := readFile(t, path); got != plain {
		t.Fatalf("被拦下时文件不该改：%q", got)
	}
}

// 路径写错：报「读不到那个文件」，不是「主密钥不匹配」。
func TestConvertReportsMissingFile(t *testing.T) {
	fake := newFake(t)
	writeConfig(t, true)

	missing := filepath.Join(t.TempDir(), "没有这个文件.txt")
	cannot := Run(contract.KeyRun{Which: contract.KeyToolConvert, Path: missing}, fake.source())
	if cannot == nil {
		t.Fatalf("文件不在应当起不来")
	}
	if !strings.Contains(cannot.Body, "读取凭据文件失败："+missing) {
		t.Fatalf("要说清是读不到那个文件：%q", cannot.Body)
	}
	if strings.Contains(cannot.Body, "主密钥不匹配") {
		t.Fatalf("不该把读不到说成密钥不对：%q", cannot.Body)
	}
}
