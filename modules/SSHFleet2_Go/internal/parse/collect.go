package parse

// 上传源的严格采集：确认屏之前把这次要传的东西清点一遍。
//
// 老版把这一步从执行期提到确认之前（`main.go` 步骤 7 前置），理由是要让「不能传的东西」
// （不可读文件、FIFO／设备／socket）在按下 y 之前就报出来——不再出现「确认屏看着正常、
// 点 y 之后才失败」。
//
// 四条规则，逐条照老版 `batch/collect.go`：
//
//   - **链接类一律过滤、不报错、留名**：软链接、Windows 目录联接、`.lnk` 快捷方式。
//     过滤不是「悄悄少传」，所以名字要记在纸上、确认屏要列出来、明细要逐个点名。
//   - **FIFO／设备／socket 一律按错报**：它们不是链接，过滤掉会静默改变语义。
//   - **逐项验证可读**：打不开的文件现在就报，别等传到一半。
//   - **只有过滤后一个可传文件都不剩时才报错**——这是「过滤」语义的必然结果。
//
// 相对路径不含上传根那一层（`-u modules -p /tmp/` ⇒ `/tmp/<内容>`），上传侧据此保留层级。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// collect 采集上传源。交回的可传文件表、被过滤的链接、根下一层的条目都装进纸上那一格。
//
// 建议并发与总大小在这里算一次（建议值按配置的上传阈值，再按节点数封顶）——
// 闸门拿纸上那个数问一句，不再自己求和：同一个数只算一处。
func collect(p *contract.Plan, cfg *settings) *contract.CannotStart {
	task := p.Task.Upload
	root := task.Source

	info, err := os.Lstat(root)
	if err != nil {
		return collectFail(fmt.Sprintf("-u 参数指定的上传文件或目录读不到：%s", root),
			message.SystemReason(err), "-u 后面要写本机上真实存在的文件或目录")
	}

	if !info.IsDir() {
		// 单文件：根路径本身是链接时同样过滤（没有可传目标 → 报错）。
		if isLinkish(root, info) {
			return collectFail(fmt.Sprintf("没有可上传的文件：-u 指定的路径本身是软链接/快捷方式：%s", root), "", "")
		}
		if why := specialTypeErr(root, info); why != "" {
			return collectFail(why, "", "")
		}
		task.Files = []contract.UploadFile{{Path: root, Rel: info.Name(), Size: info.Size()}}
		task.TotalBytes = info.Size()
		return nil
	}

	task.RootIsDir = true
	task.TopEntries = topEntries(root)

	err = filepath.Walk(root, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if isLinkish(path, fi) || strings.HasSuffix(strings.ToLower(fi.Name()), ".lnk") {
			task.Skipped = append(task.Skipped, relOf(root, path))
			return nil
		}
		if why := specialTypeErr(path, fi); why != "" {
			return errSkipUpload(why)
		}
		if fi.IsDir() {
			return nil
		}
		f, openErr := os.Open(path)
		if openErr != nil {
			return errSkipUpload(fmt.Sprintf("文件不可读: %s: %v", path, openErr))
		}
		_ = f.Close()

		task.Files = append(task.Files, contract.UploadFile{
			Path: path,
			Rel:  relOf(root, path),
			Size: fi.Size(),
		})
		task.TotalBytes += fi.Size()
		return nil
	})
	if err != nil {
		var stop uploadStop
		if asUploadStop(err, &stop) {
			return collectFail(string(stop), "", "")
		}
		return collectFail(fmt.Sprintf("-u 参数指定的上传目录读不了：%s", root),
			message.SystemReason(err), "请检查目录的读权限")
	}

	if len(task.Files) == 0 {
		return collectFail(fmt.Sprintf("没有可上传的文件：%s 下的文件全部是软链接/快捷方式（已过滤 %d 个：%s）",
			root, len(task.Skipped), summarize(task.Skipped)), "", "")
	}

	p.Run.ConcurrencyAdvice = suggestConcurrency(task.TotalBytes, cfg, len(p.Nodes))
	return nil
}

// topEntries 记下上传根下一层的条目：名字 + 是不是目录。
//
// **含空子目录与被过滤的链接**：确认屏那棵树按它摆，不靠文件表反推——反推会漏掉
// 空子目录，而老版那一屏列得出来。
func topEntries(root string) []contract.UploadEntry {
	items, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	out := make([]contract.UploadEntry, 0, len(items))
	for _, item := range items {
		out = append(out, contract.UploadEntry{Name: item.Name(), IsDir: item.IsDir()})
	}
	return out
}

// suggestConcurrency 按上传总大小给建议并发；0 表示没有建议（小文件不限制）。
//
// 阈值取自配置 `[upload]` 三段：小于 small_file 不限；大于 large_file 一次一个；
// 中间那段取 medium_parallel。最后按节点数封顶——建议不可能比要跑的机器还多。
func suggestConcurrency(total int64, cfg *settings, nodes int) int {
	t := cfg.upload
	advice := 0
	switch {
	case total < int64(t.SmallFile):
		advice = 0
	case total > int64(t.LargeFile):
		advice = 1
	default:
		advice = t.MediumParallel
	}
	if advice > nodes {
		advice = nodes
	}
	return advice
}

// isLinkish 判一个目录项是不是「链接类」：软链接，或 Windows 目录联接。
//
// Go 1.23 起 Windows 目录联接被标为 ModeIrregular 而不是 ModeSymlink，故 ModeIrregular
// 时用 Readlink 复核一次，避免把其它非常规文件误判成链接。
func isLinkish(path string, info os.FileInfo) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	if info.Mode()&os.ModeIrregular != 0 {
		_, err := os.Readlink(path)
		return err == nil
	}
	return false
}

// specialTypeErr FIFO／设备／socket 一律按错报：它们不是"链接"，过滤掉会静默改变语义。
func specialTypeErr(path string, info os.FileInfo) string {
	if info.Mode()&os.ModeNamedPipe != 0 || info.Mode()&os.ModeDevice != 0 || info.Mode()&os.ModeSocket != 0 {
		return fmt.Sprintf("不支持的文件类型: %s (FIFO/device/socket)", path)
	}
	return ""
}

// relOf 相对上传根的路径（POSIX 斜杠，远端拼接用）。
func relOf(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// summarize 把被过滤的条目拼成提示：超过 5 个时截断（清单可能几百个，一句提示刷满屏没有意义）。
func summarize(names []string) string {
	const maxShow = 5
	if len(names) <= maxShow {
		return strings.Join(names, "、")
	}
	return strings.Join(names[:maxShow], "、") + fmt.Sprintf(" 等 %d 个", len(names))
}

// uploadStop 是「遍历中途要停」的那条原因：它带着老版那句话，不再包装一层。
type uploadStop string

func (s uploadStop) Error() string { return string(s) }

// errSkipUpload 把遍历里遇到的那句话变成一个能停住 Walk 的错误值。
func errSkipUpload(why string) error { return uploadStop(why) }

// asUploadStop 认得出遍历里我们自己抛的那条原因。
func asUploadStop(err error, target *uploadStop) bool {
	s, ok := err.(uploadStop)
	if ok {
		*target = s
	}
	return ok
}

// —— 与「起不来」的接口 ——

// collectFail 采集失败：老版这一处用 where=upload 排屏，不带合规外壳。
func collectFail(problem, reason, hint string) *contract.CannotStart {
	body := problem
	switch {
	case reason != "" && hint != "":
		body = message.Problem(problem, reason, hint)
	case hint != "":
		body = message.ProblemHint(problem, hint)
	}
	return cannotStart("upload", "", body)
}
