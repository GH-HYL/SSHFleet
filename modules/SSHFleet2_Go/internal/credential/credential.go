// Package credential 是横切叶子包：凭据与主密钥那一路。
//
// 它管四件事：从环境变量读主密钥、按本工具的密文格式解一段字节与封一段字节、
// 报「这次读到哪把、本机存着哪把、齐不齐」、把明文凭据值读出来。它不造屏、不退出、
// 不判断该不该跑——那是站与塔尖的事。
//
// 三条密钥命令（生成、看状态、转换）的**流程**不在这里，住 keytool；但那三条命令要的
// 主密钥读写与加解密都在这里——主密钥是一件事，两处来源一致不一致只有住同一个包才判得出来。
//
// 判据见 ADR-0032 第四条、ADR-0036 决定三。
package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// EnvName 是主密钥的唯一定居点：运行期只从它读（老版同一处，spec D15 统一名）。
const EnvName = "SSHFLEET_KEY"

// 密文格式（0x02，老版当前唯一支持的那一版）：
// base64( 版本 1B || nonce 12B || 密文||认证标签 )，密钥由主密钥经 HKDF-SHA256 派生。
// 老版 2026-09-14 裁定不再兼容 0x01；本包照同一版，格式一字不改——同一份凭据文件
// 新老两版都要解得开。
const (
	cipherV2    = 0x02
	v2NonceSize = 12
	v2TagSize   = 16

	// v2Info 是 HKDF 的上下文串，老版写死这一条；改它等于换格式，所有密文文件立刻解不开。
	v2Info = "SSHFleet-credential-encryption-v2"
)

// KeyStatus 是「主密钥齐不齐」这个事实。
//
// 报的是状态不是句子：齐不齐怎么跟使用者说，由 message 说、由塔尖决定打不打那一屏
// （ADR-0034、ADR-0032 第四条）。
type KeyStatus string

const (
	// KeyReady 环境变量里有主密钥，加密开着的凭据解得开。
	KeyReady KeyStatus = "ready"
	// KeyMissing 环境变量里没有主密钥。加密关着时这不是问题；开着时一条密文都解不开。
	KeyMissing KeyStatus = "missing"
)

// 两种失败分开报——使用者下一步要做的完全不是一件事：
// 主密钥没读到要他去 source 一下，密文不对要他去看那个文件。
var (
	// ErrKeyMissing 环境变量里没有主密钥。
	ErrKeyMissing = errors.New("主密钥没读到")
	// ErrBadCipher 这段内容不是本工具的密文格式（长度不对、版本认不出、或校验不过）。
	ErrBadCipher = errors.New("不是本工具的密文格式")
)

// Status 报主密钥的现状。它只读环境变量，不碰磁盘、不碰终端。
func Status() KeyStatus {
	if strings.TrimSpace(os.Getenv(EnvName)) == "" {
		return KeyMissing
	}
	return KeyReady
}

// MasterKey 交回主密钥本身；没有就交 ErrKeyMissing。
//
// 首尾空白照老版去掉：主密钥是从 shell 的 rc 文件里 export 出来的，粘贴时常常带空格。
func MasterKey() (string, error) {
	key := strings.TrimSpace(os.Getenv(EnvName))
	if key == "" {
		return "", ErrKeyMissing
	}
	return key, nil
}

// Secret 把一个「凭据位置」换成明文。
//
// 加密关着：那一格本身就是密码／口令，原样交回——**一个字都不动**，包括首尾空白
// （ADR-0025 第零节：本程序不替使用者修剪凭据）。
//
// 加密开着：那一格是**装了密文的文件**的路径，读出来再解。路径由调用方补齐成完整路径
// （清单里的相对名归解析站去拼），本包不做路径推断。
func Secret(value string, encrypted bool) (string, error) {
	if !encrypted {
		return value, nil
	}
	// 先看文件在不在、读不读得到，再谈密钥：文件不在与密钥不对是两件事，
	// 说错了使用者会去修错的那一头（老版也是先 Stat 再解）。
	path := strings.TrimSpace(value)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读不到凭据文件 %s：%w", path, err)
	}
	key, err := MasterKey()
	if err != nil {
		return "", err
	}
	return Decrypt(strings.TrimSpace(string(content)), key)
}

// Decrypt 解一段 0x02 密文。解密失败一律归到 ErrBadCipher，具体哪一步不对写在话里。
func Decrypt(token, masterKey string) (string, error) {
	raw, err := decodeStrictB64(token)
	if err != nil {
		return "", fmt.Errorf("%w：%v", ErrBadCipher, err)
	}
	if len(raw) < 1+v2NonceSize+v2TagSize {
		return "", fmt.Errorf("%w：内容长度不足，文件可能被截断", ErrBadCipher)
	}
	if raw[0] != cipherV2 {
		return "", fmt.Errorf("%w：版本号 %d 认不出", ErrBadCipher, raw[0])
	}
	key, err := deriveKey(masterKey)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("%w：建不出密码器：%v", ErrBadCipher, err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("%w：建不出 GCM：%v", ErrBadCipher, err)
	}
	plain, err := gcm.Open(nil, raw[1:1+v2NonceSize], raw[1+v2NonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("%w：完整性校验不过——主密钥不匹配，或文件已损坏", ErrBadCipher)
	}
	return string(plain), nil
}

// deriveKey 由主密钥派生 32 字节 AES 密钥。派生参数与老版逐字相同，否则老文件解不开。
func deriveKey(masterKey string) ([]byte, error) {
	key, err := hkdf.Key(sha256.New, []byte(masterKey), nil, v2Info, 32)
	if err != nil {
		return nil, fmt.Errorf("%w：密钥派生失败：%v", ErrBadCipher, err)
	}
	return key, nil
}

// b64CharsetRe 是严格 base64 的字符集；用它先把明显不是密文的东西挡掉。
var b64CharsetRe = regexp.MustCompile(`^[A-Za-z0-9+/]+={0,2}$`)

// decodeStrictB64 去空白后严格解码：先看长度是不是 4 的倍数，再看字符集。
func decodeStrictB64(text string) ([]byte, error) {
	compact := compactB64(text)
	if len(compact)%4 != 0 {
		return nil, errors.New("base64 长度不合法")
	}
	if !b64CharsetRe.MatchString(compact) {
		return nil, errors.New("含 base64 字符集之外的字符")
	}
	return base64.StdEncoding.DecodeString(compact)
}

// compactB64 去掉各种空白：密文文件常被人从终端粘进去，带换行。
func compactB64(text string) string {
	var b strings.Builder
	for _, r := range text {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
