package contract

// 上传与下载那一件事的几张格子：要传的文件表、被过滤掉的链接、上传根下一层的条目、
// 总大小，以及每台机器传完之后的四个数。
//
// 前四样在「这次干什么」那一格里（解析站采集后放上去），最后一样在「每台一行」那一格里。
// **每台的事实住 `NodeResult`，不进 `Extra`**：`Extra` 是纸上一格，装不下逐台的东西
// （建站 05 规格第二节；审计改过一次，别再挪回去）。

// UploadFile 一个要传的文件：本机绝对路径、相对上传根的那一段（POSIX 斜杠）、字节数。
//
// 相对路径不含上传根那一层（`-u modules -p /tmp/` ⇒ `/tmp/<内容>`），算一次放纸上——
// 执行站照着落位，不再自己推。
type UploadFile struct {
	Path string // 本机路径（读它）
	Rel  string // 相对上传根的路径，已归一成 POSIX 斜杠
	Size int64  // 字节数
}

// UploadEntry 上传根下一层的一个条目：名字，以及是不是目录。
//
// 确认屏那棵树由它摆，**不由文件表反推**——反推会漏掉空子目录与被过滤的链接，
// 而老版那一屏列得出来。
type UploadEntry struct {
	Name  string
	IsDir bool
}

// TransferStat 传输类的四个数（按每台一份）。非传输模式这一格是零值。
//
// 四个数都是事实，不是结论：判定要拿它们算成功规则，报告要印，07 的表格也要。
type TransferStat struct {
	TotalFiles   int   // 这次打算传几个文件
	SuccessFiles int   // 传成了几个
	FailFiles    int   // 失败几个
	TotalBytes   int64 // 实际走过的字节数
}
