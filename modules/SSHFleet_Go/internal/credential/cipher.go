// 加解密引擎。
//
// 0x02（当前唯一支持格式，2026-09-14 裁定：不考虑与 4.x 旧密文的兼容，0x01 支持已移除）：
// AES-256-GCM + HKDF（stdlib crypto/hkdf），布局 base64( 版本1B + nonce12B + ct||tag )。
package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	cipherV2 = 0x02

	v2NonceSize = 12 // AES-GCM 标准 nonce 长度
	v2TagSize   = 16
)

// CipherError 加解密失败（格式非法 / 校验不通过 / 版本不支持）。
type CipherError struct{ msg string }

func (e *CipherError) Error() string { return e.msg }

func cipherErr(format string, args ...any) error {
	return &CipherError{msg: fmt.Sprintf(format, args...)}
}

// ---- 0x02：AES-256-GCM + HKDF ----

// DeriveKeyV2 由主密钥经 HKDF-SHA256 派生 32B AES 密钥。
func DeriveKeyV2(masterKey string) ([]byte, error) {
	return hkdf.Key(sha256.New, []byte(masterKey), nil, "SSHFleet-credential-encryption-v2", 32)
}

// EncryptV2 加密为 0x02 文本：base64( 0x02 || nonce12B || ct||tag )。
func EncryptV2(plaintext, masterKey string) (string, error) {
	key, err := DeriveKeyV2(masterKey)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, v2NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	out := make([]byte, 0, 1+v2NonceSize+len(sealed))
	out = append(out, cipherV2)
	out = append(out, nonce...)
	out = append(out, sealed...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// DecryptV2 解密 0x02 密文。
func DecryptV2(token, masterKey string) (string, error) {
	raw, err := decodeStrictB64(token)
	if err != nil {
		return "", cipherErr("不是有效的加密格式：%v", err)
	}
	if len(raw) < 1+v2NonceSize+v2TagSize {
		return "", cipherErr("加密内容长度不足，文件可能被截断")
	}
	if raw[0] != cipherV2 {
		return "", cipherErr("不支持的加密格式版本：%d", raw[0])
	}
	key, err := DeriveKeyV2(masterKey)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, raw[1:1+v2NonceSize], raw[1+v2NonceSize:], nil)
	if err != nil {
		return "", cipherErr("完整性校验失败：主密钥不匹配或文件已损坏")
	}
	return string(plain), nil
}

// ---- 结构识别（与旧 classify/looks_encrypted/is_probably_base64_text 对齐） ----

var b64CharsetRe = regexp.MustCompile(`^[A-Za-z0-9+/]+={0,2}$`)

// decodeStrictB64 去空白后严格 base64 解码（对位 Python validate=True）。
func decodeStrictB64(text string) ([]byte, error) {
	compact := compactB64(text)
	if len(compact)%4 != 0 {
		return nil, errors.New("base64 长度不合法")
	}
	return base64.StdEncoding.DecodeString(compact)
}

func compactB64(text string) string {
	var b strings.Builder
	for _, r := range text {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// 旧 0x01 格式的布局常量：仅剩结构识别在用（2026-09-14 裁定移除 0x01 解密支持）。
const (
	legacyV1Version = 0x01
	legacyV1Nonce   = 16
	legacyV1Mac     = 32
)

// looksEncryptedV1 结构判断：是否旧 0x01 加密格式（仅看结构，不解密）。
// 保留仅用于给旧密文一个明确的「不再支持」报错，而不是含混的「解密失败」。
func looksEncryptedV1(text string) bool {
	raw, err := decodeStrictB64(text)
	if err != nil {
		return false
	}
	return len(raw) >= 1+legacyV1Nonce+legacyV1Mac && raw[0] == legacyV1Version
}

// looksEncryptedV2 结构判断：是否新 0x02 加密格式。
func looksEncryptedV2(text string) bool {
	raw, err := decodeStrictB64(text)
	if err != nil {
		return false
	}
	return len(raw) >= 1+v2NonceSize+v2TagSize && raw[0] == cipherV2
}

// isProbablyBase64Text 规范 base64 判定：重编码一致性回验，避免恰好合法的明文误判。
func isProbablyBase64Text(text string) bool {
	compact := compactB64(text)
	if len(compact) < 4 || len(compact)%4 != 0 {
		return false
	}
	if !b64CharsetRe.MatchString(compact) {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(compact)
	if err != nil {
		return false
	}
	return base64.StdEncoding.EncodeToString(raw) == compact
}

// ContentFormat 凭据内容预分类（无需密钥）：encrypted / base64 / plain。
// 判定顺序关键：加密 token 本身是合法 base64，必须优先判 encrypted。
func ContentFormat(text string) string {
	if looksEncryptedV1(text) || looksEncryptedV2(text) {
		return "encrypted"
	}
	if isProbablyBase64Text(text) {
		return "base64"
	}
	return "plain"
}

// GenerateMasterKey 生成随机主密钥（约48字符 URL 安全文本，对位 token_urlsafe(36)）。
func GenerateMasterKey() (string, error) {
	buf := make([]byte, 36)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
