package ssh

import (
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"

	"github.com/pkg/sftp"
)

// 上传前的空间判据（ADR-0026、工单 01）。
//
// 这一摊只做两件事：把 StatVFS 的块数换算成「谁能用多少字节」，以及拿可用量与需要量
// 硬比出结论并拼出那句报文。探测本身（要不要探、探哪条路径、失败怎么降级）留在 sftp.go
// 的接线里——纯函数好测，接线靠真机用例。

// spaceSide 判的是哪一侧。两侧的报文名与路径都不同，判据本身完全一样。
type spaceSide int

const (
	sideTarget spaceSide = iota // 目标目录所在分区
	sideTmp                     // --sudo 时的暂存目录（/tmp）所在分区
)

// judgeSpace 硬比可用量与需要量，不够就给出该发哪条报文。
//
// beforeWrite 区分两种场景：预检（一个文件都没写，报文里要声明这一点）与
// 失败后再查（已经写过并回滚，只说还差多少）。判据两处同源，只有细节不同。
func judgeSpace(side spaceSide, path string, avail, need int64, beforeWrite bool) (string, bool) {
	if avail >= need {
		return "", false
	}
	name := transferTargetSpaceShort
	if side == sideTmp {
		name = transferTmpSpaceShort
	}
	return Message(EnvTransfer, name, spaceShortDetail(path, avail, need, beforeWrite)), true
}

// spaceShortDetail 报文正文：路径 + 两个数（ADR-0021 决定第 8 条：执行侧只陈述事实）。
func spaceShortDetail(path string, avail, need int64, beforeWrite bool) string {
	if beforeWrite {
		return fmt.Sprintf("%s: %s available, %s needed; no file written",
			path, humanBytes(avail), humanBytes(need))
	}
	return fmt.Sprintf("%s: %s available, %s left to send",
		path, humanBytes(avail), humanBytes(need))
}

// availBytes 把 StatVFS 的块数换算成字节。
//
// 取哪个字段由「谁在写」决定：以 root 写时 root 能用保留块（Bfree），普通账号只能用
// Bavail。两者的差在满盘时最明显——实测一块 52MB 的卷写满后，Bfree 约 4.5MB、
// Bavail 约 16KB（ADR-0026 实测记录）。
//
// 块大小按 Frsize 算（Bavail/Bfree 都以它为单位）；有的实现只填 Bsize，那时退回 Bsize。
func availBytes(st *sftp.StatVFS, asRoot bool) int64 {
	unit := st.Frsize
	if unit == 0 {
		unit = st.Bsize
	}
	blocks := st.Bavail
	if asRoot {
		blocks = st.Bfree
	}
	return int64(blocks) * int64(unit)
}

// sftpAvail 问一次某条路径所在分区还能用多少字节；探不出来就返回 false。
//
// 探测失败一律当「没有这一侧的数」处理，不区分原因：服务端不支持这个扩展、路径在中途
// 不可达、连接刚好断了，对调用方是同一件事——少一个判断依据，而不是多一条失败。
func sftpAvail(client *sftp.Client, p string, asRoot bool) (int64, bool) {
	st, err := client.StatVFS(p)
	if err != nil {
		return 0, false
	}
	return availBytes(st, asRoot), true
}

// probeFunc 问一条路径所在分区的可用字节；探不出来返回 false。
//
// 抽成一个函数值，是为了让「某一侧探不到」「两侧都探不到」这两条降级分支能被测到——
// 真机上没法稳定造出「探测失败」，而这恰恰是最不能出错的一条：探不到不许挡住上传。
type probeFunc func(path string, asRoot bool) (int64, bool)

// spaceShort 两侧各探一次：哪一处不够就说那一处，够就一个字都不说。
//
// 预检与失败后再查共用它，区别只在需要量与措辞（beforeWrite）：前者是一个文件都没写，
// 后者已经写过并回滚，只说还差多少。
func spaceShort(probe probeFunc, remotePath string, sudo bool, targetNeed, tmpNeed int64, beforeWrite bool) string {
	if avail, ok := probe(remotePath, sudo); ok {
		if msg, short := judgeSpace(sideTarget, remotePath, avail, targetNeed, beforeWrite); short {
			return msg
		}
	}
	if !sudo {
		return ""
	}
	// 暂存侧只在 sudo 路径上存在：非 sudo 直接写目标，没有暂存这一步。
	probePath := tmpProbePath()
	if avail, ok := probe(probePath, false); ok {
		if msg, short := judgeSpace(sideTmp, probePath, avail, tmpNeed, beforeWrite); short {
			return msg
		}
	}
	return ""
}

// sftpProbe 把这条连接的探测包成 probeFunc。
func sftpProbe(client *sftp.Client) probeFunc {
	return func(p string, asRoot bool) (int64, bool) { return sftpAvail(client, p, asRoot) }
}

// tmpProbePath 暂存目录所在的分区路径。
//
// 探的是暂存目录的父目录而不是它自己：tmpRoot 下每个文件才建一个随机子目录，
// 预检这一刻它还不存在。从 tmpRoot 推出来，别另写一个 "/tmp" 字面量——将来改暂存根，
// 这里跟着走。
func tmpProbePath() string {
	return path.Dir(strings.TrimSuffix(tmpRoot, "/"))
}

// maxFileSize 本次上传里最大的那个文件。
//
// sudo 路径的暂存侧只需要容得下这一个：传一个就 mv 一个，暂存区任何时刻只有一个文件。
func maxFileSize(files []LocalFile) int64 {
	var max int64
	for _, f := range files {
		if f.Size > max {
			max = f.Size
		}
	}
	return max
}

// humanBytes 报文里的人读大小：按量级选单位，保留两位小数，多余的零不写。
func humanBytes(n int64) string {
	const (
		kb = int64(1024)
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return trimFloat(float64(n)/float64(gb)) + " GB"
	case n >= mb:
		return trimFloat(float64(n)/float64(mb)) + " MB"
	case n >= kb:
		return trimFloat(float64(n)/float64(kb)) + " KB"
	default:
		return strconv.FormatInt(n, 10) + " B"
	}
}

// trimFloat 四舍五入到两位小数，并用最短表示输出（"3" 而不是 "3.00"）。
func trimFloat(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}
