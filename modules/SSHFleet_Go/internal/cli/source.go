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
//
// 第二个返回值是**判成「不是文件」时系统给的那条原因**，报错要把它原样带出来
// （ADR-0025 决定 6）：只说「文件不存在」会把「权限不够」这种真原因吞掉。
func resolveSource(raw string) (source, error) {
	_, statErr := os.Stat(raw)
	if statErr == nil {
		return source{IsFile: true, Path: raw, Raw: raw}, nil
	}
	return source{Raw: raw}, statErr
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
	// 节点清单：CSV 读出的首字段是 IPv4 就算一行节点信息。判据本身在 common 一处，
	// 显示时的脱敏也调它——这里只是把它接进本侧的取值口径。
	nodeListForm = valueForm{
		flag:        "-f",
		isRecord:    common.LooksLikeInlineNodeRecord,
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
// `-f` 与 `-a` 都走这一处（各自只提供一条判据）：合规检查是唯一的调用点，
// `-f` 的来源也在那里落回 FIsInline（装载与归档要用）。
func resolveValue(raw string, form valueForm) (source, error) {
	src, statErr := resolveSource(raw)
	if src.IsFile {
		return src, nil
	}
	if err := checkInlineValue(raw, form, statErr); err != nil {
		return source{}, err
	}
	return src, nil
}

// checkInlineValue 校验一个「不是文件」的值能不能当本侧的一条记录。
//
// 内联值里出现换行直接拒绝，不静默当记录分隔、也不丢后半截——内联只写一条，
// 多台 / 多条、内容里要回车的，都写成文件（ADR-0023 决定 2）。
// 报错与提示两侧同形，只差参数名与去路：原因只说一句「不能有换行」，
// 去路留给下一行，两处不重复同一件事。
//
// 「既不是文件也不像一条记录」按三行写（问题 / 原因 / 提示），原因取系统原文：
// 文件找不到与权限不够，系统分得比我们清楚（ADR-0025 决定 6）。
//
// statErr 此处必非 nil：走到这里就说明 resolveSource 没认定成文件，而它只在 Stat 出错时
// 才这么返回。不为「可能是 nil」再补一个兜底分支——那种分支永远走不到，只会让人以为
// 另有一条路径。
func checkInlineValue(raw string, form valueForm, statErr error) error {
	if strings.ContainsAny(raw, "\r\n") {
		return fmt.Errorf("%s 的内联值里不能有换行：内联只写一条\n提示：%s", form.flag, form.newlineHint)
	}
	if !form.isRecord(raw) {
		return fmt.Errorf("%s 参数指定的文件读不到：%s\n原因：%s\n提示：%s", form.flag, raw, systemReason(statErr), form.goneHint)
	}
	return nil
}

// looksLikeAnswerRecord 一条代填的判据：值里得有列分隔符，才可能同时给出
// 「内容 + 触发词」。真正读成几列由解析器最后定（引号里的逗号不分列）。
func looksLikeAnswerRecord(raw string) bool {
	return strings.Contains(raw, ",")
}
