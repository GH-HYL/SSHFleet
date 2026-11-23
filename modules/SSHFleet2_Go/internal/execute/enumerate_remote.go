package execute

// 远端枚举：一条 find 拿全（ADR-0063 决定 4）。
//
// 主路要类型、大小、相对路径一起给，NUL 收尾——名字里带换行也不会被拆成两条。目标机不认
// `-printf` 时（BusyBox 一类）走退路：`find -print0` 只给名字，大小与类型由同一条 SFTP
// 会话逐个 `Lstat` 补。退路准，但每个文件一次往返，只在少数机器上用。
//
// 不走协议自己的目录遍历（`ReadDir`／`Walk`）：每进一层一次往返，深树上慢一个量级。

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"sshfleet2/internal/criterion"
)

const (
	// 主路：`F 大小 相对路径` 与 `L 相对路径`，都以 NUL 收尾。
	findPrintfCmd = `find '%s' \( -type f -printf 'F %%s %%P\0' \) -o \( -type l -printf 'L %%P\0' \)`
	// 退路：只有名字，同样 NUL 收尾。
	findPrint0Cmd = `find '%s' \( -type f -print0 \) -o \( -type l -print0 \)`
)

// findRunner 跑一条枚举命令：交回输出、这一次发起的结局、命令本身的错误。
type findRunner func(command string) (string, armEnd, error)

// lstatRunner 问一次远端路径的属性：退路拿大小与「是不是软链接」，同样归传输预算管。
type lstatRunner func(path string) (os.FileInfo, armEnd, error)

// enumerateRemote 枚举 root 下的真文件与软链接；软链接只交回相对路径。
func enumerateRemote(root string, useSudo bool, run findRunner, lstat lstatRunner) ([]fileEntry, []string, armEnd, error) {
	cmd := fmt.Sprintf(findPrintfCmd, criterion.EscapeSingle(root))
	if useSudo {
		cmd = "sudo " + cmd
	}
	out, end, err := run(cmd)
	if !end.ran() {
		return nil, nil, end, nil
	}
	if err == nil {
		files, links := parseFindRecords(out)
		return files, links, armRan, nil
	}

	fallback := fmt.Sprintf(findPrint0Cmd, criterion.EscapeSingle(root))
	if useSudo {
		fallback = "sudo " + fallback
	}
	names, end, fbErr := run(fallback)
	if !end.ran() {
		return nil, nil, end, nil
	}
	if fbErr != nil {
		// 报退路这条：主路的失败多半只是「不认 -printf」，那是预期内的，没有信息量。
		return nil, nil, armRan, fbErr
	}
	files, links, end, err := classifyFindNames(root, names, lstat)
	return files, links, end, err
}

// parseFindRecords 主路的输出：按 NUL 切，只认真的两种记录。
func parseFindRecords(out string) ([]fileEntry, []string) {
	var (
		files []fileEntry
		links []string
	)
	for _, rec := range splitRecords(out) {
		switch {
		case strings.HasPrefix(rec, "F "):
			parts := strings.SplitN(rec[2:], " ", 2)
			if len(parts) != 2 {
				continue
			}
			size, err := strconv.ParseInt(parts[0], 10, 64)
			if err != nil {
				continue
			}
			files = append(files, fileEntry{Rel: parts[1], Size: size})
		case strings.HasPrefix(rec, "L "):
			links = append(links, rec[2:])
		}
	}
	return files, links
}

// classifyFindNames 退路的输出：只有完整路径，大小与「是不是软链接」逐个问同一会话。
//
// 每一个 `Lstat` 也是一次发起，同样归传输预算管——几百个名字就是几百次，其中任何一次卡住
// 都要能到点解开，所以问法由调用方交进来（`lstat`）。
//
// `Lstat` 失败按枚举失败交回去：那时不知道它多大，接着传只会在核大小那一步对不上。
func classifyFindNames(root, out string, lstat lstatRunner) ([]fileEntry, []string, armEnd, error) {
	base := strings.TrimRight(root, "/")
	var (
		files []fileEntry
		links []string
	)
	for _, full := range splitRecords(out) {
		if base != "" && !strings.HasPrefix(full, base+"/") {
			// 不在这次枚举的根底下：远端在 stdout 上说了别的话。
			continue
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(full, base), "/")
		if rel == "" {
			continue
		}
		info, end, err := lstat(full)
		if !end.ran() {
			return nil, nil, end, nil
		}
		if err != nil {
			return nil, nil, armRan, fmt.Errorf("lstat %s: %w", full, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			links = append(links, rel)
			continue
		}
		files = append(files, fileEntry{Rel: rel, Size: info.Size()})
	}
	return files, links, armRan, nil
}
