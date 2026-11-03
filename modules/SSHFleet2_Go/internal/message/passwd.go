package message

// 改密值那一路的话（建站 05 的 07 号单）：密文贴错地方、文件读不到、解不出来、形态不对。
//
// 逐字照老版 `cli/passwd.go`：哪一句是「问题／原因／提示」三段、哪一句只有两段，都照原样。
// 使用者照着做的那一步不一样——有的去看文件，有的去改配置，有的换一个密码。

// PasswdValueIsCipherEncrypted 开了加密，但这一格贴的是密文内容本身。
func PasswdValueIsCipherEncrypted() string {
	return Problem(
		"--change-password 写的是密文内容，不是文件路径",
		"开了加密时，这一格要写密文文件的本机绝对路径",
		"把这段密文存成文件，这里改写成该文件的绝对路径（用 --convert-secret 转出来的那个文件）")
}

// PasswdValueIsCipherPlain 没开加密，但这一格贴的是密文内容。
func PasswdValueIsCipherPlain() string {
	return Problem(
		"--change-password 写的是密文内容，不是新密码",
		"当前没开加密，这一格要写新密码本身；密文只能装进文件、由路径指过来",
		"要用密文，先把配置的 encrypt 改成 true，再把密文存成文件、这里写该文件的绝对路径")
}

// PasswdValueUnreadable 开了加密，但那个密文文件读不到。原因取系统原文。
func PasswdValueUnreadable(path, reason string) string {
	return Problem(
		"--change-password 的值读不到："+path,
		reason,
		"打开加密时这里写密文文件的本机绝对路径（用 --convert-secret 转出来的那个文件）")
}

// PasswdValueUndecodable 读到了，但解不出新密码（主密钥没读到、密文坏了）。
//
// 与「值有问题」分开说：这一条要人去看主密钥或那个文件，不是去改命令行。
func PasswdValueUndecodable(reason string) string {
	return ProblemReason("--change-password 的值读不出新密码", reason)
}

// PasswdValueProblems 值读到了，但内容不对：problems 是逐个毛病拼起来的一句。
func PasswdValueProblems(problems string) string {
	return ProblemHint(
		"--change-password 的值有问题："+problems,
		"也可以把配置的 encrypt 改成 false，直接在这一格写新密码")
}

// PasswdHasNewline 新密码里有换行或回车。
//
// 改密是逐行喂进远端终端的，换行等于回车：提交上去是半截密码，而且看不出来。
func PasswdHasNewline() string {
	return Problem(
		"--change-password 的新密码里有换行符或回车",
		"参数被换行截断，或复制粘贴时带进了换行",
		"把整串写成一行（或换一个密码）后重跑")
}

// PasswdHasOuterSpace 新密码首尾有空白（全空白的值也落在这里）。
//
// 远端看不见这些字符：设上去以后自己也敲不出来。
func PasswdHasOuterSpace() string {
	return Problem(
		"--change-password 的新密码开头或结尾有空白",
		"多半是多打或复制粘贴时带进来的空格、制表符",
		"换一个不带首尾空白的密码后重跑；远端看不见这些字符，设上去以后你自己也敲不出来")
}

// PasswdProblemLabel 凭据包报的一条问题 → 一句能照做的话。
//
// 码是凭据包的稳定取值，话只在这里成形（比文案可靠：凭据包只交码与事实）。
func PasswdProblemLabel(code, shown, detail string) string {
	label := ""
	switch code {
	case "path_missing":
		label = "不存在（打开加密时这里填的是凭据文件的绝对路径，不加密时才是密码本身）"
	case "read_error":
		label = "无法读取"
	case "empty":
		label = "内容为空"
	case "empty_decoded":
		label = "解密后内容为空"
	case "not_ciphertext":
		label = "不是本工具的密文；若刚把 encrypt 改成 true，这一格要改写密文文件的绝对路径（用 --convert-secret 转换）"
	case "switch_mismatch":
		label = "读到的是密文，但当前没开加密；若刚把 encrypt 改成 false，这一格要写成密码本身"
	default:
		label = "值不对"
	}
	if detail != "" {
		return label + " → " + shown + "（" + detail + "）"
	}
	if shown != "" {
		return label + " → " + shown
	}
	return label
}
