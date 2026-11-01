package guard

import (
	"fmt"
	"os"
	"strings"
)

// Exception 是放行台账的一行：哪条红线、哪个位置、为什么放、谁批的。
//
// 台账能让检查闭嘴，权力与 .gitignore 同级：改它等同改约束类文件，要开发者单独点头
// （07 号工单答案第七条）。
type Exception struct {
	Rule  string // 类名，与报错方括号里那一串同写法
	Where string // 模块内相对路径；写目录就当整个目录放行
	Why   string
	Who   string
}

// Match 判这条放行是不是盖住了某条违规：规则同名，且位置落在放行的那一段里。
func (e Exception) Match(v Violation) bool {
	if e.Rule != v.Class {
		return false
	}
	where := strings.TrimSuffix(e.Where, "/")
	return where != "" && (v.File == where || strings.HasPrefix(v.File, where+"/"))
}

// LoadLedger 读放行台账。文件不在就报错——台账读不到时静默放过，等于整套检查都可能没在跑。
func LoadLedger(path string) ([]Exception, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读不到放行台账 %s：%w", path, err)
	}
	var out []Exception
	header := false
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := cells(line)
		if len(cells) < 4 {
			continue
		}
		if !header {
			header = true // 表头那一行
			continue
		}
		if strings.HasPrefix(cells[0], "---") {
			continue
		}
		out = append(out, Exception{Rule: cells[0], Where: cells[1], Why: cells[2], Who: cells[3]})
	}
	return out, nil
}

func cells(line string) []string {
	parts := strings.Split(strings.Trim(line, "|"), "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

// Apply 拿台账过一遍违规：命中的不报，顺带交出没盖住任何违规的那几行。
//
// 交出「没用的那一行」不是多管闲事：台账烂着的典型长相就是条目越攒越多、谁也不敢删，
// 到最后没人再看它。
func Apply(vs []Violation, ledger []Exception) (kept []Violation, stale []Exception) {
	used := make([]bool, len(ledger))
	for _, v := range vs {
		hit := false
		for i, e := range ledger {
			if !e.Match(v) {
				continue
			}
			hit = true
			used[i] = true
		}
		if !hit {
			kept = append(kept, v)
		}
	}
	for i, e := range ledger {
		if !used[i] {
			stale = append(stale, e)
		}
	}
	return kept, stale
}
