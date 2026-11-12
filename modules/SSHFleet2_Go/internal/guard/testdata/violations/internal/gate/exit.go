package gate

import "os"

// 把退出存成函数值，等别处再调——类三的第三种形状（ADR-0048）。
// 只认「当场调用」的话，加这一行赋值就绕过去了。
var exitNow = os.Exit
