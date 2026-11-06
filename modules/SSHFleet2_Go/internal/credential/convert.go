package credential

import (
	"os"
	"strings"

	"sshfleet2/internal/contract"
)

// 转换前读一份凭据文件：读得到、能当文本用才谈得上转。
//
// 这一步只认「这份料能不能转」，不判方向、不写盘——方向由配置里的开关定，写回由 keytool
// 做。认出的毛病用码交回，句子由 message 按码成形（文案会改，码不会）。

// ReadConvertible 读一份要转换的凭据文件。
//
// 三种毛病分开报：读不到（路径写错、没权限）、空的、含二进制、多行却不像密文。
// 它们让使用者做的事完全不是一件，混成一句「文件有问题」等于没说。
func ReadConvertible(path string) (text string, problem contract.CredFileProblem, reason string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", contract.CredFileUnreadable, SystemReason(err)
	}
	text = strings.TrimSpace(string(data))
	if text == "" {
		return "", contract.CredFileEmpty, ""
	}
	if strings.ContainsRune(text, 0) {
		return "", contract.CredFileBinary, ""
	}
	// 明文凭据应为单行；密文（去空白后仍是合法存储形态）放行。
	if strings.ContainsAny(text, "\n\r") && !IsEncrypted(text) {
		return "", contract.CredFileMultiline, ""
	}
	return text, "", ""
}
