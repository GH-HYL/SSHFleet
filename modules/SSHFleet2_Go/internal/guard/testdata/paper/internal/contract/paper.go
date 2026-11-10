package contract

// 样本：Node 与真契约同名，IP 在纸面白名单上；Note 是故意加的那一格裸 string，
// 名单上没有它，类四要报出来（05 号单的反向验证样本）。
type Node struct {
	IP   string
	Note string
}

// Mode 是具名类型，不是裸 string，不算越界——ADR-0044 要走的就是这一步。
type Mode string
