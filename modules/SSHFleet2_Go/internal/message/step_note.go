package message

// 执行期日志里那条「中间步骤」的事实行。
//
// **它只给日志**：中间步骤是工具在远端额外跑的伴随命令（预检 `test -e`、枚举 `find`、
// 改密收尾 `:`），它们的退出码是排查现场的事实，不是给使用者看的结论——终端、`report.txt`
// 与两张表都不摊这一行（失败原因由报文那一路说人话）。

import (
	"strconv"
	"strings"

	"sshfleet2/internal/contract"
)

// stepLabels 是中间步骤在日志里的中文名与那条命令的原文。
//
// 名字照老版：那三条各有模式限定（预检、枚举只在下载里、改密收尾只在改密里），日志里换一段
// 就没别的行说明它是哪个模式的步骤——所以「下载」「远程文件」不省。
var stepLabels = map[string]struct {
	label string
	cmd   string
}{
	contract.StepPrecheck:  {"下载预检", "test -e"},
	contract.StepEnumerate: {"远程文件枚举", "find"},
	contract.StepPasswd:    {"改密收尾", ":"},
}

// StepNote 是「中间步骤：预检（test -e） 退出码 1、…」那一行；没有一步失败时给空串。
//
// 判据照老版：这一段里**但凡有一个非 0 的退出码**，整段照常列出（同一段里既有 0 又有非 0
// 时不能只挑非 0 的——「收尾 0、预检 1」拼起来才看得出先后）。全 0 或压根没有中间步骤时
// 一个字都不写：日志已经够长，干净跑完的那台不需要这一行。
func StepNote(steps []contract.Step) string {
	if !anyStepFailed(steps) {
		return ""
	}
	items := make([]string, 0, len(steps))
	for _, st := range steps {
		meta, ok := stepLabels[st.Name]
		if !ok {
			meta.label, meta.cmd = st.Name, st.Name
		}
		if st.ExitCode == nil {
			items = append(items, meta.label+"（"+meta.cmd+"） 未拿到退出码")
			continue
		}
		items = append(items, meta.label+"（"+meta.cmd+"） 退出码 "+strconv.Itoa(*st.ExitCode))
	}
	return "中间步骤：" + strings.Join(items, "、")
}

// anyStepFailed 这一段里有没有一步真的没跑成。
func anyStepFailed(steps []contract.Step) bool {
	for _, st := range steps {
		if st.ExitCode != nil && *st.ExitCode != 0 {
			return true
		}
	}
	return false
}
