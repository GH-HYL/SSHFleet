package message

// Line 是一句已经排好版、要交给终端的话：字在 s 里，写到哪条流在 toErr 里。
//
// **字段一律不导出，这是全工程唯一能「造话」的地方。** 别的包既拼不出 Line 的值，也就
// 没有东西可以塞给 console——「自己取个数拼一句直接打出去」在编译期就过不去。ADR-0032
// 第四条要的就是这道硬闸：拦得住顺手，比检查硬，也比审计硬。
//
// 读的两面（要打什么、打到哪条流）各给一个方法，因为 console 在别的包里，字段够不着。
type Line struct {
	s     string
	toErr bool
}

// Text 是这句话的字，含色码。只有 console 该调它——别处拿到它也只是个字符串，打不出去。
func (l Line) Text() string { return l.s }

// Stderr 报这句话写标准错误（true）还是标准输出（false）。
//
// 老版里同一类失败走哪条流是固定的：错误套 [ERROR] 走 stderr，危险框与「操作已取消」
// 走 stdout。重定向时看得见，所以流跟着句子走，不由塔尖猜——它一猜就是在做判断。
func (l Line) Stderr() bool { return l.toErr }
