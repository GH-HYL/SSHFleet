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

// 三条命令的流程（自定义区）：真跑一遍，看的是使用者看得见的结果——rc 文件变没变、
// 凭据文件转没转、起不来的那一屏说了什么。
//
// 主密钥来源用运行期那一个（`DefaultKeySource`），但 HOME 指到临时目录：跑测试不许写坏
// 本机。注册表那一侧在 Linux 上根本不会被走到。

// setupHome 造一个空主目录并把它当 HOME；环境变量里的主密钥清空。
func setupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")
	t.Setenv(credential.EnvName, "")
	return home
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

// seedRC 往 .bashrc 里放一把已保存的主密钥。
func seedRC(t *testing.T, home, key string) string {
	t.Helper()
	rc := filepath.Join(home, ".bashrc")
	body := "# SSHFleet 主密钥\nexport SSHFLEET_KEY='" + key + "'\n"
	if err := os.WriteFile(rc, []byte(body), 0o644); err != nil {
		t.Fatalf("写 rc 失败：%v", err)
	}
	return rc
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败：%v", path, err)
	}
	return string(data)
}

// 空现场跑 --gen-key：写完就退，rc 末尾出现唯一一条主密钥行。
func TestGenKeyWritesPersistedKey(t *testing.T) {
	home := setupHome(t)
	writeConfig(t, true)

	if cannot := Run(contract.KeyRun{Which: contract.KeyToolGen}, credential.DefaultKeySource()); cannot != nil {
		t.Fatalf("空现场不该起不来：%+v", cannot)
	}

	body := readFile(t, filepath.Join(home, ".bashrc"))
	if got := strings.Count(body, "export SSHFLEET_KEY="); got != 1 {
		t.Fatalf("rc 里应当只有一条主密钥行，实际 %d 条：%q", got, body)
	}
	line := strings.TrimSpace(strings.Split(strings.TrimSpace(body), "\n")[1])
	key := strings.Trim(strings.TrimPrefix(line, "export SSHFLEET_KEY="), "'")
	if len(key) != 48 {
		t.Fatalf("生成的密钥应当是 48 字符，实际 %d：%q", len(key), key)
	}
}

// 已有密钥、加 --yes：拒绝自动覆盖，rc 一字不改。
func TestGenKeyYesRefusesOverwrite(t *testing.T) {
	home := setupHome(t)
	writeConfig(t, true)
	rc := seedRC(t, home, "KEEP-ME")
	before := readFile(t, rc)

	cannot := Run(contract.KeyRun{Which: contract.KeyToolGen, Yes: true}, credential.DefaultKeySource())
	if cannot == nil {
		t.Fatalf("已有密钥时加 --yes 应当是起不来")
	}
	if !strings.Contains(cannot.Body, "非交互模式不自动覆盖") {
		t.Fatalf("那一屏要说清非交互不覆盖：%q", cannot.Body)
	}
	if cannot.Where != "credential" {
		t.Fatalf("环节名沿用老版 credential，got %q", cannot.Where)
	}
	if after := readFile(t, rc); after != before {
		t.Fatalf("拒绝覆盖时 rc 不该变：\n before %q\n after  %q", before, after)
	}
}

// 明文转密文、再转回明文：转完能解回去，长度与方向都写在屏上（屏上的话由 message 测）。
func TestConvertBothDirections(t *testing.T) {
	setupHome(t)
	t.Setenv(credential.EnvName, "FIXED-MASTER-KEY")
	writeConfig(t, true)

	path := filepath.Join(t.TempDir(), "sec.txt")
	const plain = "P@ssw0rd-中文"
	if err := os.WriteFile(path, []byte(plain), 0o644); err != nil {
		t.Fatalf("写凭据文件失败：%v", err)
	}

	if cannot := Run(contract.KeyRun{Which: contract.KeyToolConvert, Path: path}, credential.DefaultKeySource()); cannot != nil {
		t.Fatalf("加密方向不该起不来：%+v", cannot)
	}
	cipher := readFile(t, path)
	if !credential.IsEncrypted(cipher) {
		t.Fatalf("转完应当是密文：%q", cipher)
	}

	writeConfig(t, false)
	if cannot := Run(contract.KeyRun{Which: contract.KeyToolConvert, Path: path}, credential.DefaultKeySource()); cannot != nil {
		t.Fatalf("解密方向不该起不来：%+v", cannot)
	}
	if got := readFile(t, path); got != plain {
		t.Fatalf("转回来不是原文：%q", got)
	}
}

// 已经是目标形态：不覆盖、不报错。
func TestConvertLeavesTargetFormAlone(t *testing.T) {
	setupHome(t)
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

	if cannot := Run(contract.KeyRun{Which: contract.KeyToolConvert, Path: path}, credential.DefaultKeySource()); cannot != nil {
		t.Fatalf("已是密文不该起不来：%+v", cannot)
	}
	if got := readFile(t, path); got != token {
		t.Fatalf("已是密文时不该动它：%q", got)
	}
}

// 两处主密钥不一致时加密方向被拦：文件不改，屏上说清两条出路。
func TestConvertBlocksEncryptWhenDiverged(t *testing.T) {
	home := setupHome(t)
	writeConfig(t, true)
	seedRC(t, home, "PERSISTED-KEY")
	t.Setenv(credential.EnvName, "OTHER-KEY")

	path := filepath.Join(t.TempDir(), "sec.txt")
	const plain = "P@ssw0rd"
	if err := os.WriteFile(path, []byte(plain), 0o644); err != nil {
		t.Fatalf("写凭据文件失败：%v", err)
	}

	cannot := Run(contract.KeyRun{Which: contract.KeyToolConvert, Path: path}, credential.DefaultKeySource())
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
	setupHome(t)
	writeConfig(t, true)

	missing := filepath.Join(t.TempDir(), "没有这个文件.txt")
	cannot := Run(contract.KeyRun{Which: contract.KeyToolConvert, Path: missing}, credential.DefaultKeySource())
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
