package credential

// 凭据值的读取：给「值 + 加密开关」，交回明文，或者一份问题清单。
//
// 与 `Secret` 的分工：`Secret` 只回答「解不解得开」，报错是一句话（清单逐行预检就用它，
// 那里只要「哪一处不对」这一层）。这里多一层：把「值这一侧的毛病」拆成一条条问题，
// 让调用方按毛病各说各的下一步——发布版那一支给的是能照做的三句话，不是一句「值有问题」。
//
// 三种结局分开：
//   - 明文：过了；
//   - 问题清单：值是读到了／没读到，但内容不对（空、不是密文、开关不匹配）；
//   - 错误：这一层给不了解释——主密钥没读到、密文校验不过。那是另一件事，
//     使用者的下一步（去 source 主密钥／去看那个文件）与上面几条完全不同，不许混成一句。

import (
	"os"
	"strings"

	"sshfleet2/internal/criterion"
)

// ValueCode 一条问题是什么毛病。码是稳定的，文案由 message 按码成形——比文案可靠。
type ValueCode string

const (
	// ValueEmpty 这一格是空的。
	ValueEmpty ValueCode = "empty"
	// ValueEmptyDecoded 密文解出来是空的。
	ValueEmptyDecoded ValueCode = "empty_decoded"
	// ValuePathMissing 开了加密，这一格该是密文文件的路径，但那个文件不在。
	ValuePathMissing ValueCode = "path_missing"
	// ValueReadError 文件在，读不出来（权限之类）。
	ValueReadError ValueCode = "read_error"
	// ValueNotCiphertext 开了加密，但内容不是本工具的密文（多半是把明文密码写进来了）。
	ValueNotCiphertext ValueCode = "not_ciphertext"
	// ValueSwitchMismatch 没开加密，但内容是密文（多半是刚把 encrypt 改成了 false）。
	ValueSwitchMismatch ValueCode = "switch_mismatch"
)

// ValueProblem 值这一侧的一条问题。
//
// Shown 是要回显的那个值（路径或内容，调用方决定遮不遮）；Detail 是系统给的那句原文，
// 没有就空着。判据在码上，不在文案上：文案会改，码不会。
type ValueProblem struct {
	Code   ValueCode
	Shown  string
	Detail string
}

// PlainValue 把一个凭据位置解释成明文。
//
// 加密关着：那一格本身就是密码，原样交回——**一个字符都不动**，包括首尾空白
// （本程序不替使用者修剪凭据）。
//
// 加密开着：那一格是**装了密文的文件**的路径。相对名先拼 `secret_dir` 是调用方的事，
// 本包不做路径推断。
//
// requireNonempty 判「解出来的明文空不空」：密码类要判，口令类可以不判。
func PlainValue(value string, encrypted bool, requireNonempty bool) (string, []ValueProblem, error) {
	if !encrypted {
		if value == "" {
			return "", []ValueProblem{{Code: ValueEmpty, Shown: value}}, nil
		}
		// 没开加密却读到密文：这是开关刚被改过，照原样当密码用会登不上，
		// 而报出来的是「认证失败」——指不到真原因。
		if IsEncrypted(value) {
			return "", []ValueProblem{{Code: ValueSwitchMismatch, Shown: value}}, nil
		}
		return value, nil, nil
	}

	path := value
	if _, err := os.Stat(path); err != nil {
		// 系统那句原因要带上：只说「不存在」会把「没权限」也说成不存在。
		return "", []ValueProblem{{Code: ValuePathMissing, Shown: path, Detail: criterion.SystemReason(err)}}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", []ValueProblem{{Code: ValueReadError, Shown: path, Detail: criterion.SystemReason(err)}}, nil
	}
	text := strings.TrimSpace(string(content))
	if !IsEncrypted(text) {
		return "", []ValueProblem{{Code: ValueNotCiphertext, Shown: path}}, nil
	}
	key, err := MasterKey()
	if err != nil {
		return "", nil, err
	}
	plain, err := Decrypt(text, key)
	if err != nil {
		return "", nil, err
	}
	if requireNonempty && plain == "" {
		return "", []ValueProblem{{Code: ValueEmptyDecoded, Shown: path}}, nil
	}
	return plain, nil, nil
}

// IsEncrypted 判一段文本是不是本工具的密文形态：严格 base64、长度够、版本号认得。
//
// 只看形状，不试着解——解不解得开要主密钥，而「这一格写的是密文还是密码」是形状问题。
func IsEncrypted(text string) bool {
	raw, err := decodeStrictB64(text)
	if err != nil {
		return false
	}
	return len(raw) >= 1+v2NonceSize+v2TagSize && raw[0] == cipherV2
}
