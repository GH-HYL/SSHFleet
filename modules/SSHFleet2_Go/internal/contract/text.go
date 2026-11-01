package contract

// Text 是「一句要打给使用者的话」的载体：站产事实、message 拼句、塔尖搬、console 打。
//
// 字段导出是认下的账：契约包不许有函数，拦的是顺手不是存心——与 15 号那格密封件同一个口径。
type Text struct {
	S string

	// ToErr 为真时写到标准错误，否则写到标准输出。
	//
	// 老版里同一类失败走不同的流：错误套 [ERROR] 走 stderr，「发现禁止命令」那张框与
	// 「操作已取消」走 stdout。重定向时这个差别看得见，所以流必须跟着句子走，
	// 不能由塔尖去猜——它一猜就是在做判断。
	ToErr bool
}
