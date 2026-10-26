//go:build integration

package ssh

// 上传前判满的真机验收（工单 01）。
//
// 这条用例只在「有容量上限的卷」上才有意义，所以那卷上的可写目录从
// SSHFLEET_TEST_FULLDIR 取；没设就跳过——别拿普通磁盘凑，那里永远判不满。
//
// 造卷配方（root，目标机上）：
//   dd if=/dev/zero of=/var/tmp/small64.img bs=1M count=64
//   mkfs.ext4 -q -F /var/tmp/small64.img
//   losetup /dev/loop0 /var/tmp/small64.img && mount /dev/loop0 /mnt/small64
//   mkdir -p /mnt/small64/w && chmod 777 /mnt/small64/w
// 然后把 SSHFLEET_TEST_FULLDIR 指到 /mnt/small64/w。

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"
)

// localFileSized 造一个指定大小的本地文件（稀疏写，不会真的占满本机盘）。
func localFileSized(t *testing.T, dir, rel string, size int64) LocalFile {
	t.Helper()
	full := path.Join(dir, rel)
	if err := os.MkdirAll(path.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(full)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := f.Truncate(size); err != nil {
		t.Fatalf("造 %d 字节的本地文件失败：%v", size, err)
	}
	return LocalFile{Path: full, Rel: rel, Size: size}
}

func TestUploadPrecheckSpaceShort(t *testing.T) {
	fullDir := strings.TrimSuffix(os.Getenv("SSHFLEET_TEST_FULLDIR"), "/")
	if fullDir == "" {
		t.Skip("未设置 SSHFLEET_TEST_FULLDIR（容量有限的卷上的可写目录），跳过")
	}
	local := t.TempDir()
	// 200MB 远大于那块卷的容量：判据成立时，工具不该写任何字节。
	big := localFileSized(t, local, "big.bin", 200<<20)

	// 先确认这块卷真的装不下 200MB：挂载掉的坑踩过一次——那时 /mnt/small64 退回成根分区，
	// 200MB 写得进去，用例「通过」却什么也没验到。
	if avail, ok := sftpAvail(connected(t), fullDir, false); !ok || avail >= 200<<20 {
		t.Fatalf("那块卷可用 %d 字节（探测成功=%v），装得下 200MB——它多半不是那块小卷，本用例没有意义", avail, ok)
	}

	before := remoteList(t, connected(t), fullDir)

	res := connected(t).UploadFiles(context.Background(), []LocalFile{big}, nil, fullDir+"/", false, 0, nil)

	if res.Error == nil {
		t.Fatalf("应判满并失败，实际没有报错（success=%d failed=%d）", res.SuccessFiles, res.FailedFiles)
	}
	if !strings.Contains(*res.Error, "sshfleet.transfer.target_space_short") {
		t.Fatalf("报错原文里没有空间报文：%q", *res.Error)
	}
	if !strings.Contains(*res.Error, "no file written") {
		t.Fatalf("预检那条应声明没写过文件：%q", *res.Error)
	}
	if res.SuccessFiles != 0 || res.FailedFiles != 0 {
		t.Fatalf("一个文件都没动，计数应都是 0，实为 success=%d failed=%d", res.SuccessFiles, res.FailedFiles)
	}

	after := remoteList(t, connected(t), fullDir)
	if len(after) != len(before) {
		t.Fatalf("目标机上多了东西：before=%v after=%v", before, after)
	}
	// 分类落到哪一格由判据表决定，语料里已有同一串报文的两条断言（verdict 包），
	// 这里不重复判——本包引 verdict 会成环。
}

func TestUploadPrecheckSpaceEnough(t *testing.T) {
	setup := connected(t)
	dir := fmt.Sprintf("/tmp/sshfleet-space-ok-%d", os.Getpid())
	mkRemoteDir(t, setup, dir)
	local := t.TempDir()
	files := []LocalFile{localFileSized(t, local, "ok.bin", 4096)}

	res := connected(t).UploadFiles(context.Background(), files, nil, dir+"/", false, 0, nil)

	if res.Error != nil {
		t.Fatalf("空间够时不该有报错：%q", *res.Error)
	}
	if res.SuccessFiles != 1 {
		t.Fatalf("应传成 1 个，实为 %d", res.SuccessFiles)
	}
}
