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

// 配置那一屏剩下的六句（06 号单）：问题行加一句提示，逐字照迁移前 `parse/config.go` 的字面量。
func TestConfigScreenText(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "[paths] 段",
			got:  ConfigPathsSectionForbidden(),
			want: "配置里不允许出现 [paths] 段\n" +
				"提示：把 [paths] 整段删掉即可，其余字段不用动",
		},
		{
			name: "[execution.env] 段",
			got:  ConfigExecutionEnvSectionRenamed(),
			want: "配置里的 [execution.env] 段要改名为 [env]\n" +
				"提示：把段名改成 [env] 即可——挪到顶层，里面一个个变量不用动",
		},
		{
			name: "未识别字段",
			got:  ConfigUnknownFields("account.bogus"),
			want: "配置包含未识别字段：account.bogus\n" +
				"提示：请使用随包提供的配置文件，不要增删任何字段",
		},
		{
			name: "缺少必填字段",
			got:  ConfigMissingFields("upload.medium_parallel"),
			want: "配置缺少必填字段：upload.medium_parallel\n" +
				"提示：请使用随包提供的配置文件，不要删掉里面的任何一行",
		},
		{
			name: "配置文件不在当前目录",
			got:  ConfigNotInWorkDir("config/SSHFleet.conf", `C:\work`),
			want: "这个位置没有配置文件\n" +
				"      应该在这里：config/SSHFleet.conf\n" +
				"      当前目录：C:\\work\n" +
				"提示：请在本工具的解压目录下执行；配置文件必须放在该目录的 config 文件夹里",
		},
		{
			name: "字符串值没带引号",
			got:  ConfigStringValueUnquoted("account.user", "root"),
			want: "account.user 取值非法：root（字符串值要带引号）\n" +
				"提示：给这一格的值加上成对的引号（'…' 或 \"…\"）即可；数字与布尔不用加",
		},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Fatalf("%s：\n got  %q\n want %q", c.name, c.got, c.want)
		}
	}
}
