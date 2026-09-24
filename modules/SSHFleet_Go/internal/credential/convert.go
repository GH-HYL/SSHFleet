// --convert-secret 入口：把凭据文件在「明文」与「密文」之间转换。
// 方向由配置里的加密开关决定：encrypt = true 转成密文，false 转成明文。
package credential

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ConvertSecret 处理 --convert-secret。
//
// 路径解析：去空白 → ~ 展开 → 绝对原样 → 相对拼 secret_dir；secret_dir 未配置 → 明确报错。
func ConvertSecret(rawPath, secretDir string, encrypted bool) error {
	path, err := resolveCredPath(rawPath, secretDir)
	if err != nil {
		return err
	}
	content, err := ReadCredFileContent(path)
	if err != nil {
		return err
	}
	if looksEncryptedV1(content) {
		return fmt.Errorf(
			"凭据文件是旧版加密格式，本版不再支持读取：%s\n"+
				"请把明文密码重新写入该文件，再转换一次", path)
	}

	if encrypted {
		return convertToCipher(path, content)
	}
	return convertToPlain(path, content)
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
		return fmt.Errorf("解密失败：主密钥与加密该文件时使用的密钥不一致，或文件已损坏/被修改：%s\n原因：%v", path, err)
	}
	if plaintext == "" {
		return fmt.Errorf("解密结果是空的，拒绝写回：%s", path)
	}
	return writeAndEcho(path, plaintext, "密文", "明文")
}

// resolveCredPath 解析 --convert-secret 传入的路径（走 ResolveSecretPath 单点）；
// 拼出的文件须存在。
func resolveCredPath(raw, secretDir string) (string, error) {
	p, err := ResolveSecretPath(raw, secretDir)
	if err != nil {
		if errors.Is(err, ErrRelativeNoSecretDir) {
			return "", fmt.Errorf(
				"凭据路径 '%s' 为相对路径，但 account.secret_dir 未配置，无法拼接\n出路：① 改写绝对路径 ② 在配置中设置 account.secret_dir", strings.TrimSpace(raw))
		}
		return "", err
	}
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("找不到凭据文件：%s（已按 secret_dir 拼接为 %s），请检查路径是否正确", raw, p)
	}
	return p, nil
}

// writeAndEcho 就地覆盖写入，再从磁盘回读校验；为避免凭据明文泄露到终端，
// 只输出转换摘要与内容长度，不回显文件内容本体。
func writeAndEcho(path, newContent, sourceFormat, targetFormat string) error {
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return fmt.Errorf("转换内容写入失败：%s\n原因：%v", path, err)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("写入后校验失败，无法读取转换结果：%s\n原因：%v", path, err)
	}
	fmt.Printf("转换成功：%s → %s：%s\n", sourceFormat, targetFormat, path)
	fmt.Printf("内容长度：%d 字符，请用编辑器打开该文件确认结果\n", len(strings.TrimSpace(string(onDisk))))
	return nil
}
