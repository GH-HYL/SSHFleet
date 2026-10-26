// Package credential 承载主干第 4 步（工具模式分流）与凭据读写：
// 凭据两态（明文 / 加密）、主密钥（SSHFLEET_KEY）、凭据转换与 0x02 AEAD 加密。
//
// 凭据的取值含义随加密开关变，与配置字段同一条规则（见 config.Account）：
// 不加密时是密码 / 口令本身，加密时是凭据文件的绝对路径。
package credential

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CredCode 凭据错误码（自定义字符串类型 + 常量：拼错编译不过）。
type CredCode string

const (
	CodeMissing      CredCode = "missing"
	CodeReadError    CredCode = "read_error"
	CodeEmpty        CredCode = "empty"
	CodeEmptyDecoded CredCode = "empty_decoded"
	CodeBadPEM       CredCode = "bad_pem"
	CodeBadCipher    CredCode = "bad_cipher"

	// 开关方向的两种错配：读到的东西与当前开关要求的形态不符
	CodeSwitchMismatch CredCode = "switch_mismatch" // 没开加密却读到密文
	CodeNotCiphertext  CredCode = "not_ciphertext"  // 开了加密却不是密文
	CodePathMissing    CredCode = "path_missing"    // 开了加密但那个文件不存在

	// 以下两个错误码属于已退役的 base64 档，只为让保留的实现仍能编译，不再有产生路径
	CodeBadBase64      CredCode = "bad_base64"
	CodeMismatchBase64 CredCode = "mismatch_base64"
)

// CredError 错误码 + 细节。
type CredError struct {
	Code   CredCode
	Detail string
}

// decodeCredential 把凭据内容还原为明文（不读盘）。
// 返回 (明文, 错误码, 错误细节, 致命错误)；明文与错误码互斥。
func decodeCredential(content string, encrypted bool) (string, CredCode, string, error) {
	if !encrypted {
		// 不加密：内容就是密码本身。读到密文说明加密开关刚被改过，这是唯一要拦的情况。
		if looksEncryptedV2(content) {
			return "", CodeSwitchMismatch, "", nil
		}
		return content, "", "", nil
	}

	// 加密：内容必须是本工具的密文，别的一概拒绝
	if !looksEncryptedV2(content) {
		return "", CodeNotCiphertext, "", nil
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
}

// ReadCredential 密码 / 口令类凭据的一条龙读取：取值 →（加密时读盘解密）→ 判空 → 出文案。
// requireNonempty 对位密码类校验（口令类不判空）。
// 返回 (明文, 问题文案列表, 致命错误)；列表空 = 通过。
// 文案每条已含来源，调用方只需拼上自己的前缀（行号 / IP / 列名）。
func ReadCredential(value string, encrypted bool, requireNonempty bool) (string, []string, error) {
	return ReadCredentialRow(value, encrypted, requireNonempty, value)
}

// ReadCredentialRow 同上，但错误文案里回显 `shown` 而不是 `value`。
//
// 清单逐行场景专用：加密刚打开、清单第 4/6 列还写着明文密码时，那个值会被当相对名
// 去拼 `secret_dir`，`value` 于是变成一条**把密码裹在路径里**的串；而这段文字要进
// 终端与工具日志，日志常常要发给别人看。调用方用 `shownCredential` 决定脱不脱敏。
func ReadCredentialRow(value string, encrypted bool, requireNonempty bool, shown string) (string, []string, error) {
	plain, credErrs, fatalErr := readCredentialCore(value, encrypted, requireNonempty)
	if fatalErr != nil {
		return "", nil, fatalErr
	}
	return plain, credProblems(credErrs, shown), nil
}

// readCredentialCore 结构化读取核心（包内 seam）：错误保持错误码形态，文案统一由 ReadCredential 组装。
func readCredentialCore(value string, encrypted bool, requireNonempty bool) (string, []CredError, error) {
	// 值原样往下走：路径交给 `os`、密码交给解码，本程序不去首尾空白（ADR-0025 第零节）。
	// 以前这里先 `TrimSpace` 一次，同一格的值在「判存在」与「读内容」两处口径不同。
	text := value

	if !encrypted {
		if text == "" {
			return "", []CredError{{CodeEmpty, ""}}, nil
		}
		plain, code, detail, fatalErr := decodeCredential(text, false)
		if fatalErr != nil {
			return "", nil, fatalErr
		}
		if code != "" {
			return "", []CredError{{code, detail}}, nil
		}
		return plain, nil, nil
	}

	if _, err := os.Stat(text); err != nil {
		// 把系统说的那一句带上（`no such file or directory` / `permission denied`）：
		// 单说「不存在」会把权限问题也说成不存在，清单行里也读不出真因（ADR-0025 决定 6）。
		return "", []CredError{{CodePathMissing, systemReasonOf(err)}}, nil
	}
	content, err := os.ReadFile(text)
	if err != nil {
		return "", []CredError{{CodeReadError, systemReasonOf(err)}}, nil
	}
	plain, code, detail, fatalErr := decodeCredential(strings.TrimSpace(string(content)), true)
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

// ReadCredentialPEMProblems 与 ReadCredentialPEM 同源，只是把**错误码**交出去。
//
// 调用方要按错因分支时（例如「文件根本不在」才补一句去路），别去比对文案——
// 文案会改，码不会。
func ReadCredentialPEMProblems(path string) (string, []CredError) {
	return readCredentialPEMCore(path)
}

// readCredentialPEMCore 私钥 PEM 结构化读取核心（包内 seam）。
func readCredentialPEMCore(path string) (string, []CredError) {
	if _, err := os.Stat(path); err != nil {
		// 系统那一句要带上：私钥位置同样可能不是「不存在」而是权限问题（ADR-0025 决定 6）
		return "", []CredError{{CodeMissing, systemReasonOf(err)}}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", []CredError{{CodeReadError, systemReasonOf(err)}}
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

// systemReasonOf 只留系统给的那一句错因，去掉前面的路径（文案里已经点过路径了）。
func systemReasonOf(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		return le.Err.Error()
	}
	return err.Error()
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
	case CodePathMissing:
		// 只出自密码 / 口令类位置（开了加密时那些位置写的应是一个文件路径），故这句解释对得上
		label = "不存在（打开加密时这里填的是凭据文件的绝对路径，不加密时才是密码本身）"
	case CodeMissing:
		// 只出自私钥位置，与加密开关无关，不能套上面那句解释
		label = "不存在"
	case CodeReadError:
		label = "无法读取"
	case CodeEmpty:
		label = "内容为空"
	case CodeEmptyDecoded:
		label = "解密后内容为空"
	case CodeBadPEM:
		label = "不是有效的PEM格式（缺少 -----BEGIN 头）"
	case CodeBadCipher:
		label = "不是有效的密文，或主密钥不匹配（先用 --convert-secret 重新转换）"
	case CodeSwitchMismatch:
		label = "读到的是密文，但当前没开加密；若刚把 encrypt 改成 false，这些位置的写法也要跟着改（改成密码/口令本身）"
	case CodeNotCiphertext:
		label = "不是本工具的密文；若刚把 encrypt 改成 true，这些位置的写法也要跟着改（改成密文文件的绝对路径，并用 --convert-secret 转换）"
	case CodeBadBase64, CodeMismatchBase64:
		label = "属于已退役的 base64 档（不应再出现）"
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

// ResolveSecretPath 「写在清单文件里」的凭据路径解析：**OS 库站在核心**（ADR-0025 决定 9）。
//
// 三层，各归各的：
//   - 简单处理（去首尾空白）程序自己做，不交给 OS——那不是路径解析；
//   - 识别（是不是绝对路径）交 `filepath.IsAbs`，拼接（目录 + 文件名）交 `filepath.Join`；
//   - **拼出来的完整路径再交回 OS 库解析一次**（`filepath.Clean`），此后只交给
//     `os.Stat` / `os.ReadFile` 消费，不再被任何字符串加工碰。
//
// **`~` 不展开**：它不属于「简单处理」，也没有 OS 库对应物，在这里就是一个普通字符
// （与清单第 5 列同口径）。以前展开过，那是本程序自己的规则，已取消。
//
// 服务对象只有内联清单与 csv 的第 4、6 列，且只在打开加密时——那时那两格写的是
// 「装了密文的文件」。清单第 5 列（私钥）、配置里的凭据字段、命令行给的路径都不经过这里。
// 存在性校验由调用方负责（清单场景经 ReadCredential 的 missing 分流）。
func ResolveSecretPath(raw, secretDir string) (string, error) {
	// 简单处理：去首尾空白
	p := strings.TrimSpace(raw)
	// 识别：交给 OS 库
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	if secretDir == "" {
		return "", ErrRelativeNoSecretDir
	}
	// 拼接交给 OS 库；拼完再交回它解析一次，得出一条完整、规范的路径
	return filepath.Clean(filepath.Join(secretDir, p)), nil
}

// ReadCredFileContent 凭据文件内容读取（--convert-secret 转换场景）：
// 读盘 + 判空 + 文本/单行防御。
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
	// 防御：明文凭据应为单行；密文（去空白后仍是合法存储形态）放行
	if strings.ContainsAny(text, "\n\r") {
		if !looksEncryptedV2(text) {
			return "", fmt.Errorf("凭据文件有多行内容，但密码应为单行，请检查是否误粘贴：%s", path)
		}
	}
	return text, nil
}
