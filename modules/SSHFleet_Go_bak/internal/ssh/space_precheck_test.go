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
	"time"

	"github.com/pkg/sftp"
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
	if avail, ok := freeBytes(t, connected(t), fullDir); !ok || avail >= 200<<20 {
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

// freeBytes 借我们自己的连接另开一个 SFTP 客户端，问某条路径所在分区的可用字节。
// 断言「这块卷真装不下」时才用——挂载掉了的话，用例会在根分区上「通过」，什么也没验到。
func freeBytes(t *testing.T, c *Client, p string) (int64, bool) {
	t.Helper()
	sc, err := sftp.NewClient(c.conn)
	if err != nil {
		t.Fatalf("开 SFTP 客户端失败：%v", err)
	}
	defer func() { _ = sc.Close() }()
	return sftpAvail(sc, p, false)
}

// fullDir 取那块容量有限的卷上的可写目录；没配就跳过。
func fullDir(t *testing.T) string {
	t.Helper()
	dir := strings.TrimSuffix(os.Getenv("SSHFLEET_TEST_FULLDIR"), "/")
	if dir == "" {
		t.Skip("未设置 SSHFLEET_TEST_FULLDIR（容量有限的卷上的可写目录），跳过")
	}
	return dir
}

// 判据本身（工单 02）：直接喂两个数，不问上传流程。
//
// 整条链路「预检过了、写的时候还是满了」只有在**别人同时占这块盘**时才发生，
// 那种竞态没法稳定复现（见 TestUploadFullAfterFailure 的注释）。所以判据这一层
// 直接调，接线靠下面那条并发用例与代码走查。
func TestSpaceShortAfterFailure(t *testing.T) {
	dir := fullDir(t)
	sc, err := sftp.NewClient(connected(t).conn)
	if err != nil {
		t.Fatalf("开 SFTP 客户端失败：%v", err)
	}
	defer func() { _ = sc.Close() }()

	// 这块卷装不下 1GB：应给出目标侧那条、且是「还差多少」的说法。
	msg := spaceShort(sftpProbe(sc), dir, false, 1<<30, 0, false)
	if !strings.Contains(msg, "sshfleet.transfer.target_space_short") {
		t.Fatalf("应给出目标侧空间报文，实为 %q", msg)
	}
	if !strings.Contains(msg, "left to send") || strings.Contains(msg, "no file written") {
		t.Fatalf("失败后再查那条应说「还差多少」、不说没写过文件，实为 %q", msg)
	}

	// 只要 1KB：够了就一个字都不说。
	if msg := spaceShort(sftpProbe(sc), dir, false, 1024, 0, false); msg != "" {
		t.Fatalf("空间够时不该说话，实为 %q", msg)
	}
}

// 传一半才失败（工单 02）：起一个外部进程同时往这块卷里灌数据，模拟「别人在占盘」。
//
// 默认跳过（SSHFLEET_TEST_RACE=1 才跑）：这条依赖时序，放 CI 里会偶发红。
// 预检已经把「总量装不下」挡在门外，所以「写到一半才满」现实中只剩这一种来路。
func TestUploadFullAfterFailure(t *testing.T) {
	if os.Getenv("SSHFLEET_TEST_RACE") != "1" {
		t.Skip("并发占盘的用例默认跳过（设 SSHFLEET_TEST_RACE=1 才跑）")
	}
	dir := fullDir(t)
	setup := connected(t)
	if avail, ok := freeBytes(t, setup, dir); !ok || avail >= 80<<20 {
		t.Fatalf("那块卷可用 %d 字节（探测成功=%v），装得下本次的文件——本用例没有意义", avail, ok)
	}
	for _, n := range remoteList(t, setup, dir) {
		_ = setup.runCommand(fmt.Sprintf("rm -f '%s'", escapeShellArg(path.Join(dir, n))))
	}

	// 外部消费者：等预检过了再往里灌，让可用量在**上传途中**掉下去。
	// 灌太早会被预检直接拦住（那验的是工单 01，不是这里），所以先等一下。
	filler := connected(t)
	go func() {
		time.Sleep(800 * time.Millisecond)
		_ = filler.runCommand(fmt.Sprintf("dd if=/dev/zero of='%s' bs=1M count=45 2>/dev/null",
			escapeShellArg(path.Join(dir, "filler.bin"))))
	}()

	local := t.TempDir()
	files := []LocalFile{localFileSized(t, local, "payload.bin", 40<<20)}
	res := connected(t).UploadFiles(context.Background(), files, nil, dir+"/", false, 0, nil)

	if res.Error == nil {
		t.Fatalf("并发占盘时上传应失败，实际没有报错（success=%d）", res.SuccessFiles)
	}
	if !strings.Contains(*res.Error, "sshfleet.transfer.target_space_short") {
		t.Fatalf("失败原因里没有空间报文：%q", *res.Error)
	}
	if strings.Contains(*res.Error, "no file written") {
		t.Fatalf("这时已经写过并回滚了，不该声明没写过文件：%q", *res.Error)
	}
}

// 失败另有原因时不许改口：空间够，就维持原来的分类（这里是最常见的权限问题）。
func TestUploadFailureNotSpace(t *testing.T) {
	dir := fullDir(t)
	// 卷的根目录属于 root、只读给普通账号：写不进去，但空间是够的。
	roDir := path.Dir(dir)
	setup := connected(t)
	if avail, ok := freeBytes(t, setup, roDir); !ok || avail < 1<<20 {
		t.Fatalf("这块卷可用 %d 字节，本用例需要它「空间够」，跳过不成立", avail)
	}

	local := t.TempDir()
	files := []LocalFile{localFileSized(t, local, "tiny.bin", 1024)}
	res := connected(t).UploadFiles(context.Background(), files, nil, roDir+"/", false, 0, nil)

	if res.Error == nil {
		t.Fatalf("往不可写的目录传应失败，实际没有报错")
	}
	if strings.Contains(*res.Error, "space_short") {
		t.Fatalf("空间够，不该报空间不足：%q", *res.Error)
	}
	if !strings.Contains(*res.Error, "sshfleet.transfer.remote_file_failed") {
		t.Fatalf("应保留原本的原因（写入失败）：%q", *res.Error)
	}
}

// 撞名那个出口不探测：错误详情里不该冒出空间报文。
func TestUploadRemoteExistsNoSpaceProbe(t *testing.T) {
	dir := fullDir(t)
	setup := connected(t)
	existing := path.Join(dir, "dup.bin")
	if err := setup.runCommand(fmt.Sprintf("printf x > '%s'", escapeShellArg(existing))); err != nil {
		t.Fatalf("预置同名文件失败：%v", err)
	}
	t.Cleanup(func() { _ = connected(t).runCommand(fmt.Sprintf("rm -f '%s'", escapeShellArg(existing))) })

	local := t.TempDir()
	files := []LocalFile{localFileSized(t, local, "dup.bin", 1024)}
	res := connected(t).UploadFiles(context.Background(), files, nil, dir+"/", false, 0, nil)

	if res.Error == nil {
		t.Fatalf("撞名应失败，实际没有报错")
	}
	if !strings.Contains(*res.Error, "sshfleet.transfer.remote_exists") {
		t.Fatalf("应保留撞名报文：%q", *res.Error)
	}
	if strings.Contains(*res.Error, "space_short") {
		t.Fatalf("撞名不是空间问题，不该出现空间报文：%q", *res.Error)
	}
}

// 暂存侧判满（工单 01 的 --sudo 那条）：目标盘够、/tmp 装不下单个最大文件。
//
// 要两样环境：测试账号能免密 sudo，且 /tmp 真被限制到小于本次单个文件（8M 那种）。
// 两样都不是默认状态，所以用 SSHFLEET_TEST_SUDO=1 显式打开。
func TestUploadPrecheckTmpSideShort(t *testing.T) {
	if os.Getenv("SSHFLEET_TEST_SUDO") != "1" {
		t.Skip("需要免密 sudo 且 /tmp 被限小（设 SSHFLEET_TEST_SUDO=1 才跑）")
	}
	setup := connected(t)
	if avail, ok := freeBytes(t, setup, "/tmp"); !ok || avail >= 40<<20 {
		t.Fatalf("/tmp 可用 %d 字节（探测成功=%v），装得下 40MB——本用例没有意义，先把 /tmp 限小", avail, ok)
	}
	dir := fmt.Sprintf("/var/tmp/sshfleet-tmpfull-%d", os.Getpid())
	mkRemoteDir(t, setup, dir)
	t.Cleanup(func() { _ = connected(t).runCommand(fmt.Sprintf("rm -rf '%s'", escapeShellArg(dir))) })

	local := t.TempDir()
	files := []LocalFile{localFileSized(t, local, "payload.bin", 40<<20)}
	res := connected(t).UploadFiles(context.Background(), files, nil, dir+"/", true, 0, nil)

	if res.Error == nil {
		t.Fatalf("应判满并失败，实际没有报错（success=%d）", res.SuccessFiles)
	}
	if !strings.Contains(*res.Error, "sshfleet.transfer.tmp_space_short") {
		t.Fatalf("应报暂存侧空间不足：%q", *res.Error)
	}
	if !strings.Contains(*res.Error, "/tmp") {
		t.Fatalf("报文里要点明是 /tmp：%q", *res.Error)
	}
	if left := remoteList(t, connected(t), dir); len(left) != 0 {
		t.Fatalf("判满时目标目录不该有东西，实为 %v", left)
	}
}

// 刚好装得下（工单 01 的边界那条）：按这块卷**实测**的可用量造文件，少留 4MB。
//
// 判据是硬比，边界两侧都得有证据：判满那边由 TestUploadPrecheckSpaceShort 给（200MB），
// 这边给「贴着边也能传成」。
func TestUploadPrecheckSpaceJustEnough(t *testing.T) {
	dir := fullDir(t)
	setup := connected(t)
	avail, ok := freeBytes(t, setup, dir)
	if !ok {
		t.Fatalf("这块卷探不出可用量，本用例没有意义")
	}
	size := avail - 4<<20
	if size <= 0 {
		t.Fatalf("这块卷可用 %d 字节，太小，造不出「刚好装得下」的文件", avail)
	}
	for _, n := range remoteList(t, setup, dir) {
		_ = setup.runCommand(fmt.Sprintf("rm -f '%s'", escapeShellArg(path.Join(dir, n))))
	}
	local := t.TempDir()
	files := []LocalFile{localFileSized(t, local, "tight.bin", size)}

	res := connected(t).UploadFiles(context.Background(), files, nil, dir+"/", false, 0, nil)
	t.Cleanup(func() {
		_ = connected(t).runCommand(fmt.Sprintf("rm -f '%s'", escapeShellArg(path.Join(dir, "tight.bin"))))
	})

	if res.Error != nil {
		t.Fatalf("贴着边也该传得成（可用 %d，需要 %d），实为报错：%q", avail, size, *res.Error)
	}
	if res.SuccessFiles != 1 {
		t.Fatalf("应传成 1 个，实为 %d", res.SuccessFiles)
	}
}

// 目标目录是 700 的 root 目录时，探测仍给得出判断（工单 01 那条）。
//
// 要 root 才能造出这种目录，所以跟暂存侧那条一样用 SSHFLEET_TEST_SUDO 打开。
func TestUploadPrecheckRootOnlyDir(t *testing.T) {
	if os.Getenv("SSHFLEET_TEST_SUDO") != "1" {
		t.Skip("需要免密 sudo 才能造 700 的 root 目录（设 SSHFLEET_TEST_SUDO=1 才跑）")
	}
	setup := connected(t)
	dir := fmt.Sprintf("/var/tmp/sshfleet-rootonly-%d", os.Getpid())
	if err := setup.runCommand(fmt.Sprintf("sudo rm -rf '%s' && sudo mkdir -p '%s' && sudo chmod 700 '%s'",
		escapeShellArg(dir), escapeShellArg(dir), escapeShellArg(dir))); err != nil {
		t.Fatalf("造 700 目录失败：%v", err)
	}
	t.Cleanup(func() { _ = connected(t).runCommand(fmt.Sprintf("sudo rm -rf '%s'", escapeShellArg(dir))) })

	// 探得到，且给的是一个真数（不是 0）——「不需要写权限」这件事的证据。
	avail, ok := freeBytes(t, setup, dir)
	if !ok || avail <= 0 {
		t.Fatalf("700 的 root 目录应能探出可用量，实为 %d（成功=%v）", avail, ok)
	}

	// 同一目录非 sudo 真去写，是权限问题、不是空间问题：报错里不该出现空间报文。
	local := t.TempDir()
	files := []LocalFile{localFileSized(t, local, "deny.bin", 1024)}
	res := connected(t).UploadFiles(context.Background(), files, nil, dir+"/", false, 0, nil)
	if res.Error == nil {
		t.Fatalf("往 700 的 root 目录写应失败，实际没有报错")
	}
	if strings.Contains(*res.Error, "space_short") {
		t.Fatalf("空间够，不该报空间不足：%q", *res.Error)
	}
}
