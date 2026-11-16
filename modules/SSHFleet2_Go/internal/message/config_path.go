package message

// 配置里的路径用不了那一族（措辞迁移 06 号单）：`credential.key`、`account.password` 这些格子
// 写了个用不了的路径。
//
// 一处错是四段：哪一格 ＋ 写的什么（问题那行）、为什么不行（原因）、怎么改（提示，带本机例子）。
// 原先由 `parse` 的 `pathErr` 闭包用三句话拼出来，现在只在 message 成形。屏上一个字节没改。

// PathProblemKind 是「路径用不了」的一种毛病。类别住这里，与 `ValueInvalidReason` 同一套写法：
// 站里只挑一个值，措辞与版式归本包。
type PathProblemKind string

const (
	// PathTilde 是写了 `~` 开头：这个工具不展开家目录。
	PathTilde PathProblemKind = "tilde"
	// PathNoDrive 是本机 Windows、这个写法没有盘符（`/opt/keys` 这种）。
	PathNoDrive PathProblemKind = "no_drive"
	// PathRelative 是相对路径：配置里的路径一律要全路径，相对路径只在清单里能用。
	PathRelative PathProblemKind = "relative"
	// PathDriveOnUnix 是本机 Unix 系、写的是 Windows 盘符（`D:/Keys`）。
	PathDriveOnUnix PathProblemKind = "drive_on_unix"
	// PathUncOnUnix 是本机 Unix 系、写的是 Windows 网络路径（`//host/share`）。
	PathUncOnUnix PathProblemKind = "unc_on_unix"
)

// pathProblems 是每一种毛病那两截：问题行里「哪里不对」那半句，与「原因」那一行。
//
// 三种「不是本机的绝对路径」共用一个 head、各自一句原因——下一步要改的东西不同。
var pathProblems = map[PathProblemKind]struct{ head, reason string }{
	PathTilde:       {"不支持 ~", "~ 不会被展开，写什么就按什么找"},
	PathNoDrive:     {"不是本机的绝对路径", "本机是 Windows，这个写法没有盘符"},
	PathRelative:    {"必须是绝对路径", "配置里的路径一律写全路径；相对路径只在清单里能用"},
	PathDriveOnUnix: {"不是本机的绝对路径", "本机是 Unix 系系统，这是 Windows 盘符写法"},
	PathUncOnUnix:   {"不是本机的绝对路径", "本机是 Unix 系系统，这是 Windows 网络路径（UNC）写法"},
}

// ConfigPathProblem 拼配置里路径那一屏：`<哪一格> <哪里不对>，当前值：<写的是什么>`，
// 后面接「原因：」「提示：」两行。
//
// windows 决定提示里的例子与去路（`D:/Keys` 还是 `/home/ops/.keys`）——两种系统的下一步不是
// 同一个写法，所以分两版而不是含糊成一句。
func ConfigPathProblem(field, raw string, kind PathProblemKind, windows bool) string {
	p, ok := pathProblems[kind]
	if !ok {
		panic("配置路径：没见过的毛病类别 " + string(kind))
	}
	return Problem(
		CurrentValue(field+" "+p.head, raw),
		p.reason,
		pathFixHint(windows))
}

// pathFixHint 是「提示」那一行：按本机是哪种系统给一个能照抄的例子。
func pathFixHint(windows bool) string {
	if windows {
		return "写成带盘符的本机路径，例如 D:/Keys"
	}
	return "写成以 / 开头的本机路径，例如 /home/ops/.keys"
}
