// Package exitcode 是横切叶子包：把「种类」换成退出码。
//
// 它不提供 Exit，也不认具体错误类型——「这是拒绝、那是起不来」由塔尖认，
// 认完把种类交过来查表，退出码的规则因此仍然只有一处。
//
// 判据见 ADR-0032 第四条。
package exitcode

// Kind 是这次怎么收场。塔尖认完结局，把种类交过来。
type Kind int

const (
	// Success 正常收场：帮助打了、活干完了。
	Success Kind = iota
	// CannotStart 起不来：前提缺了，连计划都生不出来。
	CannotStart
	// Refused 拒绝：计划成形了，结论是不能跑。
	Refused
	// Interrupted 被中断：启动段按了 Ctrl+C。
	Interrupted
)

// Code 把种类换成退出码。
//
// 与老版逐项一致：老版只有 0／1／130 三个值，「拒绝」与「起不来」同码是既有行为。
func Code(k Kind) int {
	switch k {
	case Success:
		return 0
	case CannotStart, Refused:
		return 1
	case Interrupted:
		return 130
	}
	return 1
}
