package credential

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// 主密钥那一摊里与平台无关的三件事：指纹、生成、封。
//
// 「封」只吃「明文 + 主密钥」、只交回密文串——不读盘、不写盘、不管方向、不做一致性保护。
// 那些是命令的事，住 keytool。这样切是为了让密文格式的解与封住在同一个包：格式一变不必
// 跨两个包改（ADR-0036 决定三）。

// fingerprintWidth 是印到屏幕上的字节数：4 字节 = 八位十六进制。
//
// 足够让人比对两处是不是同一把，又不至于把密钥本身漏出去。老版同值，改了会让两边的
// 指纹对不上。
const fingerprintWidth = 4

// masterKeyBytes 是生成主密钥时的随机字节数：36 字节，base64 之后约 48 字符。
// 老版 `token_urlsafe(36)` 同值。
const masterKeyBytes = 36

// Fingerprint 是密钥指纹：sha256 前 4 字节的十六进制，八位。
//
// 屏幕与日志里只许出现它，不许出现密钥本体（老版那条纪律：日志和截图都会流出去）。
func Fingerprint(key string) string {
	if strings.TrimSpace(key) == "" {
		return wordFingerprintEmpty
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:fingerprintWidth])
}

// GenerateMasterKey 生成一把随机主密钥：url 安全 base64、无填充、约 48 字符。
func GenerateMasterKey() (string, error) {
	buf := make([]byte, masterKeyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// Encrypt 把一段明文封成 0x02 密文。
//
// 格式与 `Decrypt` 同一版：base64( 版本 1B || nonce 12B || 密文||认证标签 )，密钥由主密钥
// 经 HKDF-SHA256 派生。两处派生参数必须逐字相同，否则自己封的自己解不开。
func Encrypt(plain, masterKey string) (string, error) {
	key, err := deriveKey(masterKey)
	if err != nil {
		return "", err
	}
	gcm, err := gcmOf(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, v2NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return "", cipherErr(wordNoRandom, err)
	}
	sealed := gcm.Seal(nil, nonce, []byte(plain), nil)

	out := make([]byte, 0, 1+v2NonceSize+len(sealed))
	out = append(out, cipherV2)
	out = append(out, nonce...)
	out = append(out, sealed...)
	return base64.StdEncoding.EncodeToString(out), nil
}
