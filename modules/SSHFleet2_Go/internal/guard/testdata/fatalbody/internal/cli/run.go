package cli

import "fixture/internal/message"

// 违规样本两种形状各一次：把 err.Error() 直接当正文、当场拼一句塞进那一屏。
// 还有一种不违规：正文由 message 的类别函数给。

// SaySysError 犯规：系统原文直接上屏。
func SaySysError(err error) {
	message.Fatal("cli", err.Error(), "")
}

// SayJoined 犯规：当场拼一句。
func SayJoined(path string) {
	message.Fatal("cli", "读不到"+path, "")
}

// TraceJoined 犯规：日志那一半同样是拼的。
func TraceJoined(err error) {
	message.TraceRefusal(err.Error())
}

// SayRefuse 不犯规：正文向 message 要。
func SayRefuse() {
	message.Fatal("cli", message.RefuseUnknownOption("--nope"), "")
}
