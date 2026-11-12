package contract

// Leak 是契约包里的函数——类四的坏样本。
func Leak() {}

// LeakValue 把函数装进包级变量——类四的第二种形状（ADR-0048）：写成 var 也是行为。
var LeakValue = func() {}

// LeakRef 取的是本包那个函数的名字——类四的第三种形状，同上。
var LeakRef = Leak
