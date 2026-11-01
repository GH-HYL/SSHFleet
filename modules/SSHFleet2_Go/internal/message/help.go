package message

import (
	"strings"

	"sshfleet2/internal/contract"
)

// 帮助那一屏：事实由解析站给（有哪些选项、版本是多少），排版归这里——
// 缩进、列对齐、标记列、说明列折行、示例块、结尾一句，字只在这一处成形（ADR-0032 第四条）。

// EntryKind 帮助选项表一行的种类：组标题、组间空行、选项行。
type EntryKind int

const (
	EntryGroup  EntryKind = iota // 组标题行：只装 Desc
	EntryBlank                   // 组间空行：四样全空
	EntryOption                  // 选项行：四样各占一列
)

// HelpEntry 帮助选项表的一行事实，由解析站填：短名、长名、标记、说明。
// 说明力求简洁、清晰、明确——能省的字省掉，必填、取值语义、默认值这些一个不省。
type HelpEntry struct {
	Kind  EntryKind
	Short string // 无短选项时为空
	Long  string
	Tag   string // [默认: 10] / [当前配置: 开] 这类标记，无则空
	Desc  string
}

// 帮助表的列间留白：短选项与长选项之间只留 1 个空格（贴成一个整体），
// 其余列之间留 2 个空格。测试按同一组常量核对列位置，避免两边各写一份。
const (
	helpGroupIndent = "  "   // 组标题缩进
	helpIndent      = "    " // 选项行缩进（比组标题深一级，一眼看出谁属于谁）
	helpOptGap      = " "
	helpGap         = "  "
)

// minDescWidth 说明列保底宽度：终端太窄时宁可整行超宽，也不把说明挤成一列一个字。
const minDescWidth = 24

// helpUsage 用法两行；helpExamples 示例八条。里面出现的选项名必须与选项表对得上
// （测试钉着这一条），两处各写一份迟早漂移。
var helpUsage = []string{
	"%s -c | -s | -u | -d | --change-password 之一，配合 -f 批量执行",
	"%s --gen-key | --key-status | --convert-secret 密钥管理",
}

var helpExamples = [][2]string{
	{"执行命令:", "%s -f nodes.csv -c \"ls -l\""},
	{"执行脚本:", "%s -f nodes.csv -s deploy.sh"},
	{"代填执行:", "%s -f nodes.csv -s deploy.sh -a \"y,是否继续\""},
	{"批量改密:", "%s -f nodes.csv --change-password \"新密码\""},
	{"上传文件:", "%s -f nodes.csv -u ./dist/app -p /opt/app/"},
	{"下载文件:", "%s -f nodes.csv -d /var/log/app -p ./logs/"},
	{"用密钥登录:", "%s -f nodes.csv -c \"uptime\" -k"},
	{"不确认执行:", "%s -f nodes.csv -c \"uptime\" --yes"},
}

// HelpScreen 把帮助排成一整屏。width 是终端列数，<=0 表示不折行（管道下长行更便于 grep）。
func HelpScreen(name, version string, width int, entries []HelpEntry) contract.Text {
	wShort, wLong, wTag := 0, 0, 0
	for _, e := range entries {
		if e.Kind != EntryOption {
			continue
		}
		wShort = max(wShort, displayWidth(e.Short))
		wLong = max(wLong, displayWidth(e.Long))
		wTag = max(wTag, displayWidth(e.Tag))
	}
	// 说明列的起始列与可用宽度：折行必须落在说明列内，不串到其它列
	descStart := displayWidth(helpIndent) + wShort + displayWidth(helpOptGap) +
		wLong + displayWidth(helpGap) + wTag + displayWidth(helpGap)
	descWidth := 0
	if width > 0 {
		descWidth = max(width-descStart-1, minDescWidth)
	}

	var b strings.Builder
	b.WriteString("SSHFleet - 批量 SSH 运维工具（命令/脚本执行、文件上传下载）\n")
	b.WriteString("版本: " + version + "\n\n")
	b.WriteString("用法:\n")
	for _, line := range helpUsage {
		b.WriteString(helpGroupIndent + strings.ReplaceAll(line, "%s", name) + "\n")
	}
	b.WriteString("\n")
	b.WriteString("选项:\n\n")
	for _, e := range entries {
		switch e.Kind {
		case EntryGroup: // 组标题
			b.WriteString(helpGroupIndent + e.Desc + "\n")
			continue
		case EntryBlank: // 分组空行
			b.WriteString("\n")
			continue
		}
		head := helpIndent + padTo(e.Short, wShort) + helpOptGap + padTo(e.Long, wLong) +
			helpGap + padTo(e.Tag, wTag) + helpGap
		for i, line := range wrapByWidth(e.Desc, descWidth) {
			if i == 0 {
				b.WriteString(head + line + "\n")
				continue
			}
			b.WriteString(strings.Repeat(" ", descStart) + line + "\n")
		}
	}
	b.WriteString("\n示例:\n")
	labelWidth := 0
	for _, e := range helpExamples {
		labelWidth = max(labelWidth, displayWidth(e[0]))
	}
	for _, e := range helpExamples {
		b.WriteString("  " + padTo(e[0], labelWidth) + " " + strings.ReplaceAll(e[1], "%s", name) + "\n")
	}
	b.WriteString("\n更多用法、配置说明、示例：见 docs/\n")
	return contract.Text{S: b.String()}
}

// CannotStartLine 起不来那一句话。本阶段是过渡形态：只有一句加一个换行，
// 话术与退出码表随阶段 03 定稿，别当口径抄。
func CannotStartLine(head, detail string) contract.Text {
	return contract.Text{S: Shell(head, detail) + "\n"}
}
