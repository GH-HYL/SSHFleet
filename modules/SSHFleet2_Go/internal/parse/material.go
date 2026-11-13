package parse

import (
	"bytes"
	"os"
	"unicode/utf8"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/criterion"
	"sshfleet2/internal/message"
)

// 材料自检：这一层管「使用者给的这份料，工具到底拿不拿得到、能不能当文本用」。
//
// 缺的是工具要用的东西（脚本、上传源、下载目录、清单文件）→ 「起不来」，由解析造；
// 对不上的是这次给的东西（参数搭配、取值、危险命令）→ 「拒绝」，由闸门造。
//
// **次序是第五层里的事**（ADR-0052 决定二）：脚本 → 上传源 → 下载目录 → 清单。
// 这一层只判「读得到、非空、非二进制」，清单的语法与逐行归内容那一层。
//
// 这里的失败**不当场报**：解析站照旧读（判决要用正文、屏要用节点），失败作为一条延后的
// 「起不来」随纸交给塔尖，等前面几层都过完才说得出口。

// materialScript 查 `-s` 那份脚本：读得到、非空、非二进制、UTF-8。
//
// 正文只读一次，危险检测与下发共用。
func materialScript(p *contract.Plan, vals map[string]string) *contract.CannotStart {
	script := vals["--script"]
	if script == "" {
		return nil
	}
	info, err := os.Stat(script)
	if err == nil && info.IsDir() {
		return cliReason(contract.CannotStartReason{
			Kind:  contract.CannotStartScriptIsDir,
			Place: script,
		})
	}
	data, rerr := os.ReadFile(script)
	if rerr != nil {
		return cliReason(contract.CannotStartReason{
			Kind:   contract.CannotStartScriptUnreadable,
			Place:  script,
			Origin: criterion.SystemReason(rerr),
		})
	}
	if fail := textMaterial(data, contract.CannotStartScriptEmpty, contract.CannotStartScriptBinary, script); fail != nil {
		return fail
	}
	if !utf8ValidExceptBOM(data) {
		return cliReason(contract.CannotStartReason{
			Kind:  contract.CannotStartScriptNotUTF8,
			Place: script,
		})
	}
	if p.Task.Script != nil {
		// 正文整备只此一处（剥 BOM、去首尾空白）：危险检测、长度判据与实际下发
		// 读的都是这一份。
		p.Task.Script.Body = scriptBody(data)
	}
	return nil
}

// textMaterial 判一份「文本料」的两件共同事：非空、非二进制。
//
// 脚本与清单都要过这两条，判据同一处成形——各拼一遍会各漏一遍；两份料的检查项本来
// 还可以再加（脚本另有 UTF-8 一条），加在调用点，不在这里。
func textMaterial(data []byte, emptyKind, binaryKind contract.CannotStartKind, place string) *contract.CannotStart {
	switch {
	case len(data) == 0:
		return cliReason(contract.CannotStartReason{Kind: emptyKind, Place: place})
	case bytes.IndexByte(data, 0) >= 0:
		return cliReason(contract.CannotStartReason{Kind: binaryKind, Place: place})
	}
	return nil
}

// materialDownloadDir 查 `-d` 的 `-p`：本机目录，要先建好。
//
// 读纸上那一格（`p.Task.Path`），不读命令行的原值：纸上的值是去过首尾空白的，
// 自检要是另读原值，同一个 `-p` 会出现「纸面通过、自检说读不到」。
func materialDownloadDir(p *contract.Plan, vals map[string]string) *contract.CannotStart {
	if vals["--download"] == "" || p.Task.Path == "" {
		return nil
	}
	info, err := os.Stat(p.Task.Path)
	if err != nil {
		return cliReason(contract.CannotStartReason{
			Kind:   contract.CannotStartDownloadPathUnreadable,
			Place:  p.Task.Path,
			Origin: criterion.SystemReason(err),
		})
	}
	if !info.IsDir() {
		return cliReason(contract.CannotStartReason{
			Kind:  contract.CannotStartDownloadPathNotDir,
			Place: p.Task.Path,
		})
	}
	return nil
}

// cliReason 是「cli 环节 ＋ 合规外壳」那一屏：类别与参数由调用点填，句子由 `message` 成形
// （ADR-0044）。材料自检、代填、改密取值、密钥命令那几处都用它。
func cliReason(reason contract.CannotStartReason) *contract.CannotStart {
	reason.Head = message.HeadCompliance
	return cannotStartReason("cli", reason)
}

// utf8ValidExceptBOM 判内容为合法 UTF-8（允许带 BOM）。
func utf8ValidExceptBOM(data []byte) bool {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	return utf8.Valid(data)
}
