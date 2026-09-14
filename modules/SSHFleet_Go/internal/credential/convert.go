// --convert-password 入口（对位旧 upgrade.py）：自适应识别文件内容格式
// （明文/base64/加密），统一还原为明文后，按目标等级重新编码写回；
// 支持升降级。目标等级 3 使用 0x02（2026-09-14 裁定：0x01 旧密文不再支持，
// 识别到时明确报错，见 D14 修订注）。
package credential

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ConvertPassword 处理 --convert-password。
//
// 路径解析为 D31 单规则（**删除**旧版"先原样找、找不到再拼 secret_dir"两段式）：
// 去空白 → ~ 展开 → 绝对原样 → 相对拼 secret_dir；secret_dir 未配置 → 明确报错。
func ConvertPassword(rawPath, secretDir string, level int) error {
	path, err := resolveCredPath(rawPath, secretDir)
	if err != nil {
		return err
	}
	content, err := ReadCredFileContent(path)
	if err != nil {
		return err
	}

	fmtStr := ContentFormat(content)

	// ① 统一还原为明文（加密格式需主密钥；解密失败=密钥不一致/文件损坏，明确提示）
	var plaintext, sourceFormat string
	switch fmtStr {
	case "encrypted":
		if looksEncryptedV1(content) {
			return fmt.Errorf(
				"凭据文件是旧版 4.x 的 0x01 加密格式，5.0.0 起不再支持读取：%s\n"+
					"请将明文密码重新写入该文件，再用 --convert-password 按当前等级转换", path)
		}
		masterKey, err := GetMasterKey()
		if err != nil {
			return err
		}
		plaintext, err = DecryptV2(content, masterKey)
		if err != nil {
			return fmt.Errorf("解密失败：主密钥与加密该文件时使用的密钥不一致，或文件已损坏/被修改：%s\n原因：%v", path, err)
		}
		sourceFormat = "加密"
	case "base64":
		raw, err := decodeStrictB64(content)
		if err != nil {
			return fmt.Errorf("base64 内容无法解码，文件可能已损坏：%s\n原因：%v", path, err)
		}
		plaintext = string(raw)
		sourceFormat = "base64"
	default:
		plaintext = content
		sourceFormat = "明文"
	}

	if plaintext == "" {
		return fmt.Errorf("内容解码后是空的，拒绝转换：%s", path)
	}

	// ② 按目标等级编码写回
	switch level {
	case 3:
		if fmtStr == "encrypted" {
			fmt.Printf("当前已是加密格式，无需转换：%s\n", path)
			return nil
		}
		masterKey, err := GetMasterKey()
		if err != nil {
			return err
		}
		token, err := EncryptV2(plaintext, masterKey)
		if err != nil {
			return err
		}
		return writeAndEcho(path, token, sourceFormat, "加密", level)
	case 2:
		if fmtStr == "base64" {
			fmt.Printf("当前已是 base64 编码，无需转换：%s\n", path)
			return nil
		}
		token := base64.StdEncoding.EncodeToString([]byte(plaintext))
		return writeAndEcho(path, token, sourceFormat, "base64", level)
	default: // level 1
		if fmtStr == "plain" {
			fmt.Printf("当前已是明文（文件内容即密码本身），无需转换：%s\n", path)
			return nil
		}
		return writeAndEcho(path, plaintext, sourceFormat, "明文", level)
	}
}

// resolveCredPath 解析 --convert-password 传入的路径（D31 单规则，走 ResolveSecretPath 单点）；
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
func writeAndEcho(path, newContent, sourceFormat, targetFormat string, level int) error {
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return fmt.Errorf("转换内容写入失败：%s\n原因：%v", path, err)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("写入后校验失败，无法读取转换结果：%s\n原因：%v", path, err)
	}
	fmt.Printf("转换成功：%s → %s（等级 %d）：%s\n", sourceFormat, targetFormat, level, path)
	fmt.Printf("内容长度：%d 字符，请用编辑器打开该文件确认结果\n", len(strings.TrimSpace(string(onDisk))))
	return nil
}
