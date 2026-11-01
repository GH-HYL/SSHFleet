package parse

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

// materialHead 是老版那层外壳句。
const materialHead = "选项/参数合规性检查未通过"

// checkMaterial 检查脚本、上传源、下载目录、清单文件。返回非空即「起不来」。
func checkMaterial(p *contract.Plan, given map[string]bool, vals map[string]string) *contract.CannotStart {
	// -s：脚本文件读得到、非空、非二进制、UTF-8；正文只读一次，危险检测与下发共用。
	if script := vals["--script"]; script != "" {
		info, err := os.Stat(script)
		if err == nil && info.IsDir() {
			return materialFail(fmt.Sprintf("-s 参数指定的路径是目录，不是脚本文件：%s\n提示：请指向一个脚本文件", script))
		}
		data, rerr := os.ReadFile(script)
		if rerr != nil {
			return materialFail(message.Problem(
				fmt.Sprintf("-s 参数指定的脚本文件读不到：%s", script),
				rerr.Error(),
				"请检查文件是否存在、当前用户有没有读权限"))
		}
		if len(data) == 0 {
			return materialFail(fmt.Sprintf("-s 参数指定的脚本文件为空：%s\n提示：请先写入内容再执行", script))
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return materialFail(fmt.Sprintf("-s 参数指定的脚本是二进制文件：%s\n提示：请换一个文本格式的脚本", script))
		}
		if !utf8ValidExceptBOM(data) {
			return materialFail(fmt.Sprintf("-s 参数指定的脚本不是 UTF-8 编码：%s\n提示：请把脚本转成 UTF-8 后再执行", script))
		}
		if p.Task.Script != nil {
			p.Task.Script.Body = string(data)
		}
	}

	// -u：上传源存在、不是符号链接、目录里至少有一个真文件。
	if upload := vals["--upload"]; upload != "" {
		info, err := os.Lstat(upload)
		if err != nil {
			return materialFail(message.Problem(
				fmt.Sprintf("-u 参数指定的上传文件或目录读不到：%s", upload),
				err.Error(),
				"-u 后面要写本机上真实存在的文件或目录"))
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return materialFail(fmt.Sprintf("-u 参数指定的上传文件或目录是符号链接，不能上传：%s\n提示：请直接写它指向的真实文件或目录", upload))
		}
		if info.IsDir() && !dirHasRealFile(upload) {
			return materialFail(fmt.Sprintf("-u 参数指定的上传目录及其所有子目录中都没有真正的文件（只有符号链接）：%s\n提示：请改上传真实文件所在的目录", upload))
		}
	}

	// -d 的 -p：本机目录，要先建好。
	if vals["--download"] != "" && vals["--path"] != "" {
		info, err := os.Stat(vals["--path"])
		if err != nil {
			return materialFail(message.Problem(
				fmt.Sprintf("下载模式：-p 参数指定的本地目录读不到：%s", vals["--path"]),
				err.Error(),
				"这个目录要在本机上先建好，工具不会替你建"))
		}
		if !info.IsDir() {
			return materialFail(fmt.Sprintf("下载模式：-p 参数指定的本地路径不是目录：%s\n提示：这个目录要在本机上先建好，工具不会替你建", vals["--path"]))
		}
	}

	// -f：是个文件就查读得了、非空、非二进制；不是文件就当一条内联记录（内容层随阶段 04）。
	if csv := vals["--csv-file"]; csv != "" {
		if info, err := os.Stat(csv); err == nil && !info.IsDir() {
			data, rerr := os.ReadFile(csv)
			if rerr != nil {
				return materialFail(message.Problem(
					fmt.Sprintf("-f 参数指定的 CSV 文件读不到：%s", csv),
					rerr.Error(),
					"请检查文件是否存在、当前用户有没有读权限"))
			}
			if len(data) == 0 {
				return materialFail(fmt.Sprintf("-f 参数指定的 CSV 文件为空：%s\n提示：一行写一台机器，只写 IP 就能跑（端口、用户名、密码走配置）", csv))
			}
			if bytes.IndexByte(data, 0) >= 0 {
				return materialFail(fmt.Sprintf("-f 参数指定的清单是二进制文件：%s\n提示：清单要是文本文件（CSV）", csv))
			}
		}
	}

	_ = given
	_ = p
	return nil
}

// materialFail 包一层老版那句外壳，再按「起不来」排屏。
func materialFail(body string) *contract.CannotStart {
	return cannotStart("cli", materialHead, body)
}

// dirHasRealFile 目录树里有没有真实文件（不是符号链接）。
func dirHasRealFile(root string) bool {
	found := false
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return fs.SkipAll
		}
		if d.Type().IsRegular() {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// utf8ValidExceptBOM 判内容为合法 UTF-8（允许带 BOM）。
func utf8ValidExceptBOM(data []byte) bool {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	return utf8.Valid(data)
}

// trimCRLF 去掉尾部换行，拼句时不留空行。
func trimCRLF(s string) string { return strings.TrimRight(s, "\r\n") }
