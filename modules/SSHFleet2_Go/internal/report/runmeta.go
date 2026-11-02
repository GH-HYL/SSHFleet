package report

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"fmt"

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
func openRunMeta(sealed contract.RunMeta) (contract.RunMetaBody, error) {
	var body contract.RunMetaBody
	if len(sealed) < 1+contract.RunMetaNonceSize+contract.RunMetaTagSize {
		return body, fmt.Errorf("运行上下文：长度不对（%d 字节）", len(sealed))
	}
	if sealed[0] != contract.RunMetaVersion {
		return body, fmt.Errorf("运行上下文：版本 %d 认不出", sealed[0])
	}
	gcm, err := runMetaGCM()
	if err != nil {
		return body, err
	}
	nonce := sealed[1 : 1+contract.RunMetaNonceSize]
	plain, err := gcm.Open(nil, nonce, sealed[1+contract.RunMetaNonceSize:], nil)
	if err != nil {
		return body, fmt.Errorf("运行上下文：解不开（密钥或格式两处漂了）：%w", err)
	}
	if err := json.Unmarshal(plain, &body); err != nil {
		return body, fmt.Errorf("运行上下文：内容认不出：%w", err)
	}
	return body, nil
}

// runMetaGCM 与解析站那一段同形：密钥长度写死，建不起来说明写死的值错了。
func runMetaGCM() (cipher.AEAD, error) {
	block, err := aes.NewCipher(runMetaKey[:])
	if err != nil {
		return nil, fmt.Errorf("运行上下文：建不出密码器：%w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("运行上下文：建不出 GCM：%w", err)
	}
	return gcm, nil
}
