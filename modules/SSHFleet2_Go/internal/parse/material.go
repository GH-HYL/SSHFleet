package parse

import (
	"bytes"
	"fmt"
	"os"
	"unicode/utf8"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// 材料自检：这一层管「使用者给的这份料，工具到底拿不拿得到、能不能当文本用」。
//
// 缺的是工具要用的东西（脚本、上传源、下载目录、清单文件）→ 「起不来」，由解析造；
// 对不上的是这次给的东西（参数搭配、取值、危险命令）→ 「拒绝」，由闸门造。
//
// 外壳句与话术逐字照老版：这几条在老版里都经「选项/参数合规性检查未通过」那层壳打出。

// checkMaterial 检查脚本、上传源、下载目录、清单文件。返回非空即「起不来」。
//
// 每一条只交「哪件事、哪段原文、系统说了什么」，三段话的行与标签由 `message` 加
// （ADR-0028 决定一、五：标签只许在构造点）。
func checkMaterial(p *contract.Plan, given map[string]bool, vals map[string]string) *contract.CannotStart {
	// -s：脚本文件读得到、非空、非二进制、UTF-8；正文只读一次，危险检测与下发共用。
	if script := vals["--script"]; script != "" {
		info, err := os.Stat(script)
		if err == nil && info.IsDir() {
			return materialFail(message.ProblemHint(
				fmt.Sprintf("-s 参数指定的路径是目录，不是脚本文件：%s", script),
				"请指向一个脚本文件"))
		}
		data, rerr := os.ReadFile(script)
		if rerr != nil {
			return materialFail(message.Problem(
				fmt.Sprintf("-s 参数指定的脚本文件读不到：%s", script),
				message.SystemReason(rerr),
				"请检查文件是否存在、当前用户有没有读权限"))
		}
		if len(data) == 0 {
			return materialFail(message.ProblemHint(
				fmt.Sprintf("-s 参数指定的脚本文件为空：%s", script),
				"请先写入内容再执行"))
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return materialFail(message.ProblemHint(
				fmt.Sprintf("-s 参数指定的脚本是二进制文件：%s", script),
				"请换一个文本格式的脚本"))
		}
		if !utf8ValidExceptBOM(data) {
			return materialFail(message.ProblemHint(
				fmt.Sprintf("-s 参数指定的脚本不是 UTF-8 编码：%s", script),
				"请把脚本转成 UTF-8 后再执行"))
		}
		if p.Task.Script != nil {
			// 正文整备只此一处（剥 BOM、去首尾空白）：危险检测、长度判据与实际下发
			// 读的都是这一份。
			p.Task.Script.Body = scriptBody(data)
		}
	}

	// -d 的 -p：本机目录，要先建好。
	if vals["--download"] != "" && vals["--path"] != "" {
		info, err := os.Stat(vals["--path"])
		if err != nil {
			return materialFail(message.Problem(
				fmt.Sprintf("下载模式：-p 参数指定的本地目录读不到：%s", vals["--path"]),
				message.SystemReason(err),
				"这个目录要在本机上先建好，工具不会替你建"))
		}
		if !info.IsDir() {
			return materialFail(message.ProblemHint(
				fmt.Sprintf("下载模式：-p 参数指定的本地路径不是目录：%s", vals["--path"]),
				"这个目录要在本机上先建好，工具不会替你建"))
		}
	}

	// -f：是个文件就查读得了、非空、非二进制；不是文件就当一条内联记录。
	// 内容层（按 CSV 规则读出行来）归 nodelist.go，不在这里。
	if csv := vals["--csv-file"]; csv != "" {
		if info, err := os.Stat(csv); err == nil && !info.IsDir() {
			data, rerr := os.ReadFile(csv)
			if rerr != nil {
				return materialFail(message.Problem(
					fmt.Sprintf("-f 参数指定的 CSV 文件读不到：%s", csv),
					message.SystemReason(rerr),
					"请检查文件是否存在、当前用户有没有读权限"))
			}
			if len(data) == 0 {
				return materialFail(message.ProblemHint(
					fmt.Sprintf("-f 参数指定的 CSV 文件为空：%s", csv),
					"一行写一台机器，只写 IP 就能跑（端口、用户名、密码走配置）"))
			}
			if bytes.IndexByte(data, 0) >= 0 {
				return materialFail(message.ProblemHint(
					fmt.Sprintf("-f 参数指定的清单是二进制文件：%s", csv),
					"清单要是文本文件（CSV）"))
			}
		}
	}

	_ = given
	_ = p
	return nil
}

// materialFail 套上老版那句外壳，再按「起不来」排屏。外壳句住在 `message`，不在这里抄一份。
func materialFail(body string) *contract.CannotStart {
	return cannotStart("cli", message.HeadCompliance, body)
}

// utf8ValidExceptBOM 判内容为合法 UTF-8（允许带 BOM）。
func utf8ValidExceptBOM(data []byte) bool {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	return utf8.Valid(data)
}
