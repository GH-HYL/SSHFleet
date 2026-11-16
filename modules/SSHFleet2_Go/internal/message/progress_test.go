package message

import (
	"errors"
	"testing"
	"time"

	"sshfleet2/internal/contract"
)

// 进度条与传输明细上那几个词：期望值一律写死字面量，不拿被测函数再算一遍。
func TestProgressLabels(t *testing.T) {
	if got := ActionProgress(contract.ModeUpload); got != "上传进度" {
		t.Fatalf("上传进度条标题：%q", got)
	}
	if got := ActionProgress(contract.ModeDownload); got != "下载进度" {
		t.Fatalf("下载进度条标题：%q", got)
	}
	if got := TotalProgressLabel(); got != "总进度" {
		t.Fatalf("总进度标题：%q", got)
	}
	if got := NodeProgressLabel(); got != "节点进度" {
		t.Fatalf("节点进度标题：%q", got)
	}
	if got := DoneRatio(3, 5); got != "已完成 3/5" {
		t.Fatalf("已完成那一格：%q", got)
	}
	if got := CountOK(3); got != "成功 3" {
		t.Fatalf("成功计数：%q", got)
	}
	if got := CountFail(1); got != "失败 1" {
		t.Fatalf("失败计数：%q", got)
	}
	if got := FileCounts(10, 9, 1); got != "共 10 个文件  成功 9  失败 1" {
		t.Fatalf("文件条数那一段：%q", got)
	}
}

// 耗时四种形态各钉一条：进位、四舍五入、秒位补零。
func TestElapsed(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0:00:00"},
		{12 * time.Second, "0:00:12"},
		{90 * time.Second, "0:01:30"},
		{3661 * time.Second, "1:01:01"},
		{1500 * time.Millisecond, "0:00:02"}, // 往上取整
	}
	for _, c := range cases {
		if got := Elapsed(c.d); got != c.want {
			t.Fatalf("%v 写成：%q，该是 %q", c.d, got, c.want)
		}
	}
}

// 传输明细两行：跳过与失败各一条，失败那行上传下载各钉一次——动词随模式变。
func TestTransferLines(t *testing.T) {
	if got := TransferSkipped("a/link"); got != "a/link: 已跳过（符号链接）" {
		t.Fatalf("跳过那一行：%q", got)
	}
	if got := TransferFailure(contract.ModeUpload, "a.txt", "file already exists on the remote host"); got !=
		"a.txt: 上传失败 - file already exists on the remote host" {
		t.Fatalf("上传失败那一行：%q", got)
	}
	if got := TransferFailure(contract.ModeDownload, "a.txt", errors.New("permission denied")); got !=
		"a.txt: 下载失败 - permission denied" {
		t.Fatalf("下载失败那一行：%q", got)
	}
}

func TestNoAuthMethod(t *testing.T) {
	if got := NoAuthMethod(); got != "未提供有效的认证方式（密码或密钥至少提供一种）" {
		t.Fatalf("认证方式那一句：%q", got)
	}
}
