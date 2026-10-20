//go:build integration

package ssh

// 上传失败即回滚（ADR-0022）的真机验收。
//
// 这里每一条用例都在真机上跑，断言的是**目标机上到底留下了什么**——回滚本质上是删文件，
// 删错比不删更糟，所以判据不放在「返回了什么」，而放在「远端的文件状态」。
//
// 场景与判据（对应工单 01 的验收）：
//  1. 全成功：不触发回滚，文件齐全。
//  2. 第一个文件就失败：writtenPaths 为空，不发回滚报文，目标机无新增。
//  3. 中途失败：已写成功的被删干净，报文写 rolled_back。
//  4. 目标目录里原本就有同名文件：上传失败，**那个原文件必须还在、内容不变**。
//  5. sudo 路径：同样回滚，root 属主的文件也能删。
//  6. 预检阶段失败（目录不存在）：根本不进循环，不产生任何删除。

import (
	"context"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"
)

// intClient 建一个客户端；环境不全时 skip。
func intClient(t *testing.T) *Client {
	t.Helper()
	host := os.Getenv("SSHFLEET_TEST_HOST")
	portStr := os.Getenv("SSHFLEET_TEST_PORT")
	user := os.Getenv("SSHFLEET_TEST_USER")
	pass := os.Getenv("SSHFLEET_TEST_PASSWORD")
	if host == "" || portStr == "" || user == "" || pass == "" {
		t.Skip("集成测试的连接信息不全（SSHFLEET_TEST_HOST/PORT/USER/PASSWORD），跳过")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("SSHFLEET_TEST_PORT 不是合法端口：%q", portStr)
	}
	return NewClient(&Config{IP: host, Port: port, User: user, Password: pass, ConnectTimeout: 10 * time.Second})
}

// connected 建一个**已连上**的客户端（测试辅助用：准备远端目录、事后断言远端状态）。
//
// 每个用例要单独开一个：UploadFiles 收尾时会 Close 掉自己那条连接，
// 拿同一条连接去断言会撞上已关闭的连接。
func connected(t *testing.T) *Client {
	t.Helper()
	c := intClient(t)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("连不上测试机：%v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// remoteList 列出远端目录下的文件名（用一条命令读回，避免为断言再引一套 SFTP 客户端）。
func remoteList(t *testing.T, c *Client, dir string) []string {
	t.Helper()
	out, err := c.runCommandCapture(fmt.Sprintf("ls -1A '%s' 2>/dev/null", escapeShellArg(dir)))
	if err != nil {
		t.Fatalf("列目录 %s 失败：%v", dir, err)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names
}

// remoteRead 读回远端文件内容（不存在时返回空串 + false）。
func remoteRead(t *testing.T, c *Client, p string) (string, bool) {
	t.Helper()
	out, err := c.runCommandCapture(fmt.Sprintf("cat '%s' 2>/dev/null", escapeShellArg(p)))
	if err != nil {
		return "", false
	}
	return out, true
}

// localFile 造一个本地待传文件。
func localFile(t *testing.T, dir, rel, content string) LocalFile {
	t.Helper()
	full := path.Join(dir, rel)
	if err := os.MkdirAll(path.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return LocalFile{Path: full, Rel: rel, Size: int64(len(content))}
}

// mkRemoteDir 在远端建一个干净目录（已存在则先清空）。
func mkRemoteDir(t *testing.T, c *Client, dir string) {
	t.Helper()
	if err := c.runCommand(fmt.Sprintf("rm -rf '%s' && mkdir -p '%s' && chmod 777 '%s'",
		escapeShellArg(dir), escapeShellArg(dir), escapeShellArg(dir))); err != nil {
		t.Fatalf("建远端目录 %s 失败：%v", dir, err)
	}
}

func TestUploadRollbackScenarios(t *testing.T) {
	setup := connected(t)
	base := fmt.Sprintf("/tmp/sshfleet-rb-%d", os.Getpid())
	local := t.TempDir()

	t.Run("全成功时不回滚", func(t *testing.T) {
		dir := base + "-ok"
		mkRemoteDir(t, setup, dir)
		files := []LocalFile{
			localFile(t, local, "ok/a.txt", "a"),
			localFile(t, local, "ok/b.txt", "b"),
		}
		res := connected(t).UploadFiles(context.Background(), files, nil, dir+"/", false, 0, nil)
		if res.FailedFiles != 0 || res.SuccessFiles != 2 {
			t.Fatalf("应全部成功，实为 success=%d failed=%d err=%v", res.SuccessFiles, res.FailedFiles, deref(res.Error))
		}
		if got := remoteList(t, connected(t), dir+"/ok"); len(got) != 2 {
			t.Fatalf("全成功时文件应都在，实际 %v", got)
		}
	})

	t.Run("第一个文件就撞名：不写任何东西、不发回滚报文", func(t *testing.T) {
		dir := base + "-firstfail"
		mkRemoteDir(t, setup, dir)
		// 远端先占住 a.txt：本次第 1 个文件就撞名，一个都没写进去。
		// 注意层级：多文件上传会保留相对结构，ff/a.txt 落到 dir/ff/a.txt。
		if err := setup.runCommand(fmt.Sprintf("mkdir -p '%s' && echo original > '%s'",
			escapeShellArg(dir+"/ff"), escapeShellArg(dir+"/ff/a.txt"))); err != nil {
			t.Fatal(err)
		}
		files := []LocalFile{
			localFile(t, local, "ff/a.txt", "a"),
			localFile(t, local, "ff/b.txt", "b"),
		}
		res := connected(t).UploadFiles(context.Background(), files, nil, dir+"/", false, 0, nil)
		if res.FailedFiles == 0 {
			t.Fatal("应失败")
		}
		got := deref(res.Error)
		if strings.Contains(got, "rolled_back") || strings.Contains(got, "rollback_partial") {
			t.Fatalf("一个都没写成功时不该发回滚报文：%q", got)
		}
		// 目标机上只该有原来那个 a.txt，b.txt 根本没轮到
		names := remoteList(t, connected(t), dir+"/ff")
		if len(names) != 1 || names[0] != "a.txt" {
			t.Fatalf("目标机不该有新增文件，实际 %v", names)
		}
		if content, _ := remoteRead(t, connected(t), dir+"/ff/a.txt"); !strings.Contains(content, "original") {
			t.Fatalf("原文件被改动了：%q", content)
		}
	})

	t.Run("第二个文件撞名：回滚掉先传成的那个，原文件不动", func(t *testing.T) {
		dir := base + "-secondfail"
		mkRemoteDir(t, setup, dir)
		if err := setup.runCommand(fmt.Sprintf("mkdir -p '%s' && echo original > '%s'",
			escapeShellArg(dir+"/ff"), escapeShellArg(dir+"/ff/b.txt"))); err != nil {
			t.Fatal(err)
		}
		files := []LocalFile{
			localFile(t, local, "ff/a.txt", "a"),
			localFile(t, local, "ff/b.txt", "b"),
		}
		res := connected(t).UploadFiles(context.Background(), files, nil, dir+"/", false, 0, nil)
		got := deref(res.Error)
		if !strings.Contains(got, "sshfleet.transfer.rolled_back: 1 of 1 written files removed") {
			t.Fatalf("应发 rolled_back 报文（a.txt 被回滚），实际：%q", got)
		}
		names := remoteList(t, connected(t), dir+"/ff")
		if len(names) != 1 || names[0] != "b.txt" {
			t.Fatalf("a.txt 应被回滚删掉、只剩原有的 b.txt，实际 %v", names)
		}
		if content, _ := remoteRead(t, connected(t), dir+"/ff/b.txt"); !strings.Contains(content, "original") {
			t.Fatalf("原文件被改动了：%q", content)
		}
	})

	t.Run("中途失败时清掉已写成功的", func(t *testing.T) {
		dir := base + "-mid"
		mkRemoteDir(t, setup, dir)
		if err := setup.runCommand(fmt.Sprintf("mkdir -p '%s' && echo original > '%s'",
			escapeShellArg(dir+"/mid"), escapeShellArg(dir+"/mid/c.txt"))); err != nil {
			t.Fatal(err)
		}
		files := []LocalFile{
			localFile(t, local, "mid/a.txt", "a"),
			localFile(t, local, "mid/b.txt", "b"),
			localFile(t, local, "mid/c.txt", "c"),
		}
		res := connected(t).UploadFiles(context.Background(), files, nil, dir+"/", false, 0, nil)
		if res.FailedFiles == 0 || res.SuccessFiles != 2 {
			t.Fatalf("应传成 2 个、失败 1 个，实为 success=%d failed=%d", res.SuccessFiles, res.FailedFiles)
		}
		got := deref(res.Error)
		if !strings.Contains(got, "sshfleet.transfer.rolled_back: 2 of 2 written files removed") {
			t.Fatalf("应发 rolled_back 报文，实际：%q", got)
		}
		names := remoteList(t, connected(t), dir+"/mid")
		if len(names) != 1 || names[0] != "c.txt" {
			t.Fatalf("a.txt/b.txt 应被回滚删掉、只剩原有的 c.txt，实际 %v", names)
		}
		if content, _ := remoteRead(t, connected(t), dir+"/mid/c.txt"); !strings.Contains(content, "original") {
			t.Fatalf("原文件被改动了：%q", content)
		}
	})

	t.Run("预检失败时不进循环", func(t *testing.T) {
		files := []LocalFile{localFile(t, local, "pre/a.txt", "a")}
		// 目标目录不存在
		res := connected(t).UploadFiles(context.Background(), files, nil, "/tmp/sshfleet-no-such-dir-xyz/", false, 0, nil)
		if res.Error == nil || !strings.HasPrefix(deref(res.Error), "sshfleet.transfer.remote_missing") {
			t.Fatalf("应停在远端目录预检，实际：%q", deref(res.Error))
		}
		if strings.Contains(deref(res.Error), "rolled_back") {
			t.Fatalf("预检失败时不该发回滚报文：%q", deref(res.Error))
		}
	})

	t.Run("sudo 路径同样回滚", func(t *testing.T) {
		dir := base + "-sudo"
		// 目标目录只允许 root 写：非 sudo 写不进，sudo 才写得进。
		// 用 sudo 建（测试账号有 NOPASSWD），否则普通用户建不出 root 属主的目录。
		if err := setup.runCommand(fmt.Sprintf("sudo rm -rf '%s' && sudo mkdir -p '%s' && sudo chmod 777 '%s'",
			escapeShellArg(dir), escapeShellArg(dir), escapeShellArg(dir))); err != nil {
			t.Fatal(err)
		}
		// sd 子目录留给 root 写：c.txt 是本次之前的既有文件，a/b 传成、c 撞名
		if err := setup.runCommand(fmt.Sprintf("sudo mkdir -p '%s' && echo original | sudo tee '%s' >/dev/null && sudo chown root:root '%s'",
			escapeShellArg(dir+"/sd"), escapeShellArg(dir+"/sd/c.txt"), escapeShellArg(dir+"/sd/c.txt"))); err != nil {
			t.Fatal(err)
		}
		files := []LocalFile{
			localFile(t, local, "sd/a.txt", "a"),
			localFile(t, local, "sd/b.txt", "b"),
			localFile(t, local, "sd/c.txt", "c"),
		}
		res := connected(t).UploadFiles(context.Background(), files, nil, dir+"/", true, 0, nil)
		got := deref(res.Error)
		if !strings.Contains(got, "rolled_back: 2 of 2 written files removed") {
			t.Fatalf("sudo 路径应回滚掉 2 个，实际：%q", got)
		}
		names := remoteList(t, connected(t), dir+"/sd")
		if len(names) != 1 || names[0] != "c.txt" {
			t.Fatalf("sudo 路径下 a/b 应被删、只剩原有 c.txt，实际 %v", names)
		}
	})

	t.Run("按 Ctrl+C 取消时同样清掉已传的", func(t *testing.T) {
		// ADR-0022 第 1 条没给「取消」开例外：同样是「传一半」，留在目标机上就要被清掉。
		//
		// 造法：第一个文件用进度回调把它传成了，回调里立刻取消 context——
		// 循环头那一下就是真实的 Ctrl+C 时刻，此时 writtenPaths 里已经有一个。
		dir := base + "-cancel"
		mkRemoteDir(t, setup, dir)
		big := strings.Repeat("x", 1<<20) // 1MB：确保进度回调被调到
		files := []LocalFile{
			localFile(t, local, "cn/a.txt", big),
			localFile(t, local, "cn/b.txt", "b"),
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		res := connected(t).UploadFiles(ctx, files, nil, dir+"/", false, 0, func(p Progress) {
			if p.SuccessFiles >= 1 {
				cancel()
			}
		})
		if !res.Canceled {
			t.Fatalf("应记成被取消，实际 %+v", res)
		}
		// 关键断言：已经传成的那个必须被清掉，目标机上是空的
		names := remoteList(t, connected(t), dir+"/cn")
		if len(names) != 0 {
			t.Fatalf("取消后目标机不该留文件，实际 %v", names)
		}
		if !strings.Contains(deref(res.Error), "sshfleet.abort.canceled") {
			t.Fatalf("应带上「被中断」那句，实际：%q", deref(res.Error))
		}
	})

	t.Run("回滚本身失败时照实报告剩下哪些", func(t *testing.T) {
		dir := base + "-rollbackfail"
		mkRemoteDir(t, setup, dir)
		files := []LocalFile{
			localFile(t, local, "rf/a.txt", "a"),
			localFile(t, local, "rf/b.txt", "b"),
		}
		c := connected(t)
		// 让远端先占住 rf/b.txt：a.txt 会先传成功，到 b.txt 才失败。
		// 回滚要删的 a.txt 在可写目录里，所以这条用例验证的是「回滚成功能不能如实说清」；
		// 真正删不掉的情形由判据侧的 rollback_partial 覆盖（见 RollbackMessage 单测）。
		if err := setup.runCommand(fmt.Sprintf("mkdir -p '%s/rf' && echo original > '%s'", escapeShellArg(dir+"/rf"), escapeShellArg(dir+"/rf/b.txt"))); err != nil {
			t.Fatal(err)
		}
		res := c.UploadFiles(context.Background(), files, nil, dir+"/", false, 0, nil)
		// 这条用例的关键不是分类，而是「失败时不静默」：报文要么说清删了几个，要么列出剩下的。
		got := deref(res.Error)
		if !strings.Contains(got, "sshfleet.transfer.rolled_back") && !strings.Contains(got, "sshfleet.transfer.rollback_partial") {
			t.Fatalf("回滚无论成没成都要说一句，实际：%q", got)
		}
	})

	if err := setup.runCommand(fmt.Sprintf("rm -rf '%s-*'", escapeShellArg(base))); err != nil {
		t.Logf("清理测试目录失败（不影响结论）：%v", err)
	}
}
