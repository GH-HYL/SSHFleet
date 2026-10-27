// --convert-secret 入口：把凭据文件在「明文」与「密文」之间转换。
// 方向由配置里的加密开关决定：encrypt = true 转成密文，false 转成明文。
package credential

import (
	"fmt"
	"os"
	"strings"

	"sshfleet/internal/common"
)

// ConvertSecret 处理 --convert-secret。
//
// 目标路径原样交给系统：相对、绝对、../ 都由 os 按当前工作目录解析，本程序不判断
// 形态、不展开 ~、不去空白。secret_dir 只服务写在文件里的凭据（见 ResolveSecretPath）。
func ConvertSecret(rawPath string, encrypted bool) error {
	content, err := ReadCredFileContent(rawPath)
	if err != nil {
		return err
	}
	if encrypted {
		return convertToCipher(rawPath, content)
	}
	return convertToPlain(rawPath, content)
}

// convertToCipher 明文 → 密文。
func convertToCipher(path, content string) error {
	if looksEncryptedV2(content) {
		fmt.Printf("当前已是密文，无需转换：%s\n", path)
		return nil
	}
	// 硬保护：两处主密钥不一致时拒绝加密（用旧密钥加的密，重开终端后就解不开了）
	if err := GuardEncrypt(); err != nil {
		return err
	}
	masterKey, err := GetMasterKey()
	if err != nil {
		return err
	}
	token, err := EncryptV2(content, masterKey)
	if err != nil {
		return err
	}
	return writeAndEcho(path, token, "明文", "密文")
}

// convertToPlain 密文 → 明文。
func convertToPlain(path, content string) error {
	if !looksEncryptedV2(content) {
		fmt.Printf("当前已是明文（文件内容即密码本身），无需转换：%s\n", path)
		return nil
	}
	masterKey, err := GetMasterKey()
	if err != nil {
		return err
	}
	plaintext, err := DecryptV2(content, masterKey)
	if err != nil {
		return common.ErrUserReason(fmt.Sprintf("解密失败：主密钥与加密该文件时使用的密钥不一致，或文件已损坏/被修改：%s", path), err.Error())
	}
	if plaintext == "" {
		return fmt.Errorf("解密结果是空的，拒绝写回：%s", path)
	}
	return writeAndEcho(path, plaintext, "密文", "明文")
}

// writeAndEcho 就地覆盖写入，再从磁盘回读校验；为避免凭据明文泄露到终端，
// 只输出转换摘要与内容长度，不回显文件内容本体。
func writeAndEcho(path, newContent, sourceFormat, targetFormat string) error {
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return common.ErrUserReason(fmt.Sprintf("转换内容写入失败：%s", path), err.Error())
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		return common.ErrUserReason(fmt.Sprintf("写入后校验失败，无法读取转换结果：%s", path), err.Error())
	}
	fmt.Printf("转换成功：%s → %s：%s\n", sourceFormat, targetFormat, path)
	fmt.Printf("内容长度：%d 字符，请用编辑器打开该文件确认结果\n", len(strings.TrimSpace(string(onDisk))))
	return nil
}
