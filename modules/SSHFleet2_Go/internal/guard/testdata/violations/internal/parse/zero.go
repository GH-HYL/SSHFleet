package parse

import "fixture/internal/gate"

// 一个零值变量就造出了「拒绝」——类五的第三种形状（ADR-0048）。
// 复合字面量能抓、零值抓不到的话，包外伪造拒绝的成本只是换个写法。
var pending gate.Refusal
