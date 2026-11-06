package message

import (
	"errors"
	"strings"
	"testing"
)

// 「最新一次」链接那两句：老版原话，黄色 [警告] 走标准错误。
func TestLatestLinkWarnings(t *testing.T) {
	skipped := LatestLinkSkipped("latest_history")
	if !strings.Contains(skipped.Text(), "当前目录已存在同名文件 latest_history，跳过链接创建（如需快捷入口，删除该文件后重跑）") {
		t.Fatalf("跳过那句不是老版原话：%q", skipped.Text())
	}
	if !skipped.Stderr() {
		t.Fatal("警告应当走标准错误（老版就是 stderr）")
	}

	failed := LatestLinkFailed("latest_history", errors.New("mklink: boom"))
	text := failed.Text()
	if !strings.Contains(text, "创建 latest_history 目录链接失败：mklink: boom") {
		t.Fatalf("失败那句没带上原因：%q", text)
	}
	if !strings.Contains(text, "提示：Windows 使用目录联接（需目标位于本地 NTFS 卷），POSIX 使用符号链接") {
		t.Fatalf("失败那句没带「怎么绕」：%q", text)
	}
	if !failed.Stderr() {
		t.Fatal("警告应当走标准错误")
	}
}

// 落日志那一版是同一句话去掉色码与结尾换行：日志是纯文本文件，色码写进去只是乱码。
func TestLinePlainDropsColorAndNewline(t *testing.T) {
	w := LatestLinkSkipped("latest_history")
	want := "[警告] 当前目录已存在同名文件 latest_history，跳过链接创建（如需快捷入口，删除该文件后重跑）"
	if got := w.Plain(); got != want {
		t.Fatalf("去色后的那句不对：\n got=%q\nwant=%q", got, want)
	}
	if strings.Contains(w.Text(), "\x1b") == false {
		t.Fatal("样本本身该带色码，否则这条用例什么都没验")
	}
}
