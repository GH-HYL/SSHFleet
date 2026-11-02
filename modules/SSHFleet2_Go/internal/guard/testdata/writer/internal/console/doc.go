package console

import "io"

// Writer 假件：这一包是唯一许碰标准输出的地方，样本只判调用点。
func Writer() io.Writer { return nil }
