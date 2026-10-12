# 执行模式名字集中化 · 规格稿

Status: 已闭环

**闭环说明（2026-10-08 复核）**：已实施。落地提交 `b2956d5`（工单 01–03 `resolved`、04 已闭环），ADR-0013 状态已置「已实施」；作者用非 root 账号真机 A/B 复跑五条执行模式路径，归一化后逐字一致。

> **决策依据**：`docs/adr/0013-centralize-execution-mode-naming.md`。本文件只放**实施细节**，不重复决策。
> **来源工单**：`docs/issues/architecture-review-2026-10-06/issues/05-mode-centralization.md`。
> **收益口径**：业务正确性 · 改动面 · 维护成本（ADR-0012），不含可测性。

---

## 一、要解决的问题

「这次是哪种模式」这一个事实，现在有 **9 套叫法**散在 `cli` / `batch` / `result` / `output` / `confirm` 五个包里各写各的（清单见 ADR-0013《背景》）。判定本身早就是单点（`cli.Args.ModeName()`），**痛的是派生名字的分散**——落改密那一档实测动了 28 个文件、8 个包，模式相关要核对的点 7 处、手改 4 处。

套用《个人开发规范》二：这属于**已经痛过一次**的干扰，够格拆。

---

## 二、已定的实施形状

### 2.1 名字表挂在哪

挂在**既有的 `cli.Mode` 类型**上，**不新建包**。`output` / `result` / `batch` / `confirm` 本来就 import `cli`，不新增任何依赖边。

### 2.2 表长什么样

```go
// cli 包内

// allModes 模式全集：新增第 6 档执行模式时，必须同时加到这里与 modeTable。
var allModes = []Mode{ModePasswd, ModeCommand, ModeScript, ModeUpload, ModeDownload}

// ModeInfo 一种执行模式的全部名字。字段只增不改名——它同时是呈现契约。
type ModeInfo struct {
	DirName         string // 归档目录英文名（现为 ModeName() 的字符串值）
	LogName         string // 执行期日志中文名
	ActionName      string // 终端明细状态行的动作名（执行 / 上传 / 下载 / 改密）
	ScreenLabel     string // 参数屏「执行模式」行的值
	ReportTitle     string // 报告「执行模式」行的值
	TimeoutLabel    string // 超时行措辞（执行超时 / 传输超时 / 改密超时）
	SuccessCategory string // 成功分类中文（执行成功 / 传输成功 / 改密成功）
	Transfer        bool   // 算不算传输类（上传 / 下载为真）
}

var modeTable = map[Mode]ModeInfo{
	ModePasswd:   {DirName: "passwd", LogName: "改密", ...},
	ModeCommand:  {DirName: "command", LogName: "命令", Transfer: false, ...},
	ModeScript:   {DirName: "script", LogName: "脚本", ...},
	ModeUpload:   {DirName: "upload", LogName: "上传", Transfer: true, ...},
	ModeDownload: {DirName: "download", LogName: "下载", Transfer: true, ...},
}

// Info 取该模式的全部名字。ModeNone 返回零值。
func (m Mode) Info() ModeInfo { return modeTable[m] }
```

**名字表里不再需要「统计档名（execute / upload / download）」这一格**：原来的 `ModeOf` 只是为了把命令与脚本并成一档给统计用。改用 `cli.Mode` 之后，合并由表的字段表达（命令与脚本的 `SuccessCategory` 同为「执行成功」、`Transfer` 同为假），消费方直接按 `cli.Mode` 取值，不必再有一个中间字符串。ADR-0003 记的四套口径因此少一套。

### 2.3 启动期自检（ADR-0013 决定 4）

```go
func init() {
	for _, m := range allModes {
		if modeTable[m].LogName == "" {
			panic(fmt.Sprintf("执行模式名字表缺格：%q 没有登记（补 cli.modeTable，或从 allModes 去掉）", string(m)))
		}
	}
}
```

Go 没有编译期穷尽检查（已实测），所以用启动期检查兜住「加了模式忘了填表」。用标准库，不引任何外部工具。

### 2.4 消费方改签名传 `cli.Mode`（**已定：采用本方案**，2026-10-07）

字符串在包外往返没有意义——表按 `cli.Mode` 取键，包外再拿着 `"upload"` 也没用。故：

- 删 `result.ModeOf`；`main.go` 与 `output.Render` 改用 `args.ModeName()`。
- 以下入参由 `string` 换成 `cli.Mode`：`output.NewReporter`、`output.ActionName`、`output.displayCategory`、`output.WriteOutputXlsx`、`output.WriteResultsXlsx`、`output.newProgressModel` 与进度模型里的 mode 字段。

换掉之后：**包外不再出现模式字符串，打错模式名变成编译错误**。

> 备选（未采用）：保留 `string` 签名、只把各处映射换成查表——改动面约减半，代价是字符串仍在包外流动、打错模式名仍能编译。作者 2026-10-07 采纳本方案。

---

## 三、改动清单

| 文件 | 改什么 |
| --- | --- |
| `cli/args.go` | 新增 `allModes` / `ModeInfo` / `modeTable` / `Info()` / `init()` 自检；清掉「模式（四选一）」过时注释 |
| `result/statistics.go` | 删 `ModeOf`；`Statistics` 改收/取自 `cli.Mode`；成功分类与传输判定改查表 |
| `output/terminal.go` | `ActionName`、`displayCategory` 入参换类型、值改查表 |
| `output/reporter.go` | `mode` 字段与 `NewReporter` 入参换类型；终端明细开关、`logNode`、`successDetail` 三处改查表 |
| `output/output.go` | `Render` 里 `ModeOf(a)` → `a.ModeName()`；两处 xlsx 调用跟改 |
| `output/xlsx.go` | 两处 `ActionName` / `displayCategory` 调用跟改类型 |
| `output/progress.go` | 进度模型 mode 字段与构造入参换类型；传输判定改查表 |
| `output/report.go` | 「执行参数」段落 switch 与超时行措辞改查表 |
| `output/archive.go` | 归档目录名取 `Info().DirName`（`ModeNone` 的「unknown」兜底保留）；清「四者必有其一」注释 |
| `batch/batch.go` | 删 `execModeName`，改查表；清「四种模式共用」注释 |
| `confirm/confirm.go` | 参数屏模式行与超时行措辞改查表 |
| `main.go` | `mode := result.ModeOf(args)` → `args.ModeName()` |
| `output/*_test.go`、`batch` 等测试 | 调用点跟着换类型（机械同步） |

## 四、实施顺序

| 工单 | 内容 | 依赖 |
| --- | --- | --- |
| 01 | 地基：建表 + 启动期自检（不迁消费方） | — |
| 02 | 统计与呈现侧改用表（含删 `ModeOf`、换签名） | 01 |
| 03 | 日志 / 报告 / 归档 / 参数屏改用表 + 清过时注释 | 01 |
| 04 | 收尾：纸面走查 + 真机 + 状态回填 | 02, 03 |

02 与 03 改的文件不重叠，可并行；三票同属**一次**改动，一起验收。

## 五、验收

作者口径：`Mode` 类型一动算**大改动**，分两步验。

1. **纸面走查**：五个执行模式 × 八个落点逐格核——参数屏、报告「执行参数」、归档目录名、执行期日志、终端明细、统计成功分类、超时行、两张 xlsx。任何一格与原值不同都算回归。
2. **真机跑一遍**：命令 / 脚本 / 上传 / 下载 / 改密 五条路径各一次，确认显示与归档目录名不变。
3. `go vet ./...` 干净、`go test ./...` 全绿。
4. **CHANGELOG**：纯重构、用户可见输出不变，按《个人开发规范》四**不记**。若走查发现任何一处显示变化，那一处就是 bug（不是要记的优化项），修到不变为止。

## 六、明确不做

- **`ssh` 包不参与**：`ssh/command.go` 里「这次发的是命令还是脚本」是 `BuildCommand` / `DescribeCommand` 的**输入契约**判断，不是模式叫法；该包刻意不认识参数载体（见其文件头注释）。收进去要给它引 `cli` 依赖、破坏边界。
- **不并 `cli/check.go` 的数模式互斥校验**：它要报「指定了几个」，不是取哪一档（ADR-0003 明写）。
- **不动执行流程、结果判定、参数校验**的既有分工。
