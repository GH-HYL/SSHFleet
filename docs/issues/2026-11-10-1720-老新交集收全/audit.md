# 实测报告：老新交集收全

**跑于** 2026-11-10 17:20，脚本 `build/交集比对_2026-11-10_1720/main.go`（只读，未改老工程、未改新工程，`git status -- modules` 为空）。
**用法**：`go run . <老工程根> <新工程根> [--ident]`；`--ident` 只列逐字同体的那些。
**判据两条**：

- **逐字同体**：取该声明在源文件里的字节区间，`go/scanner` 取 token 流（跳过空白与注释与注释位置），拼串取 sha256 前 16 位；两份相同即同体。
- **判据字面量同**：同一区间里 `token.STRING` 排序去重后逐项相同。正则、关键词这类字面量被抄过来、函数体被重排过，这一列会亮。

## 一、总数

| 项 | 数 |
| --- | --- |
| 老工程导出符号 | 224 |
| 新工程导出符号 | 491 |
| 交集 | 48 |
| **逐字同体** | **20** |
| 判据字面量同 | 34 |

## 二、逐字同体的 20 个（带位置）

| # | 符号 | 类别 | 老（包 文件） | 新（包 文件） | 字面量 老/新 |
| --- | --- | --- | --- | --- | --- |
| 1 | `CategoryCount` | type | `result internal/result/statistics.go` | `contract internal/contract/ledger.go` | 0/0 |
| 2 | `FormatSpeed` | func | `output internal/output/terminal.go` | `message internal/message/stats.go` | 3/3 |
| 3 | `KeyState` | type | `credential internal/credential/masterkey.go` | `contract internal/contract/key.go` | 0/0 |
| 4 | `KeyStateDiverged` | const | 同上 | 同上 | 1/1 |
| 5 | `KeyStateOK` | const | 同上 | 同上 | 1/1 |
| 6 | `KeyStateStaleShell` | const | 同上 | 同上 | 1/1 |
| 7 | `KeyStateUnpersisted` | const | 同上 | 同上 | 1/1 |
| 8 | `KeyStateUnset` | const | 同上 | 同上 | 1/1 |
| 9 | `MatchOptions` | type | `ssh internal/ssh/matcher.go` | `execute internal/execute/interactive.go` | 0/0 |
| 10 | `Mode` | type | `cli internal/cli/args.go` | `contract internal/contract/task.go` | 0/0 |
| 11 | `ModeCommand` | const | 同上 | 同上 | 1/1 |
| 12 | `ModeDownload` | const | 同上 | 同上 | 1/1 |
| 13 | `ModeNone` | const | 同上 | 同上 | 1/1 |
| 14 | `ModePasswd` | const | 同上 | 同上 | 1/1 |
| 15 | `ModeScript` | const | 同上 | 同上 | 1/1 |
| 16 | `ModeUpload` | const | 同上 | 同上 | 1/1 |
| 17 | `StepEnumerate` | const | `ssh internal/ssh/types.go` | `contract internal/contract/step.go` | 1/1 |
| 18 | `StepPasswd` | const | 同上 | 同上 | 1/1 |
| 19 | `StepPrecheck` | const | 同上 | 同上 | 1/1 |
| 20 | `SystemReason` | func | `common internal/common/systemreason.go` | `message internal/message/system.go` | 0/0 |

## 三、这 20 个怎么读

**只有两个是函数体，其余 18 个是类型与常量。**

- `FormatSpeed`——**真搬移**。91 已实锤（函数体逐字节同），本表复核为同体、3 条字面量也同。新工程里它住在 `message`，而 `message` 的话术与格式本就逐字照老版（A/B 要求），所以这一处要么是「应当照搬的格式化规则」，要么是「该重写成新结构的漏项」——**这一条要处置**（见第五节）。
- `SystemReason`——同体，但**已有裁定**：`message` 与 `credential` 各一份、认下不合并（开发者 2026-11-06 定，两份文件的注释都写着「改这里要一起改那边」）。这一条不是漏项，是备案。
- 其余 18 个——**同名的词表与外形，不是搬实现**。理由是它们全都属于「两版共同对外的一批名字与值」：
  - `Mode` 与七个 `Mode*`、`Step*` 三个：模式名与步骤名是使用者可见面的一部分（`report.txt`、日志、统计块都印它们），值相同是要求、不是巧合；新工程把它们放进 `contract`，正是「纸只装结构化事实」那条决定的落点。
  - `KeyState` 与五个 `KeyState*`：同上，密钥状态名是屏上印的字。
  - `MatchOptions`、`CategoryCount`：两个字段外形相同的结构体，零字面量。`CategoryCount` 的字段就是「分类名 + 计数」，两版都从统计块那一面来，外形相同是必然。

**「判据字面量同」这一列不构成本身证据。** A/B 要求两侧屏幕与归档产物逐字一致，所以文案、正则、关键词本来就得照抄——34 个里绝大多数（`KeyStatusReport` 21/21、`ResultLine` 11/1 等）是「同一批使用者可见文案」，抄是要求。真正的搬移面只有「同体」那一列，本表按这一列下结论。

## 四、与 91 报的 30 个差在哪

91 报的是「老 181 ∩ 新 301 = 30」。本表实扫 224／491／48。差 18 个，可指出的两条：

1. **计数口径**：本脚本按 `ast` 逐个名字计，`const`／`var` 组里每个名字算一个（`ModeCommand`、`KeyStateOK` 这类因此单列）；91 的 181／301 未说明是否含 `const`／`var`，从其报的实例看（`FormatSpeed`、`SanitizeRemark`、`DisplayWidth`、`Judge`）都是函数，很可能只算了函数与类型。
2. **扫描面**：本脚本跳过 `_test.go`、`testdata`、`cmd_scratch`、`build`、`release`，其余全扫。

**结论不变**：函数面的交集仍是 91 报的那几个，本表多认出 `FormatSpeed` 与 `SystemReason` 两处同体；18 个新增交集全是类型与常量，都是名词表，不是搬实现。

## 五、要处置的

**一条要开发者定**：`FormatSpeed`（新 `message/internal/message/stats.go`，老 `output/internal/output/terminal.go`）——留（登记成「照搬的格式化规则，理由是什么」）还是改写成新结构（如并入更一般的格式化函数）。本议题不自作主张。

**结论（2026-11-12，评估后定）**：**登记成照搬的格式化规则，代码不动。**

判据：A/B 是防「搬错了」的手段，不是目标——老版设计有问题、或照抄会破坏新结构时允许开口，但要评估值不值并写下来（开发者 2026-11-12 明示）。这一处评估完不值得开口：

- 输入受保护：速度来自 `execute/transfer_view.go:37` 的滑动窗口 `update()`，`deltaTime <= 0 || deltaBytes <= 0` 直接返回 0，`FormatSpeed` 拿不到 `Inf`／`NaN`，不会印出怪值。
- 两处瑕疵都只在边界：`< 1024` 用 `%.0f`，1023.6 会印成 `1024B/s`；`1023.95KB` 会印成 `1024.0KB/s` 而不是 `1.0MB/s`。窗口采样落在最后 0.05% 那一格里才有，印出来也是人看得懂的近似值。
- 没有 GB 档：SSH 传输到不了 1GB/s，加了是空档。
- 单位按 1024 却写 `KB`／`MB`：命令行工具的惯例，改了反而不合读者预期。

改写的代价是动 `transfer_view.go:254` 与 `:270` 两个调用点、连带 `message` 的形态断言，A/B 要重跑传输面板那几场才有把握——换不来任何使用者能感知的好处，属规范三.2 明令不许的「为更好读而动结构」。


**一条备案即可**：`SystemReason`——已有 2026-11-06 的裁定，两份不合并。

**18 个类型与常量**：无需处置，本表已给判据与理由。

## 六、未覆盖面与局限

- **同名多份只留最后一个**：`collect` 以符号名为键，windows／unix 各一份声明的同名符号被后扫到的那一份覆盖。`SanitizeRemark` 因此判「否同体」（字面量同），而 91 说 unix 版逐字同——**两张单的口径差在这里，不是结论冲突**。要逐文件严格判，键得改成「符号 + 文件」，并允许「任一份同体即算同体」。本议题不做，如下一轮要收全再开单。
- **方法不算**：只收导出的顶层函数，方法（带接收者）不在面内。91 的「30 个交集」也未含方法。
- **不判语义等价**：只看 token 流与字面量，重写过的等价实现一定会判「否」——这正是这一列要的性质。
- **没跑 A/B、没碰红线区、没改两个工程**（`git status -- modules` 为空）。
