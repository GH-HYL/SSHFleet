// Package credential 承载主干第 4 步（工具模式分流）与凭据读写：
// 三等级凭据、主密钥（SSHFLEET_KEY）、--convert-password 与 0x02 AEAD 加密
// （spec D15/D30/D31；2026-09-14 裁定移除 0x01 旧密文兼容，见 D14 修订注）。
package credential

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// CredCode 凭据错误码（自定义字符串类型 + 常量，spec 实现途径 8：拼错编译不过）。
type CredCode string

const (
	CodeMissing           CredCode = "missing"
	CodeReadError         CredCode = "read_error"
	CodeEmpty             CredCode = "empty"
	CodeBadBase64         CredCode = "bad_base64"
	CodeEmptyDecoded      CredCode = "empty_decoded"
	CodeBadPEM            CredCode = "bad_pem"
	CodeBadCipher         CredCode = "bad_cipher"
	CodeLegacyCipher      CredCode = "legacy_cipher"
	CodeMismatchEncrypted CredCode = "mismatch_encrypted"
	CodeMismatchBase64    CredCode = "mismatch_base64"
)

// CredError 错误码 + 细节。
type CredError struct {
	Code   CredCode
	Detail string
}

// checkFmtLevel 凭据内容格式 × 密码安全等级的错配诊断（全工具单一事实来源）。
// encrypted 仅等级3匹配；base64 仅等级2匹配（等级3要求加密格式，报 bad_cipher）；plain 仅等级1匹配。
func checkFmtLevel(fmtStr string, level int) (CredCode, bool) {
	switch {
	case fmtStr == "encrypted" && level != 3:
		return CodeMismatchEncrypted, true
	case fmtStr == "base64" && level == 1:
		return CodeMismatchBase64, true
	case fmtStr == "base64" && level == 3:
		return CodeBadCipher, true
	case fmtStr == "plain" && level == 2:
		return CodeBadBase64, true
	case fmtStr == "plain" && level == 3:
		return CodeBadCipher, true
	}
	return "", false
}

// decodeCredential 按密码安全等级把凭据内容还原为明文（不读盘）。
// 返回 (明文, 错误码, 错误细节, 致命错误)。明文与错误互斥；
// 等级 3 且内容确为加密格式时才拉取主密钥（懒获取，对位旧 get_master_key_or_exit 时机）。
// 旧 0x01 密文不再解密：识别为 legacy_cipher，给「不再支持」的明确报错（2026-09-14 裁定）。
func decodeCredential(content string, level int) (string, CredCode, string, error) {
	fmtStr := ContentFormat(content)
	if code, bad := checkFmtLevel(fmtStr, level); bad {
		return "", code, "", nil
	}
	switch level {
	case 1:
		return content, "", "", nil
	case 3:
		if looksEncryptedV1(content) {
			return "", CodeLegacyCipher, "", nil
		}
		masterKey, fatalErr := GetMasterKey()
		if fatalErr != nil {
			return "", "", "", fatalErr
		}
		plain, err := DecryptV2(content, masterKey)
		if err != nil {
			return "", CodeBadCipher, err.Error(), nil
		}
		return plain, "", "", nil
	default: // level 2
		raw, err := decodeStrictB64(content)
		if err != nil {
			return "", CodeBadBase64, "", nil
		}
		if !utf8.Valid(raw) {
			return "", CodeBadBase64, "", nil
		}
		return string(raw), "", "", nil
	}
}

// ReadCredential 密码/口令类凭据的一条龙读取：读盘 → 判空 → 格式分类 → 等级匹配 → 解码/解密 → 出文案。
// requireNonempty 对位密码类校验（口令类不判空）。旧版 (path, level) 解码缓存不移植：
// 解码发生在预检、结果直接进节点数据（spec 实现层差异）。
// 返回 (明文, 问题文案列表, 致命错误)；列表空 = 通过。
// 文案每条已含路径，调用方只需拼上自己的前缀（行号 / IP / 列名），不必了解错误码。
func ReadCredential(path string, level int, requireNonempty bool) (string, []string, error) {
	plain, credErrs, fatalErr := readCredentialCore(path, level, requireNonempty)
	if fatalErr != nil {
		return "", nil, fatalErr
	}
	return plain, credProblems(credErrs, path), nil
}

// readCredentialCore 结构化读取核心（包内 seam）：错误保持错误码形态，文案统一由 ReadCredential 组装。
func readCredentialCore(path string, level int, requireNonempty bool) (string, []CredError, error) {
	if _, err := os.Stat(path); err != nil {
		return "", []CredError{{CodeMissing, ""}}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", []CredError{{CodeReadError, err.Error()}}, nil
	}
	text := strings.TrimSpace(string(content))
	if text == "" {
		return "", []CredError{{CodeEmpty, ""}}, nil
	}
	plain, code, detail, fatalErr := decodeCredential(text, level)
	if fatalErr != nil {
		return "", nil, fatalErr
	}
	if code != "" {
		return "", []CredError{{code, detail}}, nil
	}
	if requireNonempty && plain == "" {
		return "", []CredError{{CodeEmptyDecoded, ""}}, nil
	}
	return plain, nil, nil
}

// ReadCredentialPEM 私钥 PEM 读取校验：读盘 → 判空 → -----BEGIN 前缀检查（不参与 fmt×level 矩阵）。
// 返回 (内容, 问题文案列表)，语义同 ReadCredential。
func ReadCredentialPEM(path string) (string, []string) {
	content, credErrs := readCredentialPEMCore(path)
	return content, credProblems(credErrs, path)
}

// readCredentialPEMCore 私钥 PEM 结构化读取核心（包内 seam）。
func readCredentialPEMCore(path string) (string, []CredError) {
	if _, err := os.Stat(path); err != nil {
		return "", []CredError{{CodeMissing, ""}}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", []CredError{{CodeReadError, err.Error()}}
	}
	text := strings.TrimSpace(string(content))
	if text == "" {
		return "", []CredError{{CodeEmpty, ""}}
	}
	if !strings.HasPrefix(text, "-----BEGIN") {
		return "", []CredError{{CodeBadPEM, ""}}
	}
	return text, nil
}

// ReadPEMRaw 私钥 PEM 原样读取（对位 _read_credential(decode_base64=False)）。
// 调用方需保证文件已通过 ReadCredentialPEM /  -k 存在性校验。
func ReadPEMRaw(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

// ---- 错误码 → 用户文案（单一事实来源，校验汇总 / 退出指引两条路径共用） ----

// credProblems 错误码列表 → 调用方可直接拼接的短文案列表（每条已含路径），空 = 通过。
func credProblems(credErrs []CredError, path string) []string {
	if len(credErrs) == 0 {
		return nil
	}
	problems := make([]string, 0, len(credErrs))
	for _, ce := range credErrs {
		problems = append(problems, CredErrorLabel(ce.Code, path, ce.Detail))
	}
	return problems
}

// CredErrorLabel 凭据错误码 → 汇总短文案（CSV 校验汇总路径用）。
func CredErrorLabel(code CredCode, path string, detail string) string {
	var label string
	switch code {
	case CodeMissing:
		label = "不存在"
	case CodeReadError:
		label = "无法读取"
	case CodeEmpty:
		label = "内容为空"
	case CodeBadBase64:
		label = "不是有效的Base64编码"
	case CodeEmptyDecoded:
		label = "解码后内容为空"
	case CodeBadPEM:
		label = "不是有效的PEM格式（缺少 -----BEGIN 头）"
	case CodeBadCipher:
		label = "不是有效的加密格式或主密钥不匹配（请先用 --convert-password 转换该文件）"
	case CodeLegacyCipher:
		label = "是旧版 0x01 加密格式（5.0.0 起不再支持，请重新录入密码后按当前等级重新转换）"
	case CodeMismatchEncrypted:
		label = "文件是本工具等级3（加密）格式，与当前密码安全等级不匹配"
	case CodeMismatchBase64:
		label = "文件是等级2（base64）格式，与当前密码安全等级不匹配"
	default:
		label = fmt.Sprintf("未知凭据错误(%s)", code)
	}
	if detail != "" {
		return fmt.Sprintf("%s → %s (%s)", label, path, detail)
	}
	return fmt.Sprintf("%s → %s", label, path)
}

// ---- D31 路径解析单点：写在文件里的凭据相对路径拼 secret_dir ----

// ErrRelativeNoSecretDir 相对路径但 secret_dir 未配置（调用方按各自场景组织文案）。
var ErrRelativeNoSecretDir = errors.New("相对路径但 secret_dir 未配置")

// ResolveSecretPath 凭据路径梯子（全工具单一事实来源，spec D31 统一解析顺序）：
// 去首尾空白 → ~ 展开 → 绝对则原样 → 相对则拼 secret_dir。
// 存在性校验由调用方负责（CSV 场景经 ReadCredential 的 missing，转换场景显式 Stat）。
func ResolveSecretPath(raw, secretDir string) (string, error) {
	p := strings.TrimSpace(raw)
	p = expandHomeTilde(p)
	if filepath.IsAbs(p) {
		return p, nil
	}
	if secretDir == "" {
		return "", ErrRelativeNoSecretDir
	}
	return filepath.Join(secretDir, p), nil
}

// expandHomeTilde 把前导 ~ 展开为用户主目录。
func expandHomeTilde(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
}

// ReadCredFileContent 凭据文件内容读取（--convert-password 转换场景）：
// 读盘 + 判空 + 文本/单行防御（对位旧 read_cred_file_content）。
func ReadCredFileContent(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取凭据文件失败：%s\n原因：%v", path, err)
	}
	text := strings.TrimSpace(string(content))
	if text == "" {
		return "", fmt.Errorf("凭据文件是空的，请先填入密码再转换：%s", path)
	}
	if strings.ContainsRune(text, 0) {
		return "", fmt.Errorf("凭据文件含二进制数据，不是文本格式，无法转换：%s", path)
	}
	// 防御：明文凭据应为单行；多行 base64 / 加密格式（去空白后合法存储形态）放行
	if strings.ContainsAny(text, "\n\r") {
		if !(isProbablyBase64Text(text) || looksEncryptedV1(text) || looksEncryptedV2(text)) {
			return "", fmt.Errorf("凭据文件有多行内容，但密码应为单行，请检查是否误粘贴：%s", path)
		}
	}
	return text, nil
}
