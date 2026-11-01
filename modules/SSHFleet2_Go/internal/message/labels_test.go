package message

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 三段话的标签只许住构造点（ADR-0028 决定五、ADR-0034 决定八）。
//
// 函数管得住愿意用的人，管不住绕开的人：Go 里自己拼一段中文标签照样编译得过。所以要有这条
// 判据——扫本模块源码，除了 `message` 自己与 `_test.go`，谁都不许出现这三个标签。
//
// 判据认三个标签（原因／提示／出处）的两种写法：紧跟在 `\n` 后面（拆行拼接），或者紧跟在一个
// 引号后面（新起一行的字面量）。只认这两种——散文里的「一条提示：」「唯一出处：」不算拼话，
// 认宽了只会逼着改注释。
func TestLabelsOnlyInConstructorPoint(t *testing.T) {
	labels := []string{"原因：", "提示：", "出处："}
	// 两种拼法：写在字符串里的换行之后，或者一个字面量一开头。
	prefixes := []string{`\n`, `"`, "`"}
	root := ".." // internal/

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "message" && filepath.Dir(path) == root {
				return fs.SkipDir // 构造点自己
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		return checkFile(t, path, labels, prefixes)
	})
	if err != nil {
		t.Fatalf("扫源码失败：%v", err)
	}

	// 塔尖在模块根上，不在 internal/ 下面，单独扫一遍。
	if err := checkFile(t, "../../main.go", labels, prefixes); err != nil {
		t.Fatalf("扫塔尖失败：%v", err)
	}
}

// checkFile 一份文件里不许出现那三个标签。
func checkFile(t *testing.T, path string, labels, prefixes []string) error {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := string(data)
	for _, label := range labels {
		for _, pre := range prefixes {
			if strings.Contains(src, pre+label) {
				t.Errorf("%s 里出现了拼好的标签 %q —— 标签只许在 message 里加", path, pre+label)
			}
		}
	}
	return nil
}
