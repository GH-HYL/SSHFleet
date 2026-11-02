package contract

import "time"

// 第二张纸：执行交结算（口径：刚才发生了什么）。
//
// 它不再回答「这次要干什么」，只回答每台机器跑成了什么样。**「成败」不在这张纸上**：
// 由结算站把分类对回本模式的成功分类才算得出来（ADR-0035 决定四）。
//
// 判据见 ADR-0035 决定一、上一张图 15 号工单第二节。

// NodeResult 是「每台一行」的那些事实。
//
// 顺序照 15 号工单那张表，一格不多一格不少。执行站只写事实，一个结论词都不写；
// 分类是它按判据表认出来的名字，算不算成功由结算站解释。
type NodeResult struct {
	IP string

	// Connected 是连接与认证都过了没有。没过的那一台，后面几格基本是零值。
	Connected bool

	// ConnectCost 与 Cost 是两段耗时：连上用了多久、跑那件事用了多久。
	ConnectCost time.Duration
	Cost        time.Duration

	// Category 是这一台归到哪一类（判据表给的分类名）；没命中任何一类时为空。
	Category string

	// ExitCode 是**目的那条命令**的退出码。命令与脚本模式写它；没有目的命令的模式恒为 nil。
	ExitCode *int

	// TimedOut 与 Cancelled 是两个事实标志，不是结论：到点收场、或被外部取消。
	// 压根没开跑的那一台不造行，只在总台数里以缺额出现。
	TimedOut  bool
	Cancelled bool

	// ErrorText 是这一台的报错原文：工具自己那条报文、远端原话、服务端提示都原样在里面，
	// 不翻译、不裁剪（词表「报文」）。终端与报告贴的就是它。
	ErrorText string

	// Output 是这一台的原文：命令输出 / 传输明细。呈现层原样用。
	Output string
}

// Extra 是附加格：一个带标签的格子，按模式装各自多出来的东西。
//
// 本阶段只有命令模式，而命令模式**没有**模式特有的事实——输出与退出码都在通用格里。
// 所以这里只落标签；其余四份随阶段 05 加字段（加字段是加格，不是改结构）。
//
// 结算站一个字段都不打开，原样递给报告站（ADR-0035 决定九）。
type Extra struct {
	Mode Mode
}

// ResultPaper 是执行站交出去的那张纸。
type ResultPaper struct {
	// Items 是逐台的事实；没跑的那些不在这里。
	Items []NodeResult

	// Extra 是附加格，原样往下一站传。
	Extra Extra

	// —— 全局格：三样都是事实，不是汇总 ——
	//
	// Total 来自计划（这次打算做多少台），StartedAt／StoppedAt／Interrupted 来自执行。
	// 「成功几台、失败几台」不在这一格里：那是结算站算出来的，两者别混。
	Total       int
	StartedAt   time.Time
	StoppedAt   time.Time
	Interrupted bool

	// Tables 是判据表：结算要看分类规则，报告要贴提示语，所以跟着走（ADR-0035 决定五）。
	Tables Tables

	// RunMeta 是运行上下文的密文：收下、原样传给下一站，只有报告站解（ADR-0035 决定九）。
	RunMeta RunMeta

	// Resources 是执行资源（清单、脚本、代填的路径）：随行件，执行站原样带过去、
	// 结算站一个字段都不开、报告站收尾时用——`report.txt` 第一段的「节点清单：」那一行
	// 要它（ADR-0032 第四条；建站 04 规格第二节：那几条路径要跟到第三张纸）。
	Resources Resources
}
