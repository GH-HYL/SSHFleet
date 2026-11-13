package parse

// 「算不算二进制」这一条判据在本包里只有一处。
//
// 为什么不住 `criterion`：那一包的准入门槛是「一条判据被两个以上的包用到」（见 `criterion/doc.go`、
// ADR-0051）。这条今天只有解析站在用（脚本、清单、代填三份料都由解析站读），搬过去就是为将来
// 「可能会有别人用」预留抽象。等真有第二个包要判，再搬。

import "bytes"

// isBinaryContent 内容算不算二进制：出现 NUL 字节即算。
//
// 查全文，不看前缀。老版 `common.IsBinaryContent` 只扫前 1KB——那是性能取舍，不是判据。差只在
// 一种料上看得见：前 1KB 干净、后面才出现 NUL 的脚本或清单，老版当文本读，这里按二进制拒
// （本议题工单 04）。
func isBinaryContent(data []byte) bool {
	return bytes.IndexByte(data, 0) >= 0
}
