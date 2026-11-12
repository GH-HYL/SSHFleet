package gate

import . "fixture/internal/ask"

// 点 import 把别包的符号倒进本文件作用域——类二的第四种绕法（ADR-0048）。
// 这条依赖边是真的（闸门依赖了问句包），只是名字写成 `.` 就没人记它。
var _ = Ask
