package report

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"

	"sshfleet2/internal/contract"
)

// 运行上下文的解。封在 internal/parse/runmeta.go —— **两段代码各写一遍**，见那边的注释。
//
// 报告站是全工程唯一读得到这一段的地方（ADR-0035 决定九）：`report.txt` 第一段的
// 【执行命令】【执行参数】两段吃它。别的站收下原样传，解不开也不该解。

// runMetaKey 与解析站那一份**逐字节相同**，两处各写一次、不留变量、不往下传。
var runMetaKey = [32]byte{
	0x63, 0xf6, 0xfc, 0x2c, 0x9c, 0xdc, 0x96, 0x26,
	0x89, 0x0b, 0xbe, 0x1c, 0xb1, 0x5d, 0x76, 0xe5,
	0x73, 0x3e, 0x35, 0x40, 0x44, 0x71, 0x21, 0xc6,
	0x02, 0x47, 0xc3, 0x50, 0xf3, 0xb0, 0xe2, 0x7f,
}

// openRunMeta 把那一段字节开回原值。
//
// 解不开只有两种原因：版本不对（封法改过），或两处密钥／格式漂了。**直接报错，不猜、
// 不拿别处的值顶**——拿别处的值顶，报告就写出一份看着对、其实不是使用者敲的东西。
//
// 交回的是「哪一类坏」而不是一句话：与解析站封侧同形（ADR-0057），句子由 `message` 按
// 类别成形。第三个返回值是「坏了」这一事实本身。
func openRunMeta(sealed contract.RunMeta) (contract.RunMetaBody, contract.CannotStartReason, bool) {
	var body contract.RunMetaBody
	if len(sealed) < 1+contract.RunMetaNonceSize+contract.RunMetaTagSize {
		return body, contract.CannotStartReason{
			Kind:  contract.CannotStartRunMetaLength,
			Index: len(sealed),
		}, true
	}
	if sealed[0] != contract.RunMetaVersion {
		return body, contract.CannotStartReason{
			Kind:  contract.CannotStartRunMetaVersion,
			Index: int(sealed[0]),
		}, true
	}
	gcm, reason, bad := runMetaGCM()
	if bad {
		return body, reason, true
	}
	nonce := sealed[1 : 1+contract.RunMetaNonceSize]
	plain, err := gcm.Open(nil, nonce, sealed[1+contract.RunMetaNonceSize:], nil)
	if err != nil {
		return body, contract.CannotStartReason{
			Kind:   contract.CannotStartRunMetaOpen,
			Origin: err.Error(),
		}, true
	}
	if err := json.Unmarshal(plain, &body); err != nil {
		return body, contract.CannotStartReason{
			Kind:   contract.CannotStartRunMetaBody,
			Origin: err.Error(),
		}, true
	}
	return body, contract.CannotStartReason{}, false
}

// runMetaGCM 与解析站那一段同形：密钥长度写死，建不起来说明写死的值错了。
func runMetaGCM() (cipher.AEAD, contract.CannotStartReason, bool) {
	block, err := aes.NewCipher(runMetaKey[:])
	if err != nil {
		return nil, contract.CannotStartReason{
			Kind:   contract.CannotStartRunMetaCipher,
			Origin: err.Error(),
		}, true
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, contract.CannotStartReason{
			Kind:   contract.CannotStartRunMetaGCM,
			Origin: err.Error(),
		}, true
	}
	return gcm, contract.CannotStartReason{}, false
}
