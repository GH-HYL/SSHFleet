package guard

import "testing"

// 真模块要数出十七个包，塔尖对的是模块名那一行（不是 main）。
func TestLoadCountsPackages(t *testing.T) {
	m, err := Load(Target{Dir: "../..", ModulePath: "sshfleet2", Rules: Rules()})
	if err != nil {
		t.Fatalf("读真模块失败：%v", err)
	}
	if len(m.Packages) != 17 {
		t.Fatalf("包数 want 17，got %d", len(m.Packages))
	}
	tip := m.Pkg("sshfleet2")
	if tip == nil {
		t.Fatal("塔尖那一行对不上：规则表里叫 sshfleet2，模块里没找到")
	}
	if tip.Name != "main" {
		t.Errorf("塔尖包名 want main，got %s", tip.Name)
	}
}

// testdata、点开头、下划线开头的目录不扫——与 go list ./... 同口径。
func TestLoadSkipsDirs(t *testing.T) {
	m := loadFixture(t, "clean")
	for _, p := range m.Packages {
		if p.Short == "testdata" || p.Short == "_hidden" || p.Short == ".git" {
			t.Fatalf("扫进了不该扫的目录：%s", p.Path)
		}
	}
	if len(m.Packages) != 7 {
		t.Fatalf("clean 样本 want 7 个包，got %d", len(m.Packages))
	}
}

// 判据按语法树数：注释里的 fmt.Println、字符串里的 os.Exit 都不算越界。
func TestCommentAndStringAreNotViolations(t *testing.T) {
	m := loadFixture(t, "clean")
	vs := CheckAll(m)
	if len(vs) != 0 {
		t.Fatalf("clean 样本应当零违规，got：\n%s", Report(vs))
	}
}
