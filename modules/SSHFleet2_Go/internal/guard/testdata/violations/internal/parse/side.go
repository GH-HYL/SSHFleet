package parse

import _ "fixture/internal/ask"

// 空白 import：只为一个 init 边——类二的第四种绕法（ADR-0048）。
// 解析站不该依赖问句包，这条边写成 `_` 就没人记它。
