package result

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// 语料回归（常驻）：旧工程 test/test_error_classification.py 的 30 条用例 + 配置归类断言。

type corpusCase struct {
	Name         string `toml:"name"`
	ExitCode     *int   `toml:"exit_code"`
	Error        string `toml:"error"`
	Output       string `toml:"output"`
	Mode         string `toml:"mode"`
	SuccessFiles int    `toml:"success_files"`
	FailedFiles  int    `toml:"failed_files"`
	Expected     string `toml:"expected"`
}

type corpusFile struct {
	Cases []corpusCase `toml:"cases"`
}

func loadKeywords(t *testing.T) *Keywords {
	t.Helper()
	kw, err := LoadKeywords(filepath.Join("..", "..", "config", "error_keywords.toml"))
	if err != nil {
		t.Fatalf("关键词文件应可加载: %v", err)
	}
	return kw
}

func TestErrorClassificationCorpus(t *testing.T) {
	kw := loadKeywords(t)

	var file corpusFile
	if _, err := toml.DecodeFile(filepath.Join("..", "..", "testdata", "error_classification_cases.toml"), &file); err != nil {
		t.Fatal(err)
	}

	passed := 0
	for _, c := range file.Cases {
		got := Classify(Case{
			ExitCode:     c.ExitCode,
			Error:        c.Error,
			Output:       c.Output,
			Mode:         c.Mode,
			SuccessFiles: c.SuccessFiles,
			FailedFiles:  c.FailedFiles,
		}, kw)
		if got == c.Expected {
			passed++
			continue
		}
		t.Errorf("语料不通过：%s\n  期望 %q\n  实际 %q", c.Name, c.Expected, got)
	}
	t.Logf("错误分类语料：%d/%d 通过", passed, len(file.Cases))
	if passed != len(file.Cases) {
		t.Fatalf("分类行为与旧实现不一致：%d/%d", passed, len(file.Cases))
	}
}

// TestKeywordPlacement 配置归类断言（对位旧语料的三个布尔检查）。
func TestKeywordPlacement(t *testing.T) {
	kw := loadKeywords(t)

	authLower := lowerAll(kw.KeywordsOf("身份验证失败"))
	if contains(authLower, "permission denied") {
		t.Error("permission denied 不应在「身份验证失败」分类里")
	}
	if !contains(kw.KeywordsOf("权限不足"), "permission denied") {
		t.Error("permission denied 应在「权限不足」分类里")
	}
	if !contains(authLower, "permission denied (publickey") {
		t.Error("认证后缀串应在「身份验证失败」分类里")
	}
}

// TestFallbackCategory 兜底判定（终端单行显示用）。
func TestFallbackCategory(t *testing.T) {
	kw := loadKeywords(t)
	unknownErr := "disk quota exceeded on /var/log"

	cases := []struct {
		category string
		want     bool
	}{
		{"SFTP失败", false},
		{"执行失败(退出码2)", false},
		{"部分成功", false},
		{"错误未分类", false},
		{unknownErr, true},
		{"", false},
	}
	for _, c := range cases {
		if got := IsFallbackCategory(c.category, kw); got != c.want {
			t.Errorf("IsFallbackCategory(%q) = %v，期望 %v", c.category, got, c.want)
		}
	}
}

// TestAuthFailureTakesPrecedence 认证失败分类优先于关键词推断（spec D44）。
func TestAuthFailureTakesPrecedence(t *testing.T) {
	kw := loadKeywords(t)
	noPwd := "ssh: handshake failed: ssh: unable to authenticate, attempted methods [none publickey], no supported methods remain"

	// 落在关键词上时是按关键词分类（仅公钥被拒 → 密钥不匹配）
	if got := Classify(Case{Error: noPwd, Mode: "execute"}, kw); got != "密钥不匹配" {
		t.Fatalf("仅公钥被拒应归「密钥不匹配」，实为 %q", got)
	}
	// 带上认证失败分类字段时，分类字段优先
	got := Classify(Case{Error: noPwd, AuthFailure: "密钥与密码均失败", Mode: "execute"}, kw)
	if got != "密钥与密码均失败" {
		t.Fatalf("认证失败分类应优先，实为 %q", got)
	}
}

func lowerAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, strings.ToLower(s))
	}
	return out
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
