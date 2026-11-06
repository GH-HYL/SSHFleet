package message

import (
	"fmt"
	"strings"

	"sshfleet2/internal/contract"
)

// 三条密钥命令的每一句话都住这里：状态报告、覆盖确认、转换收尾、以及转型失败那几屏。
//
// 密钥包只管流程与事实，一个字都不自己拼（它是横切包，拼出来的字也送不到屏幕）。平台差异
// 只以事实递进来（rc 文件路径、注册表那一格、命令名），**本文件不判平台**——它只看
// `contract.KeyHost` 这个值。

// KeyModeEntered 是开工那行日志：哪条命令进来了。逐字照老版三处。
func KeyModeEntered(which contract.KeyTool) Line {
	switch which {
	case contract.KeyToolGen:
		return Line{s: "进入密钥管理模式（--gen-key）"}
	case contract.KeyToolStatus:
		return Line{s: "进入主密钥状态查看模式（--key-status）"}
	}
	return Line{s: "进入凭据转换模式（--convert-secret）"}
}

// KeyToolNeedAlone 是一次给了两条以上密钥命令那句。names 是逗号连起来的命令名。
//
// 这句走「起不来」，环节名 cli，外壳句由调用方套（选项/参数合规性检查未通过）。
func KeyToolNeedAlone(names string) string {
	return names + " 只能给一个——它们是三个独立命令，一次做一件事"
}

// KeyToolWithBatch 是密钥命令与批量执行参数一起给那句。given 是给出来的那几个选项名。
func KeyToolWithBatch(name, given string) string {
	return name + " 不能和批量执行选项一起用：同时给了 " + given + "\n" +
		"提示：分成两次执行——先跑密钥管理命令，再跑批量执行"
}

// KeyStatusReport 是 --key-status 那一整屏：现状 + 两处来源 + 下一步。
//
// 五种状态各一套说法，字逐句照老版 `KeyStatusReport`；只有标题那行带色，其余是纯文本。
func KeyStatusReport(f contract.KeyFacts) Line {
	var b strings.Builder
	switch f.State {
	case contract.KeyStateOK:
		b.WriteString("主密钥状态：正常（当前终端读到的与已保存的一致）\n")
		b.WriteString(keySourceLines(f))
		b.WriteString("  下一步：无。加密的凭据可正常读写了。\n")
	case contract.KeyStateDiverged:
		b.WriteString(ansiYellow + "主密钥状态：不一致" + ansiReset + "\n")
		b.WriteString(keySourceLines(f))
		b.WriteString("  影响：用「本次运行这把」加密出的文件，重开终端后就解不开；凭据解密也可能报「主密钥不匹配」\n")
		b.WriteString("  下一步：" + keyReloadHint(f) + "，让本次运行改用已保存的那把\n")
		b.WriteString("         若确定要用本次运行这把：" + keyPersistHint(f) + "\n")
	case contract.KeyStateStaleShell:
		b.WriteString(ansiYellow + "主密钥状态：已保存，但当前终端没读到" + ansiReset + "\n")
		b.WriteString(keySourceLines(f))
		b.WriteString("  原因：生成密钥后忘了让它生效（这就是「改了密钥反而不能用」的典型场景）\n")
		b.WriteString("  下一步：" + keyReloadHint(f) + "；本机已有密钥，不需要 --gen-key\n")
	case contract.KeyStateUnpersisted:
		b.WriteString(ansiYellow + "主密钥状态：未持久化" + ansiReset + "\n")
		b.WriteString(keySourceLines(f))
		b.WriteString("  影响：关掉终端后这把密钥就没了，用它加密的凭据文件将解不开\n")
		b.WriteString("  下一步：保持这把密钥不变并持久化——" + keyPersistHint(f) + "\n")
	case contract.KeyStateUnset:
		b.WriteString("主密钥状态：未设置\n")
		b.WriteString(keySourceLines(f))
		b.WriteString("  下一步：先生成主密钥——SSHFleet --gen-key（加密凭据需要它）\n")
	}
	return Line{s: b.String()}
}

// KeyPrecheckNote 是开工前那段提醒：走标准错误，前面空一行、带 [警告]。
//
// 只有三种状态会走到这里（不一致、已保存但没读到、未持久化）；正常与两处都空各自返回空行，
// 调用方按「有没有字」决定打不打。
func KeyPrecheckNote(f contract.KeyFacts) Line {
	body := keyPrecheckText(f)
	if body == "" {
		return Line{}
	}
	return Line{s: "\n" + ansiYellow + "[警告]" + ansiReset + " " + body, toErr: true}
}

// KeyPrecheckTrace 是预检查没通过时落到工具日志的那一行（纯文本，不上色）。
//
// 括号里是老版的状态名（diverged / stale_shell / unpersisted）：维护者靠它认是哪一档。
func KeyPrecheckTrace(state contract.KeyState) Line {
	return Line{s: "主密钥预检查未通过（" + string(state) + "）"}
}

// keyPrecheckText 是那段提醒的正文（不带 [警告] 前缀）：预检查那一屏与「缺少主密钥」那句
// 都要用它，两处说的话必须一样。
func keyPrecheckText(f contract.KeyFacts) string {
	switch f.State {
	case contract.KeyStateDiverged:
		return "主密钥两处来源不一致：\n" + keySourceLines(f) +
			"  影响：用「本次运行这把」加密的文件，重开终端后就解不开了（加密已被工具拦下）\n" +
			"  让两处一致：" + keyReloadHint(f) + "\n"
	case contract.KeyStateStaleShell:
		return "当前终端没读到已保存的主密钥（凭据解密/加密会失败）：\n" +
			fmt.Sprintf("  本机已保存：      指纹 %s（%s）\n", f.PersistPrint, f.PersistWhere) +
			"  不需要重新生成（--gen-key 会换成另一把，旧密文就解不开了），让它生效即可：" + keyReloadHint(f) + "\n"
	case contract.KeyStateUnpersisted:
		return "当前主密钥没有持久化（只活在这个终端里）：\n" +
			fmt.Sprintf("  本次运行实际使用：指纹 %s（进程环境变量 %s）\n", f.EnvPrint, f.EnvName) +
			"  影响：关掉终端后密钥就没了，用它加密的凭据文件将解不开\n" +
			"  保持这把密钥不变并持久化：" + keyPersistHint(f) + "\n"
	}
	return ""
}

// KeyNoMasterKey 是「要加密／解密，但这一趟根本没读到主密钥」那一屏。
//
// 有预检查那一段就附上（它比「去生成一把」更准：本机其实存着，只是终端没读到），
// 否则给生成指引。老版 `GetMasterKey` 就是这个次序。
func KeyNoMasterKey(f contract.KeyFacts) string {
	body := "缺少主密钥，无法解密/加密凭据文件\n"
	if note := keyPrecheckText(f); note != "" {
		return body + note
	}
	return body + "请先生成主密钥：SSHFleet --gen-key"
}

// keySourceLines 是两处来源那三行：本次运行、本机已保存、另一处持久值（有才写）。
func keySourceLines(f contract.KeyFacts) string {
	var b strings.Builder
	if f.EnvSet {
		b.WriteString(fmt.Sprintf("  本次运行实际使用：指纹 %s（进程环境变量 %s）\n", f.EnvPrint, f.EnvName))
	} else {
		b.WriteString(fmt.Sprintf("  本次运行实际使用：未读到（进程环境变量 %s 为空）\n", f.EnvName))
	}
	if f.Persisted {
		b.WriteString(fmt.Sprintf("  本机已保存：      指纹 %s（%s）\n", f.PersistPrint, f.PersistWhere))
	} else {
		b.WriteString("  本机已保存：      无\n")
	}
	if f.OtherPrint != "" {
		b.WriteString(fmt.Sprintf("  另一处持久值：    指纹 %s（%s）\n", f.OtherPrint, f.OtherWhere))
	}
	return b.String()
}

// keyReloadHint 是「怎么让本次运行读到已保存那把」的做法。
func keyReloadHint(f contract.KeyFacts) string {
	if f.Host == contract.KeyHostWindows {
		return "重新打开终端（setx 写入的值只对新开的终端生效）"
	}
	return fmt.Sprintf("执行 source %s 或重新打开终端", f.RCPath)
}

// keyPersistHint 是「怎么把当前这把写成持久值」的做法：不必把密钥打出来。
func keyPersistHint(f contract.KeyFacts) string {
	if f.Host == contract.KeyHostWindows {
		return fmt.Sprintf("在当前终端执行 setx %s \"%%%s%%\" 后重新打开终端", f.EnvName, f.EnvName)
	}
	return fmt.Sprintf(
		"把 %s 里的 export 行改成当前值（先 echo $%s 取值）：\n"+
			"       sed -i \"s|^export %s=.*|export %s='$%s'|\" %s && source %s",
		f.RCPath, f.EnvName, f.EnvName, f.EnvName, f.EnvName, f.RCPath, f.RCPath)
}

// KeyConfigUnreadable 是密钥命令自己读那一格配置读不出时那句。
//
// 这条路径实际上走不到：进密钥分支之前，解析站已经把整份配置读通一遍。留着是为了
// 「读不到就说清读不到哪一格」，而不是让它掉进一句「主密钥不匹配」。
func KeyConfigUnreadable(reason string) string {
	return ProblemReason("读不到配置文件里的加密开关（[credential] encrypt）："+ConfigShownPath, reason)
}

// KeyGenExisting 是本机已有主密钥时开场那一段：现状两处、不一致点明、覆盖的代价。
func KeyGenExisting(f contract.KeyFacts) Line {
	var b strings.Builder
	b.WriteString("检测到已存在主密钥，当前未做任何修改\n")
	if f.EnvSet {
		b.WriteString(fmt.Sprintf("  本次运行实际使用：指纹 %s（进程环境变量 %s）\n", f.EnvPrint, f.EnvName))
	} else {
		b.WriteString(fmt.Sprintf("  本次运行实际使用：未设置（进程环境变量 %s 为空）\n", f.EnvName))
	}
	if f.Persisted {
		b.WriteString(fmt.Sprintf("  本机已保存：      指纹 %s（%s）\n", f.PersistPrint, f.PersistWhere))
	}
	if f.State == contract.KeyStateDiverged {
		b.WriteString("  " + ansiYellow + "→ 两处不一致" + ansiReset + "——这正是凭据解密报「主密钥不匹配」的常见原因\n")
	}
	b.WriteString("注意：如果覆盖，用旧密钥加密的凭据文件将永久无法解密\n")
	return Line{s: b.String()}
}

// KeyGenNoOverwrite 是非交互（--yes）下拒绝自动覆盖那一屏的正文：说清代价与怎么覆盖。
func KeyGenNoOverwrite() string {
	return "检测到已存在主密钥，非交互模式不自动覆盖（覆盖后旧密钥加密的凭据文件将无法解密）。\n" +
		"如需覆盖：去掉 --yes 后重新执行 --gen-key"
}

// KeyGenConfirm 是覆盖那一问：缺省否。
func KeyGenConfirm() Line {
	return Line{s: "是否确认覆盖？"}
}

// KeyGenKept 是「答否 / 读到输入结束」时的收场句：原密钥留着，正常退出。
func KeyGenKept() Line {
	return Line{s: "已保留原密钥，未做修改\n"}
}

// KeyGenOverwrite 是答「是」之后那句：接下来才真去写。
func KeyGenOverwrite() Line {
	return Line{s: "已确认覆盖，正在重新生成主密钥...\n"}
}

// KeyGenSaved 是写成本机持久值之后的收尾：写到哪、怎么生效、生效前会被拦下、新指纹。
//
// 平台差异只体现为事实：Windows 写的是注册表那一格、要重开终端；POSIX 是一个 rc 文件路径。
func KeyGenSaved(regenerated bool, f contract.KeyFacts, res contract.PersistResult, print string) Line {
	action := "生成"
	if regenerated {
		action = "重新生成"
	}
	var b strings.Builder
	if f.Host == contract.KeyHostWindows {
		b.WriteString(fmt.Sprintf("主密钥已%s，并写入本机（HKCU\\Environment）\n", action))
		b.WriteString("请重新打开终端后再使用：当前终端的环境变量仍是旧密钥\n")
	} else {
		b.WriteString(fmt.Sprintf("主密钥已%s，并自动保存到 %s\n", action, res.Where))
		if res.OtherUnified != "" {
			b.WriteString(fmt.Sprintf("另一侧 shell 的 %s 原本存着别的主密钥，已统一为同一把\n", res.OtherUnified))
		}
		b.WriteString(fmt.Sprintf("请执行 source %s 或重新打开终端后生效\n", res.Where))
	}
	b.WriteString(ansiYellow + "注意：在此之前，本次运行读到的仍是旧密钥，执行 --convert-secret 会被拦下（避免加密出将来看不开的文件）" + ansiReset + "\n")
	b.WriteString(fmt.Sprintf("  新密钥指纹：%s\n", print))
	return Line{s: b.String()}
}

// KeyGenSaveFailed 是持久值写不进去那一屏：说清失败在哪、以及怎么手工补。
func KeyGenSaveFailed(f contract.KeyFacts, where, reason string) string {
	if f.Host == contract.KeyHostWindows {
		return "主密钥自动保存失败：" + reason + "\n可手动保存：执行 setx " + f.EnvName + " 你的随机密钥"
	}
	return "主密钥自动保存失败：" + reason + "\n可手动保存：在 " + where + " 追加 export " + f.EnvName + "='你的随机密钥'"
}

// KeyConvertAlreadyCipher 是「本来要加密，可它已经是密文」：不覆盖、正常退出。
func KeyConvertAlreadyCipher(path string) Line {
	return Line{s: "当前已是密文，无需转换：" + path + "\n"}
}

// KeyConvertAlreadyPlain 是「本来要解密，可它就是明文密码」：不覆盖、正常退出。
func KeyConvertAlreadyPlain(path string) Line {
	return Line{s: "当前已是明文（文件内容即密码本身），无需转换：" + path + "\n"}
}

// KeyConvertReadProblem 是读文件那一步的毛病：哪种毛病决定说哪句、要不要带系统原因。
func KeyConvertReadProblem(p contract.CredFileProblem, path, reason string) string {
	switch p {
	case contract.CredFileUnreadable:
		return ProblemReason("读取凭据文件失败："+path, reason)
	case contract.CredFileEmpty:
		return "凭据文件是空的，请先填入密码再转换：" + path
	case contract.CredFileBinary:
		return "凭据文件含二进制数据，不是文本格式，无法转换：" + path
	case contract.CredFileMultiline:
		return "凭据文件有多行内容，但密码应为单行，请检查是否误粘贴：" + path
	}
	return "读取凭据文件失败：" + path
}

// KeyConvertBlocked 是加密方向被硬保护拦下那一屏：为什么拦、两条出路各是什么。
func KeyConvertBlocked(f contract.KeyFacts) string {
	return "已阻止加密。\n" + divergenceNote(f) +
		"出路：\n  ① " + keyReloadHint(f) + "，然后重试\n" +
		"  ② 若确定要用本次运行这把密钥：" + keyPersistHint(f) + "，然后重试"
}

// divergenceNote 是两处来源不一致时的现状与后果（与预检查同一段事实，不提出路）。
func divergenceNote(f contract.KeyFacts) string {
	var b strings.Builder
	b.WriteString("主密钥来源不一致——这会让凭据解密报「主密钥不匹配」，且用旧的那把加密出的文件重开终端后就解不开\n")
	if f.EnvSet {
		b.WriteString(fmt.Sprintf("  本次运行实际使用：指纹 %s（进程环境变量 %s）\n", f.EnvPrint, f.EnvName))
	} else {
		b.WriteString(fmt.Sprintf("  本次运行实际使用：未设置（进程环境变量 %s 为空）\n", f.EnvName))
	}
	if f.Persisted {
		b.WriteString(fmt.Sprintf("  本机已保存：      指纹 %s（%s）\n", f.PersistPrint, f.PersistWhere))
	}
	if f.OtherPrint != "" {
		b.WriteString(fmt.Sprintf("  另一处持久值：    指纹 %s（%s）\n", f.OtherPrint, f.OtherWhere))
	}
	return b.String()
}

// KeyConvertDecryptFailed 是解不开那句：先给最可能的原因，系统原文跟在「原因：」后面。
func KeyConvertDecryptFailed(path, reason string) string {
	return ProblemReason("解密失败：主密钥与加密该文件时使用的密钥不一致，或文件已损坏/被修改："+path, reason)
}

// KeyConvertEmpty 是解出来是空串那句：拒绝写回，免得把文件写成一个空文件。
func KeyConvertEmpty(path string) string {
	return "解密结果是空的，拒绝写回：" + path
}

// KeyConvertWriteFailed 是写回失败那句。
func KeyConvertWriteFailed(path, reason string) string {
	return ProblemReason("转换内容写入失败："+path, reason)
}

// KeyConvertVerifyFailed 是写完回读不到那句。
func KeyConvertVerifyFailed(path, reason string) string {
	return ProblemReason("写入后校验失败，无法读取转换结果："+path, reason)
}

// KeyConvertDone 是转换成功那两行：方向与路径、内容长度（不回显内容本体）。
func KeyConvertDone(from, to, path string, length int) Line {
	return Line{s: fmt.Sprintf("转换成功：%s → %s：%s\n内容长度：%d 字符，请用编辑器打开该文件确认结果\n",
		from, to, path, length)}
}
