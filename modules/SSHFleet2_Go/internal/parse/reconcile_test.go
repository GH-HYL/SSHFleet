package parse

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// config/ 对账（母规格 Testing Decisions）：新工程随包那份与老工程那份，
// 每个文件的「节名 + 键名」集合逐条相等；值不比——值随环境配，比了就天天红。
// 认键的规则只有一份：直接用 parseRawConfig，不另写一套。
//
// 新工程改过名的节由 `keyRenames` 单点记着：对账要的是「这份料一个字没少」，
// 不是「名字一辈子不变」。

// configDirPair 返回新旧两份 config 目录的路径（按本文件位置回溯到仓库根）。
func configDirPair(t *testing.T) (string, string) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("定位不到本文件")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	return filepath.Join(repoRoot, "modules", "SSHFleet2_Go", "config"),
		filepath.Join(repoRoot, "modules", "SSHFleet_Go", "config")
}

// sectionRenames 新工程相对老工程改过名的节：文件名 → 新名 → 老名。
//
// 2026-11-03：`keywords_error.conf` 的 `[[categories]]`／`[[exit_code_categories]]` 改成单数，
// 与 `keywords_passwd.conf` 的 `[[category]]` 同一套写法。老工程不动，所以对账时把新名折回老名。
// 只对那一份文件生效——改密那份本来就是单数，一并折会把它折错。
var sectionRenames = map[string]map[string]string{
	"keywords_error.conf": {
		"category":           "categories",
		"exit_code_category": "exit_code_categories",
	},
}

// contentSet 一个配置文件拿去对账的内容集合：
// 键值形态的文件取「节名 + 节.键」；不是键值形态的（危险规则那份是一行一条正则）
// 取裁掉注释与空白后的原文行——对账的是「这份料一个字没少」，不是它的语法。
//
// renames 非空时按它折名（把新名折回老名），只作用于节名。
func contentSet(t *testing.T, path string, renames map[string]string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败：%v", path, err)
	}
	lines, perr := tokenize(data)
	if perr == nil {
		raw := rawView(lines)
		var out []string
		for section, kv := range raw {
			if old, ok := renames[section]; ok {
				section = old
			}
			out = append(out, section)
			for key := range kv {
				out = append(out, section+"."+key)
			}
		}
		slices.Sort(out)
		return out
	}
	// 行形态的兜底：非空、非注释的行，裁掉两端空白后整行参与对账
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	slices.Sort(out)
	return out
}

func TestConfigDirReconciles(t *testing.T) {
	newDir, oldDir := configDirPair(t)

	// 文件清单先对上：多一份少一份都红
	list := func(dir string) []string {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("读目录 %s 失败：%v", dir, err)
		}
		var out []string
		for _, e := range entries {
			if !e.IsDir() {
				out = append(out, e.Name())
			}
		}
		slices.Sort(out)
		return out
	}
	newFiles, oldFiles := list(newDir), list(oldDir)
	if !slices.Equal(newFiles, oldFiles) {
		t.Fatalf("两份 config 的文件清单不一致：\n新：%v\n老：%v", newFiles, oldFiles)
	}

	for _, name := range newFiles {
		newKeys := contentSet(t, filepath.Join(newDir, name), sectionRenames[name])
		oldKeys := contentSet(t, filepath.Join(oldDir, name), nil)
		missing, extra := diff(newKeys, oldKeys)
		if len(missing) > 0 || len(extra) > 0 {
			t.Errorf("%s 的内容集合不一致：\n  新工程缺：%v\n  新工程多：%v", name, missing, extra)
		}
	}
}

// diff 有序集合的差集：a 有 b 没有 → extra；b 有 a 没有 → missing。
func diff(a, b []string) (missing, extra []string) {
	setB := map[string]bool{}
	for _, k := range b {
		setB[k] = true
	}
	for _, k := range a {
		if !setB[k] {
			extra = append(extra, k)
		}
	}
	setA := map[string]bool{}
	for _, k := range a {
		setA[k] = true
	}
	for _, k := range b {
		if !setA[k] {
			missing = append(missing, k)
		}
	}
	return missing, extra
}
