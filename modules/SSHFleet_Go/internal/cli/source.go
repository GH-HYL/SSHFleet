package cli

import (
	"fmt"
	"os"
	"strings"

	"sshfleet/internal/common"
)

// 取值来源：`-f`（节点清单）与 `-a`（代填）的值都可以是「一个 CSV 文件路径」
// 或「一条内联记录」。
//
// 判定只看原值能不能被系统找到，不认后缀名——`-f nodes.txt`、`-a answer` 一样能用。
// 命令行给的路径一律原样交给系统：本程序不换分隔符、不去空白、不拼前缀（ADR-0023）。
// 不是文件时，值必须能当一条记录读；像不像由两侧各给一条判据，判定只走 resolveValue。

// source 一个值的来源。
type source struct {
	IsFile bool   // 命中已存在的路径
	Path   string // IsFile 为真时有效，就是判定时用的那个字符串
	Raw    string // 原始值，报错时回显它
}

// resolveSource 判定取值来源：路径存在即文件，否则当内联文本。
//
// 只试原值一次：`\` 与 `/` 的互换、去空白、拼前缀这些都不做——命令行给的路径
// 由系统按当前目录解析，程序不替它加工（ADR-0023 决定 5）。
func resolveSource(raw string) source {
	if _, err := os.Stat(raw); err == nil {
		return source{IsFile: true, Path: raw, Raw: raw}
	}
	return source{Raw: raw}
}

// valueForm 一侧的取值口径：参数名、像不像一条记录的判据、两句去路。
//
// 识别只走 resolveValue 一处，两侧只在这里给各自那一条判据——`-f` 与 `-a` 不会
// 再因为各写一套判断而各自出错（ADR-0023 决定 5）。
type valueForm struct {
	flag        string
	isRecord    func(string) bool // 不是文件时，这个值像不像本侧的一条记录
	goneHint    string            // 既不是文件也不像一条记录时的去路
	newlineHint string            // 内联值里有换行时的去路
}

// 两侧的取值口径。这两份是约定不改的查找表，不是可变状态。
var (
	// 节点清单：首字段是 IPv4 就算一行节点信息。
	nodeListForm = valueForm{
		flag:        "-f",
		isRecord:    looksLikeNodeRecord,
		goneHint:    "要临时传一台机器，可以直接写一行节点信息，第一个字段是 IP，例如 192.168.1.1,22,root,密码",
		newlineHint: "把节点写进 CSV 文件，再用 -f 指向它",
	}
	// 代填：至少能给出「内容 + 触发词」两列，值里就得有列分隔符。
	answerForm = valueForm{
		flag:        "-a",
		isRecord:    looksLikeAnswerRecord,
		goneHint:    "直接写代填内容就用逗号分隔：代填内容,触发词,触发词",
		newlineHint: "把代填写进 CSV 文件，再用 -a 指向它；内容里要回车也只能写文件",
	}
)

// resolveValue 判定一处取值：先当路径交给系统，不是文件时再按本侧判据认领。
// 代填侧用它一次做完；节点清单侧的来源在 Parse 就定了（装载要 FIsInline），
// 合规检查只调 checkInlineValue 校验同一个值——规则仍只有下面那一处。
func resolveValue(raw string, form valueForm) (source, error) {
	src := resolveSource(raw)
	if src.IsFile {
		return src, nil
	}
	if err := checkInlineValue(raw, form); err != nil {
		return source{}, err
	}
	return src, nil
}

// checkInlineValue 校验一个「不是文件」的值能不能当本侧的一条记录。
//
// 内联值里出现换行直接拒绝，不静默当记录分隔、也不丢后半截——内联只写一条，
// 多台 / 多条、内容里要回车的，都写成文件（ADR-0023 决定 2）。
// 报错与提示两侧同形，只差参数名与去路。
func checkInlineValue(raw string, form valueForm) error {
	if strings.ContainsAny(raw, "\r\n") {
		return fmt.Errorf("%s 的内联值里不能有换行：内联只写一条，多台 / 多条请写成 CSV 文件\n提示：%s",
			form.flag, form.newlineHint)
	}
	if !form.isRecord(raw) {
		return fmt.Errorf("%s 参数指定的文件不存在：%s\n提示：%s", form.flag, raw, form.goneHint)
	}
	return nil
}

// looksLikeNodeRecord 一行节点信息的判据：首字段是 IPv4。
//
// CSV 允许给字段加引号（Excel 导出的单行就长成 `"10.0.0.1","22",…`），所以先剥掉
// 一对外层双引号再判。只剥引号、不预解析整行：预解析会把「引号不配对」这类真语法
// 错误吞成「文件不存在」，比现在更难查。
func looksLikeNodeRecord(raw string) bool {
	first := strings.TrimSpace(strings.SplitN(raw, ",", 2)[0])
	if len(first) >= 2 && first[0] == '"' && first[len(first)-1] == '"' {
		first = strings.TrimSpace(first[1 : len(first)-1])
	}
	return common.IsIPv4Literal(first)
}

// looksLikeAnswerRecord 一条代填的判据：值里得有列分隔符，才可能同时给出
// 「内容 + 触发词」。真正读成几列由解析器最后定（引号里的逗号不分列）。
func looksLikeAnswerRecord(raw string) bool {
	return strings.Contains(raw, ",")
}
