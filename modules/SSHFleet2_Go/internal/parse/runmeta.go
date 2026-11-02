package parse

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"sshfleet2/internal/contract"
)

// 运行上下文的封。解在 internal/report/runmeta.go —— **两段代码各写一遍**，这是
// ADR-0035 决定九给的两条路里省掉一个包的那条，代价是改算法要改两处、写歪了当场对不上。
//
// 为什么封：见 contract/runmeta.go 的注释。一句话——报告第一段要原值，而原值一路直传
// 会让中间站变成「拿到不用、原样转手」，封成看不懂的字节拦的是顺手。

// runMetaKey 与报告站那一份**逐字节相同**。密钥写死两处、各一次，不留变量、不往下传
// （ADR-0035 决定九）。改它要同时改 internal/report/runmeta.go：两边各有一条固定密文
// 样本，谁改了一边，那边的测试立刻红。
var runMetaKey = [32]byte{
	0x63, 0xf6, 0xfc, 0x2c, 0x9c, 0xdc, 0x96, 0x26,
	0x89, 0x0b, 0xbe, 0x1c, 0xb1, 0x5d, 0x76, 0xe5,
	0x73, 0x3e, 0x35, 0x40, 0x44, 0x71, 0x21, 0xc6,
	0x02, 0x47, 0xc3, 0x50, 0xf3, 0xb0, 0xe2, 0x7f,
}

// sealRunMeta 把这次的原值封成纸上那一格。
//
// nonce 不用随机数，而取「明文哈希的前 12 字节」：这样同一份原值封出来的字节永远一样，
// 两处代码（封与解）就能各拿一条**固定的密文样本**对账——谁把密钥或格式写歪了，样本立刻
// 对不上。GCM 对 nonce 的要求是「同一把密钥下不同消息不撞」，取明文哈希满足它；代价是
// 相同原值封出相同字节，泄漏的只是「这两次的命令行一样」，那不是秘密。
func sealRunMeta(body contract.RunMetaBody) (contract.RunMeta, error) {
	plain, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("封装运行上下文：%w", err)
	}
	gcm, err := runMetaGCM()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(plain)
	nonce := sum[:contract.RunMetaNonceSize]

	out := make([]byte, 0, 1+contract.RunMetaNonceSize+len(plain)+contract.RunMetaTagSize)
	out = append(out, contract.RunMetaVersion)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, plain, nil), nil
}

// runMetaBodyOf 把这次的原值收进那一格的内容里。
//
// 收的全是报告站出那两样要用的东西：整条命令行、模式、各模式的原值、这次生效的执行参数，
// 再加两样「呈现口径」——提示开关与判据文件的展示路径。它不重算任何东西：执行参数取的是
// 纸上算好的那几个数，不是再解一遍命令行；那两样直接取配置里的原值。
func runMetaBodyOf(p *contract.Plan, argv []string, cfg *settings) contract.RunMetaBody {
	body := contract.RunMetaBody{
		Argv:            append([]string(nil), argv...),
		Mode:            p.Mode,
		Answers:         p.Run.Answer,
		Concurrency:     p.Run.Concurrency,
		TimeoutConnect:  p.Run.TimeoutConnect,
		TimeoutExecute:  p.Run.TimeoutExecute,
		TimeoutTransfer: p.Run.TimeoutTransfer,

		ShowCategoryTips:  cfg.enable.ShowCategoryTips,
		ErrorKeywordsPath: displayPath(errorKeywordsPath),
	}
	switch {
	case p.Task.Command != nil:
		body.Command = p.Task.Command.Text
	case p.Task.Script != nil:
		body.ScriptPath = p.Task.Script.Path
	case p.Task.Upload != nil:
		body.Source = p.Task.Upload.Source
		body.Target = p.Task.Path
	case p.Task.Download != nil:
		body.Source = p.Task.Download.RemotePath
		body.Target = p.Task.Path
	}
	return body
}

// runMetaGCM 建出那次用的 GCM；建不起来说明密钥长度不对（写死的东西不该发生）。
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

// displayPath 给使用者看的路径形态：去掉开头的 `./`，它对使用者没有意义（老版
// `config.DisplayPath` 同一形态、同一个道理）。
//
// 算在解析站这一侧，是因为报「判据文件在哪」只该有一处口径：报告站按纸印，不认路径怎么归一。
func displayPath(p string) string { return strings.TrimPrefix(p, "./") }
