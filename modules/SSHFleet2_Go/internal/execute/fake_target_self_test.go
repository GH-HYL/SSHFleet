package execute

// 01 号工单：假目标机自己得先能用——连得上、跑得动命令、传得进去文件。
// 先绿了这一条，02、03 才敢从执行站的入口进。

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func TestFakeTargetServesCommandAndFiles(t *testing.T) {
	ft := startFakeTarget(t)
	c := ft.dial(t)

	// 一、跑一条命令：拿回输出，退出码 0。
	sess, err := c.NewSession()
	if err != nil {
		t.Fatalf("开会话失败：%v", err)
	}
	out, err := sess.Output("echo hi")
	_ = sess.Close()
	if err != nil {
		t.Fatalf("执行命令失败：%v", err)
	}
	if got := string(out); got != "hi\n" {
		t.Fatalf("命令输出 want %q，got %q", "hi\n", got)
	}

	// 二、非零退出要真的回来：不然「失败」这一格永远测不到。
	sess, err = c.NewSession()
	if err != nil {
		t.Fatalf("开会话失败：%v", err)
	}
	err = sess.Run("false")
	_ = sess.Close()
	var exitErr *ssh.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitStatus() != 1 {
		t.Fatalf("跑 false 应拿回退出码 1，got %v", err)
	}

	// 三、sftp：写一个文件进去，读回来一样，且真落在远端根里。
	sc, err := sftp.NewClient(c)
	if err != nil {
		t.Fatalf("开 sftp 失败：%v", err)
	}
	defer func() { _ = sc.Close() }()

	const body = "假目标机上的一个文件\n"
	remote := ft.path("note.txt")

	w, err := sc.Create(remote)
	if err != nil {
		t.Fatalf("建远端文件失败（%s）：%v", remote, err)
	}
	if _, err := io.WriteString(w, body); err != nil {
		t.Fatalf("写远端文件失败：%v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("关远端文件失败：%v", err)
	}

	fi, err := sc.Stat(remote)
	if err != nil {
		t.Fatalf("取远端文件信息失败：%v", err)
	}
	if fi.Size() != int64(len(body)) {
		t.Fatalf("远端文件大小 want %d，got %d", len(body), fi.Size())
	}

	r, err := sc.Open(remote)
	if err != nil {
		t.Fatalf("读远端文件失败：%v", err)
	}
	got, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		t.Fatalf("读远端内容失败：%v", err)
	}
	if string(got) != body {
		t.Fatalf("远端内容 want %q，got %q", body, got)
	}

	// 落在临时根里，不是别处——证明「远端」就是这台测试机器上的一个临时目录。
	if _, err := os.Stat(filepath.Join(ft.root, "note.txt")); err != nil {
		t.Fatalf("文件没落在临时根里：%v", err)
	}
}
