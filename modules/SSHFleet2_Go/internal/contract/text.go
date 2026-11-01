package contract

// Text 是「一句要打给使用者的话」的载体：解析站产事实、message 拼句、塔尖搬、console 打。
// 字段导出是认下的账：契约包不许有函数，拦的是顺手不是存心——与 15 号那格密封件同一个口径。
type Text struct {
	S string
}
