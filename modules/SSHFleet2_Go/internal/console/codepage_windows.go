//go:build windows

package console

import (
	"sync"
	"unsafe"
)

// cpUTF8 是 UTF-8 的代码页号。
const cpUTF8 uint32 = 65001

// 控制台模式位：ENABLE_VIRTUAL_TERMINAL_PROCESSING（0x0004）。
const enableVTProcessing uint32 = 0x0004

// 码页是控制台缓冲区上的全局状态：改过才还，且只还一次。这份状态只有本包读写——
// 终端适配是 console 自己的那一件事。
var (
	cpMu      sync.Mutex
	cpOrig    uint32
	cpChanged bool
)

var (
	procGetConsoleOutputCP = kernel32.NewProc("GetConsoleOutputCP")
	procSetConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
	procGetConsoleMode     = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode     = kernel32.NewProc("SetConsoleMode")
)

// adaptCodepage 把输出码页切成 UTF-8，并把原值记下来等 Restore 还回去。
//
// 为什么由程序自己做：我们写出去的是 UTF-8 字节，而 Windows 出厂控制台是 936 码页，
// 同一段字节会被按 GBK 解成乱码。只声明、不转码——字节链路一个字节不动。
//
// 三种情形直接不管：拿不到句柄、句柄不是控制台（重定向／管道，字节直接进文件）、
// 码页本来就是 65001。改不成也不拦：显示不对总比跑不起来强。
func adaptCodepage() {
	cpMu.Lock()
	defer cpMu.Unlock()
	if cpChanged {
		return
	}
	h, _, _ := procGetStdHandle.Call(stdOutputHandle)
	if h == 0 || h == ^uintptr(0) {
		return
	}
	var mode uint32
	if r, _, _ := procGetConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode))); r == 0 {
		return // 不是控制台
	}
	orig, _, _ := procGetConsoleOutputCP.Call()
	if uint32(orig) == 0 || uint32(orig) == cpUTF8 {
		return
	}
	if r, _, _ := procSetConsoleOutputCP.Call(uintptr(cpUTF8)); r == 0 {
		return
	}
	cpOrig = uint32(orig)
	cpChanged = true
}

// restoreCodepage 把码页还回原值。没改过就什么都不做；调重了也只还一次。
func restoreCodepage() {
	cpMu.Lock()
	defer cpMu.Unlock()
	if !cpChanged {
		return
	}
	_, _, _ = procSetConsoleOutputCP.Call(uintptr(cpOrig))
	cpChanged = false
}
