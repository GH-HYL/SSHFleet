package execute

// 两条远端循环的命令形态：不提权与提权各长什么样。
//
// 钉的是 `sudo -n for …` 那个坑：`for` 是 shell 关键字不是可执行文件，真 bash 对它报
// `syntax error near unexpected token 'do'`、退出码 2、一条都没执行；而收尾那一路不看
// 退出码，于是会把「什么都没删」报成「删干净了」。所以提权必须包一层 `sh -c`。

import (
	"strings"
	"testing"
)

func TestRemoteLoopsWrapSudoCommandInShell(t *testing.T) {
	paths := []string{"/opt/app/a.txt", "/opt/app/b c.txt"}

	for _, c := range []struct {
		name string
		loop func([]string, bool) string
	}{
		{name: "收尾", loop: cleanupLoop},
		{name: "判重", loop: dedupeLoop},
	} {
		plain := c.loop(paths, false)
		sudo := c.loop(paths, true)

		const prefix = "sudo -n sh -c "
		if !strings.HasPrefix(sudo, prefix) {
			t.Fatalf("%s：提权那一路该用 `sh -c` 包起来，实际 %q", c.name, sudo)
		}
		// 剥掉外壳之后里头那条要与不提权时一字不差：包的是同一条命令，不是另写一条。
		if inner := unquote(strings.TrimPrefix(sudo, prefix)); inner != plain {
			t.Fatalf("%s：包起来之后里头那条不一致\ngot  %q\nwant %q", c.name, inner, plain)
		}
	}
}
