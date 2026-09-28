package cli

import "os"

// 取值来源：`-f`（节点清单）与 `-a`（代填）的值都可以是「一个 CSV 文件路径」
// 或「同一格式的内联文本」。
//
// 判定只看路径存不存在，不认后缀名——`-f nodes.txt`、`-a answer` 一样能用。
// 内联文本要靠各自的列语义认领（`-f` 要求首字段是 IP，`-a` 要求有逗号），
// 认不下来就报「文件不存在」，那时多半是路径打错了。

// source 一个值的来源。
type source struct {
	IsFile bool   // 命中已存在的路径
	Path   string // IsFile 为真时有效，是**探测成功**的那个形态
	Raw    string // 原始值，报错时回显它
}

// resolveSource 判定取值来源：路径存在即文件，否则当内联文本。
//
// 探测先试原值、再试规范化后的形态：`\` 与 `/` 通用是文档承诺的，Windows 风格的
// 路径在 Linux/macOS 上只有规范化之后才找得到。规范化只用在探测这一步——真当路径
// 用时才把它写回 Args，内联文本必须原样留着（`\`→`/` 会把里面的 `\n` 分行符弄坏）。
func resolveSource(raw string) source {
	if _, err := os.Stat(raw); err == nil {
		return source{IsFile: true, Path: raw, Raw: raw}
	}
	if normalized := normalizePath(raw); normalized != raw {
		if _, err := os.Stat(normalized); err == nil {
			return source{IsFile: true, Path: normalized, Raw: raw}
		}
	}
	return source{Raw: raw}
}
