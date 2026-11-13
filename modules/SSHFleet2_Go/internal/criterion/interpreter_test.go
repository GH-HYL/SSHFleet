package criterion

import "testing"

// 归一只认名字与版本号：`/` 是目标机上的分隔符，`\` 不是。本机路径写法归不出来，
// 两处（确认屏、`-a`）于是都照实说「不认」——同一个结论，且错在解析期就暴露。

func TestInterpreterBaseName(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"   ", ""},
		{"bash", "bash"},
		{"/bin/bash", "bash"},
		{"bash5", "bash"},
		{"perl -w", "perl"},
		{"/usr/bin/python3", "python"},
		{"php8.1", "php"},
		{"zsh5", "zsh"},
		{`C:\Python\bash`, `c:\python\bash`}, // 解释器跑在目标机上，`\` 不是它的分隔符：归不出 bash
		{`C:\bin\zsh5`, `c:\bin\zsh`},        // 归不出目录，版本号仍照剥（老版就这样）
		{"bash.exe", "bash.exe"},             // 带扩展名的写法两版都不认，钉住现状
	} {
		if got := InterpreterBaseName(c.in); got != c.want {
			t.Errorf("InterpreterBaseName(%q) = %q，想要 %q", c.in, got, c.want)
		}
	}
}
