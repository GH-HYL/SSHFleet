package message

import "testing"

// 配置里路径用不了那一族（自定义区）：五类毛病各钉一条，外加「本机是哪一种系统」翻转提示那格。
// 期望值照迁移前 `parse/config.go` 的 `pathErr` 拼出来，一个字节没改。
func TestConfigPathProblem(t *testing.T) {
	cases := []struct {
		name    string
		field   string
		raw     string
		kind    PathProblemKind
		windows bool
		want    string
	}{
		{
			name:  "写 ~ 开头",
			field: "credential.key", raw: "~/k", kind: PathTilde,
			want: "credential.key 不支持 ~，当前值：~/k\n" +
				"原因：~ 不会被展开，写什么就按什么找\n" +
				"提示：写成以 / 开头的本机路径，例如 /home/ops/.keys",
		},
		{
			name:  "Windows 上写没有盘符的路径",
			field: "credential.key", raw: "/opt/k", kind: PathNoDrive, windows: true,
			want: "credential.key 不是本机的绝对路径，当前值：/opt/k\n" +
				"原因：本机是 Windows，这个写法没有盘符\n" +
				"提示：写成带盘符的本机路径，例如 D:/Keys",
		},
		{
			name:  "相对路径",
			field: "credential.key", raw: "opt/k", kind: PathRelative,
			want: "credential.key 必须是绝对路径，当前值：opt/k\n" +
				"原因：配置里的路径一律写全路径；相对路径只在清单里能用\n" +
				"提示：写成以 / 开头的本机路径，例如 /home/ops/.keys",
		},
		{
			name:  "相对路径（本机 Windows 时提示换成盘符例子）",
			field: "credential.key", raw: "opt/k", kind: PathRelative, windows: true,
			want: "credential.key 必须是绝对路径，当前值：opt/k\n" +
				"原因：配置里的路径一律写全路径；相对路径只在清单里能用\n" +
				"提示：写成带盘符的本机路径，例如 D:/Keys",
		},
		{
			name:  "Unix 上写盘符",
			field: "credential.key", raw: "D:/Keys", kind: PathDriveOnUnix,
			want: "credential.key 不是本机的绝对路径，当前值：D:/Keys\n" +
				"原因：本机是 Unix 系系统，这是 Windows 盘符写法\n" +
				"提示：写成以 / 开头的本机路径，例如 /home/ops/.keys",
		},
		{
			name:  "Unix 上写网络路径",
			field: "credential.key", raw: "//host/share", kind: PathUncOnUnix,
			want: "credential.key 不是本机的绝对路径，当前值：//host/share\n" +
				"原因：本机是 Unix 系系统，这是 Windows 网络路径（UNC）写法\n" +
				"提示：写成以 / 开头的本机路径，例如 /home/ops/.keys",
		},
	}
	for _, c := range cases {
		if got := ConfigPathProblem(c.field, c.raw, c.kind, c.windows); got != c.want {
			t.Fatalf("%s：\n got  %q\n want %q", c.name, got, c.want)
		}
	}
}
