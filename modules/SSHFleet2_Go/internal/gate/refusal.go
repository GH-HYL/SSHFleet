package gate

import (
	"sshfleet2/internal/message"
)

// Refusal 是「拒绝」：计划已经成形，结论是不能跑。
//
// **它是接口，不是结构体**，为的是「别处造不出一个拒绝」（ADR-0034 决定三）：接口里那一个
// 不导出的方法，别的包实现不了，于是跨包换不出一个非 nil 的 Refusal。结构体做不到这件事——
// `gate.Refusal{}` 是合法的零值字面量，加了不导出字段也照样编得过（2026-11-03 试过，
// 探针证明过：加了字段之后 `gate.Refusal{}` 仍然编译通过）。塔尖仍认得出它（`!= nil`
// 就是「被拒了」），要话的时候问 `Say()`。
//
// 早先它是带导出字段 `Say` 的结构体：外面拿一句现成的话（任何 `message.Line` 都行）就能
// 拼出一个「拒绝」，「拒绝权只有闸门一处」只靠第五类检查兜着。
//
// 判据见 ADR-0034 决定二、三；谁在哪儿造它，见 ADR-0032 第四条。
type Refusal interface {
	// Say 是那一屏话：含「写到哪条流」那一格，闸门填、塔尖搬、console 打。
	Say() message.Line

	// Trace 是同一句拒绝落进工具日志的形态（见 `message.TraceRefusal`）：空 Line 表示
	// 这一拒不留痕——使用者自己答否、危险框上按「不继续」，都不是失败。
	//
	// 为什么由闸门交、不由塔尖自己拼：塔尖手里只有 `Say()` 那一屏（带红头、带日志路径），
	// 抄进日志就是两层壳；而正文住在闸门这一站，只有它在造拒绝时同时拿得到两样。
	Trace() message.Line

	// refused 不导出：本包之外实现不了这个接口，也就造不出一个「拒绝」。
	refused()
}

// refusal 是唯一实现，只有本包的 newRefusal 造得出。
type refusal struct {
	say   message.Line
	trace message.Line
}

func (r refusal) Say() message.Line   { return r.say }
func (r refusal) Trace() message.Line { return r.trace }

func (r refusal) refused() {}

// newRefusal 是「拒绝」的唯一构造点：给那一屏话，交回一个拒绝。**不带 trace**——
// 于是这一拒只上屏、不落日志，只给「使用者自己喊停」那类用。
//
// 不导出：闸门之外谁都不该说「不让走」。
func newRefusal(line message.Line) Refusal {
	return refusal{say: line}
}

// newRefusalLogged 是「这一拒也要留痕」的构造点：屏话与日志话各给一格。
//
// 参数与规则类拒绝全走它（`refuse`）——它们都是失败，与「起不来」同族，老版一律经
// `fatal` 落一条 `[cli] …`；屏幕上那句话末尾既然报了「日志：<路径>」，日志里就得有。
func newRefusalLogged(say, trace message.Line) Refusal {
	return refusal{say: say, trace: trace}
}
