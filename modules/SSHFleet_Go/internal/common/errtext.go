package common

import "errors"

// 报错三行套的构造点：标签与换行在这里拼，调用点只给内容。
//
// 四种形态按两件事分——有没有系统原文可引、有没有「怎么改」可写：
//   - ErrUser：三行齐。有系统原文（`os` 报错），也有提示可写；
//   - ErrUserReason：两行。有系统原文，但没有合适的提示可写；
//   - ErrUserShort：两行。纯形态判定，没有系统原文可借（「不是目录」「互斥」这类自己看出来的）；
//   - ErrInternal：三行。使用者看得见却改不了，第三行固定让他把命令反馈回来。
//
// 这里只负责标签与换行：**不 trim、不加句号、不改写任何一句内容**。
// 第 1 行该怎么拼、提示该怎么说，仍由写那个报错点的人决定（ADR-0028 决定 4）。
//
// 执行阶段的单机失败不走这里——那里用报文；日志与内部断言也不走（ADR-0028 决定 2）。
const (
	reasonLabel  = "\n原因："
	hintLabel    = "\n提示："
	internalHint = "这是内部错误，请把本次命令反馈给作者"
)

// ErrUser 拼「问题 / 原因 / 提示」三行：有系统原文可引、也有「怎么改」可写的报错用这个。
func ErrUser(problem, reason, hint string) error {
	return errors.New(problem + reasonLabel + reason + hintLabel + hint)
}

// ErrUserReason 拼「问题 / 原因」两行：有系统原文，但写不出有意义的提示时用这个。
//
// 写不出就别硬凑一句「请检查文件是否存在」——那是废话，还不如不写。
func ErrUserReason(problem, reason string) error {
	return errors.New(problem + reasonLabel + reason)
}

// ErrUserShort 拼「问题 / 提示」两行：没有系统原文可借的形态判定用这个。
//
// 「系统说它不存在」与「我们看出它不是目录」是两回事：后者去借 `os` 的报错会自相矛盾
// （Windows 对普通文件回的是「找不到路径」），所以不带原因行。
func ErrUserShort(problem, hint string) error {
	return errors.New(problem + hintLabel + hint)
}

// ErrInternal 拼「问题 / 原因 / 固定说明」三行：使用者看得见、却没有任何动作能让这次跑通的那种。
//
// 第三行是恒定的——它与「你去改」不是一个语义，所以不并进 ErrUser 的提示栏。
func ErrInternal(problem, reason string) error {
	return errors.New(problem + reasonLabel + reason + hintLabel + internalHint)
}
