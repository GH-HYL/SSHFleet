# 规格：措辞迁移（站里自拼的中文收进 message）

**状态**：ready-for-agent
**底本**：`docs/issues/2026-11-12-1454-措辞出口收口/spec.md`（出口已收口，正文没迁）；`docs/issues/2026-11-09-0945-切换前回顾审计/修复面盘点.md` §4.2

## Problem Statement

出口收口之后，「谁能往屏上放字」只剩 `message` 与 `console` 两个口子，可**字本身**还有一半是在站里拼的。改一句口吻的人打开 `message`，改完发现屏幕上没变——那句话其实长在 `execute` 的进度条里、`parse` 的配置校验里，得再去翻两个包。同一件事在两处各有一种说法时，也没人会发现，因为比对只看字节。

## Solution

把站里自拼的中文句子搬进 `message`，站里改成「说出要素、由 `message` 成形」。**屏上字节一个字都不变**，变的是这些字长在哪。

## User Stories

1. As a 改口吻的人, I want 屏幕上每一句中文都在 `message` 里找得到, so that 改说法只翻一个包。
2. As a 使用者, I want 这一版屏上所见与上一版逐字相同, so that 升级不产生新的「怎么变了」。
3. As a 维护者, I want 站里不再出现写死的中文句子, so that 下一处新文案不会又长在站里。

## Implementation Decisions

- **只搬构造点，不改措辞**：每个函数照原文拼，`labels.go` 那批就是这么干的（连同它那条扫描判据一起看）。
- **碎片也归 `message`**：进度条那几个词是半句（`已完成 3/5`、`成功 3`），与 `IdentityLabel`、`ValueInvalid` 同属半句族——按半句进，不硬凑成整句。
- **风格不搬**：`lipgloss` 的着色与 `styleTitle`／`styleOK` 这类渲染留在 `execute`；`message` 只出字，不出样式。
- **耗时格式（`0:00:12`）进 `message`**：`message` 的职责写明「把数字写成大小」，时间同属此类，且 `stats.go` 已有 `FormatSpeed` 的先例。
- **不新开包**：全部加进既有的 `message`，不新建横切包（改锚要单独走）。
- **老代码只是参照物**：迁移过程中发现某句老文案本身有问题，**不顺手改**——另记一份对照（原句、建议句、理由）交开发者，同意之后才算下一批。

## Testing Decisions

- 接缝是包边界与既有用例：`execute` 的 `progress_test.go`／`transfer_view_test.go`／`verdict_test.go`，加 `message` 自己的用例。
- 判据是「字节不变」：迁移前后同一条用例的断言不动、仍绿，即为逐字相同。
- 新加的每个 `message` 函数各配一条断言（字面量写死），照 `labels_test.go` 的先例。

## Out of Scope

- 无：`execute` 屏上 16 处与 `parse` 的 31 处都在本规格内，按工单 01～05 分批（04、05 是把 02 拆细之后的两批）。
- **执行期日志那 15 处**（`log_node.go` 9、`log_session.go` 6）：只进 `SSHFleetExec.log`，读者是维护者；要不要迁等开发者定。
- 文案本身的优化：一律先出对照、等同意。
- `archive`／`report` 那两句系统原文改由类别表成形（0044 说过的后续）。
