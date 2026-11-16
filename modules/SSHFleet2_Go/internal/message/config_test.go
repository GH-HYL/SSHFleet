package message

import "testing"

// 配置写错那十屏的字（自定义区）：期望值照迁移前 `parse/config.go` 的 `fmt.Sprintf` 逐字抄，
// 连「原因／提示」两行的换行一起钉住——迁的判据就是字节不变。
func TestConfigProblemText(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "env 名字不合法",
			got:  EnvNameInvalid("1BAD"),
			want: "env 里的 \"1BAD\" 不是合法的环境变量名\n" +
				"原因：shell 的 export 只认「字母或下划线开头，后接字母、数字、下划线」的名字，写别的到远端会报 not a valid identifier、变量不生效\n" +
				"提示：改成一个合法名字，例如 MY_VAR",
		},
		{
			name: "env 值以反斜杠结尾",
			got:  EnvValueTrailingBackslash("GOOD", "a\\"),
			want: "env 里的 \"GOOD\" 取值不能以反斜杠结尾，当前值：\"a\\\\\"\n" +
				"原因：值会被包进双引号下发，结尾的反斜杠会吃掉那个收尾引号，远端整条命令变成语法错\n" +
				"提示：要一个反斜杠就写两个",
		},
		{
			name: "command 为空",
			got:  InterpreterCommandEmpty(),
			want: "interpreter.command 不能为空\n" +
				"提示：写命令模式（-c）要用的解释器，例如 bash",
		},
		{
			name: "command 含换行",
			got:  InterpreterCommandNewline("ba\nsh"),
			want: "interpreter.command 不能含换行，当前值：\"ba\\nsh\"",
		},
		{
			name: "script 一条都没有",
			got:  InterpreterScriptMissing(),
			want: "interpreter.script 至少要配一条\n" +
				"提示：写成 \".sh\" = \"bash\" 这样，后缀带点、等号两边都要有值",
		},
		{
			name: "后缀是空的",
			got:  InterpreterSuffixBlank(),
			want: "interpreter.script 里有一条后缀是空的\n" +
				"提示：每条都要写成 \".sh\" = \"bash\"，后缀带点、等号两边都要有值",
		},
		{
			name: "后缀不以点开头",
			got:  InterpreterSuffixNotDotted("sh"),
			want: "interpreter.script 的后缀要以点开头，当前值：\"sh\"\n" +
				"提示：写成 \".sh\"，不要写成 \"sh\"",
		},
		{
			name: "后缀没配解释器",
			got:  InterpreterValueMissing(".sh"),
			want: "interpreter.script 里 \".sh\" 没有配解释器\n" +
				"提示：等号右边要写解释器，例如 \".sh\" = \"bash\"",
		},
		{
			name: "解释器含换行",
			got:  InterpreterValueNewline(".sh", "ba\nsh"),
			want: "interpreter.script 里 \".sh\" 的解释器不能含换行，当前值：\"ba\\nsh\"",
		},
		{
			name: "后缀重复",
			got:  InterpreterSuffixDuplicate(".SH", ".sh"),
			want: "interpreter.script 里后缀重复：\".SH\" 与 \".sh\" 是同一个后缀（大小写不区分）\n" +
				"提示：一个后缀只能配一个解释器",
		},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Fatalf("%s：\n got  %q\n want %q", c.name, c.got, c.want)
		}
	}
}
