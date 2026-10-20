package ssh

import "fmt"

// 结构形态 → 报文的导出纯函数（每摊一个）。
//
// 为什么抽成函数、又为什么导出：原先那批「给一组布尔 → 中文分类名」的判定挂在判定包里，
// 改造后形态由执行侧在收尾时说成一句陈述事实的报文，判定层只查表。收尾那一段本身
// （连着真实的会话与连接）不好单测，所以把「形态 → 报文」这一小步剥出来、导出、包外测。
// 导出不是为了给外部用，是为了满足「不测私有函数」这条约定——它同时就是这个映射的接缝。
//
// 三条共同纪律：
//   - 入参只是该形态要用的几个布尔与计数，不是整个 Result；
//   - 干净收场返回空串：报文只在「不该算干净成功」的形态下写；
//   - 环节名只陈述现象（finished / canceled / give_up），不写 partial / failed。
const (
	envTransfer = "transfer"
	envPasswd   = "passwd"
	envAnswer   = "answer"
	envAbort    = "abort"
)

// FinishedMessage 传输收尾那句：这批里成了几个、一共几个。
//
// 只有「有成功也有失败」才说：全成了说它没意义（干净收场一个字都不写），一个都没成
// 也不是这句——那时失败原因本身就是全部事实。
//
// total 传**枚举后要传的文件数**（结果里的 TotalFiles），不是「成功数 + 失败数」：
// 逐文件失败会中断该节点剩余的传输，失败数最多 1，后面那些是「没轮到」，仍算在这批总数里。
func FinishedMessage(success, failed, total int) string {
	if failed <= 0 || success <= 0 {
		return ""
	}
	return Message(envTransfer, "finished",
		fmt.Sprintf("%d of %d files transferred", success, total))
}

// PasswdShape 改密收尾要问纯函数的那几个事实（与 Result 的字段一一对应）。
type PasswdShape struct {
	FedAny          bool // 有没有往会话里喂进去过至少一句
	EarlyClose      bool // 远端当场重问（早收场）
	TimedOut        bool // 到点收场
	EnterSignalSeen bool // 整场出现过改密 / 密码过期的服务端信号
	RemoteSaid      bool // 远端自己说过原因（报错原文非空）
}

// PasswdStuckMessage 改密卡住时的报文。
//
// **远端说了原因就让位**：这种形态下分类照旧由判据表按远端原话认，不再补一句 passwd.*
// （与改造前的让位口径一致）。两个例外是超时与早收场：那时原文里那点内容多半是「命令
// 执行超时」这类通用描述，照它分只会盖掉「改密卡在哪一步」这个更有用的信息，所以
// 那两种形态照发报文。
//
// 调用点只在「不干净收场」时问它（干净收场由 PasswdNotExpiredMessage 负责）。
func PasswdStuckMessage(s PasswdShape) string {
	switch {
	case s.EarlyClose:
		return Message(envPasswd, "give_up", "no further prompts after answers used up")
	case s.TimedOut && s.FedAny:
		return Message(envPasswd, "no_reply", "password fed but no reply before timeout")
	case s.TimedOut:
		return Message(envPasswd, "no_signal", "no password prompt seen before timeout")
	case s.RemoteSaid:
		// 远端已经说了原因：分类交给判据表按原话认。
		return ""
	case s.FedAny:
		return Message(envPasswd, "give_up", "no further prompts after answers used up")
	default:
		return Message(envPasswd, "no_prompt", "passwd exited without asking for input")
	}
}

// PasswdNotExpiredMessage 改密整场没出现过入场信号：这台机器本来就不用改。
// 出现过信号时返回空串（那是干净成功，什么都不说）。
func PasswdNotExpiredMessage(enterSignalSeen bool) string {
	if enterSignalSeen {
		return ""
	}
	return Message(envPasswd, "not_expired", "no password-expiry prompt seen")
}

// AnswerMessage 代填收尾那句。
//
// 优先说「几条没送出去」（最具体、可操作）；其次说「全送出去了仍在等」（表空了还超时，
// 工具分不出是脚本卡住还是任务本身久）；其余形态不说话。
func AnswerMessage(missed int, exhausted, timedOut bool) string {
	if missed > 0 {
		unit := "answers"
		if missed == 1 {
			unit = "answer"
		}
		return Message(envAnswer, "missed",
			fmt.Sprintf("%d %s not delivered", missed, unit))
	}
	if exhausted && timedOut {
		return Message(envAnswer, "exhausted_timeout",
			"all answers sent but no output before timeout")
	}
	return ""
}

// AbortMessage 外部中断（Ctrl+C / 上级取消）那句。
//
// 按「中断在哪一步」分两档：连接都没建成的机器压根没被执行到，跟「跑到一半被中断」
// 混为一谈会让人以为它跑过。没被中断时返回空串。
func AbortMessage(canceled, connectSuccess bool) string {
	if !canceled {
		return ""
	}
	if connectSuccess {
		return Message(envAbort, "canceled", "interrupted after session established")
	}
	return Message(envAbort, "connect_canceled", "interrupted while connecting")
}
