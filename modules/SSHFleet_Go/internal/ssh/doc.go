// Package ssh 承载第 8 步的 SSH/SFTP 连接与单节点执行（M3 落地）。
//
// 主机密钥校验刻意跳过（ADR-0001，数千台规模）；进度读写器（包裹 SFTP
// 读写的 io.Writer/io.Reader）住这里；worker pool / 连接 / 认证回退 /
// 退出码提取直接沿用旧 Go 实现。
package ssh
