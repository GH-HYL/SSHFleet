// Package credential 承载主干第 4 步（工具模式分流）与凭据读写（M2 落地）：
// 三等级凭据、主密钥（SSHFLEET_KEY）、--convert-password、新 0x02 AEAD 加密
// 与旧 0x01 只读解密（spec D14/D15/D30/D31）。
package credential

import "fmt"

// errNotImplemented 占位：M2 落地后删除。
var errNotImplemented = fmt.Errorf("未实现：M2 数据层落地")

// GenKey 处理 --gen-key：生成随机主密钥并持久化（zsh→~/.zshrc，bash→~/.bashrc，
// Windows 写注册表；spec D30）。disinteractive 对应旧版非交互语义。
func GenKey(disinteractive bool) error {
	_ = disinteractive
	return errNotImplemented
}

// ConvertPassword 处理 --convert-password：按当前密码安全等级把凭据文件
// 就地转换为目标格式（路径解析规则见 spec D31）。
func ConvertPassword(target, secretDir string, passwordSecurity int) error {
	_, _, _ = target, secretDir, passwordSecurity
	return errNotImplemented
}
