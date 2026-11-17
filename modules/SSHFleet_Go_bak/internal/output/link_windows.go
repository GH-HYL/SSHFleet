//go:build windows

package output

import (
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// mountPointReparseBuffer 对应 Windows 的 MOUNT_POINT_REPARSE_BUFFER：
// 4 个 USHORT 头 + 路径缓冲（替代名与显示名首尾相接，各自以 NUL 结尾）。
type mountPointReparseBuffer struct {
	SubstituteNameOffset uint16
	SubstituteNameLength uint16
	PrintNameOffset      uint16
	PrintNameLength      uint16
	PathBuffer           [1]uint16
}

// reparseDataBuffer 对应 REPARSE_DATA_BUFFER 的挂载点子集。
// 头 8 字节（Tag 4 + 数据长度 2 + 保留 2），联合体从偏移 8 开始——字段顺序与对齐
// 必须与 C 结构一致，故按内存布局直接填充（与 Go 标准库 os 包的读侧做法同源）。
type reparseDataBuffer struct {
	ReparseTag        uint32
	ReparseDataLength uint16
	Reserved          uint16
	MountPoint        mountPointReparseBuffer
}

// createDirLink 在 Windows 上建「目录联接（junction）」，作为 latest_history 的等价物。
//
// 与符号链接一样能当目录用（`cd latest_history`、资源管理器双击、按路径读文件都通），
// 但**不需要管理员权限或开发者模式**——符号链接需要 SeCreateSymbolicLinkPrivilege，
// 那正是旧版在 Windows 上没做成的根因。
//
// 原生 DeviceIoControl 实现：不起子进程、不进 cmd，省掉控制台闪窗与中文/空格路径的
// 引号问题。限制：目标须是本地 NTFS 卷上的目录（联接点不支持网络路径）。
func createDirLink(link, target string) error {
	absLink, err := filepath.Abs(link)
	if err != nil {
		return err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}

	// 联接点是在「已存在且为空」的目录上设置重解析数据
	if err := os.Mkdir(absLink, 0o755); err != nil {
		return err
	}
	if err := setMountPoint(absLink, absTarget); err != nil {
		_ = os.Remove(absLink)
		return err
	}
	return nil
}

// setMountPoint 为已存在的空目录写入挂载点重解析数据。
func setMountPoint(linkPath, targetPath string) error {
	handle, err := windows.CreateFile(
		windows.StringToUTF16Ptr(linkPath),
		windows.GENERIC_WRITE,
		0, nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)

	// 替代名走内核命名空间（\??\ 前缀），显示名留人可读的普通路径
	substitute, err := windows.UTF16FromString(`\??\` + targetPath)
	if err != nil {
		return err
	}
	printName, err := windows.UTF16FromString(targetPath)
	if err != nil {
		return err
	}

	// 缓冲 = 8 字节头 + 8 字节挂载点字段 + 两条路径（含各自 NUL）
	words := len(substitute) + len(printName)
	buf := make([]byte, 16+words*2)
	rdb := (*reparseDataBuffer)(unsafe.Pointer(&buf[0]))
	rdb.ReparseTag = windows.IO_REPARSE_TAG_MOUNT_POINT
	rdb.ReparseDataLength = uint16(8 + words*2)
	rdb.MountPoint.SubstituteNameOffset = 0
	rdb.MountPoint.SubstituteNameLength = uint16(len(substitute)-1) * 2
	rdb.MountPoint.PrintNameOffset = uint16(len(substitute)) * 2
	rdb.MountPoint.PrintNameLength = uint16(len(printName)-1) * 2

	path := unsafe.Slice(&rdb.MountPoint.PathBuffer[0], words)
	copy(path, substitute)
	copy(path[len(substitute):], printName)

	var returned uint32
	return windows.DeviceIoControl(
		handle,
		windows.FSCTL_SET_REPARSE_POINT,
		&buf[0], uint32(len(buf)),
		nil, 0,
		&returned, nil,
	)
}
