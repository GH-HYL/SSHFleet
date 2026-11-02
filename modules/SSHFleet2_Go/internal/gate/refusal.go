package gate

import (
	"sshfleet2/internal/message"
)

// Refusal 是「拒绝」：计划已经成形，结论是不能跑。
//
// 类型本身可见——塔尖要认出它才能把话交给终端。拒绝权只有闸门一处：别处能造出一个「拒绝」，
// 就等于第二个地方能说「不让走」。构造函数不导出，只在本包用；别处写 gate.Refusal{…}
// 会被第五类（失败类型的构造点）当场判红。
//
// 判据见 ADR-0034 决定二、三；谁在哪儿造它，见 ADR-0032 第四条。
type Refusal struct {
	// Say 是那一屏话：含「写到哪条流」那一格，闸门填、塔尖搬、console 打。
	Say message.Line
}
