# 01: 在 cli.Mode 上建名字表 + 启动期自检（地基）

Status: done
Blocked by: None

**要做的**：在 `cli` 包给 `Mode` 加名字表与启动期自检。本票**不迁任何消费方**——产出是「表可用、漏格会被当场拦住」。

形状与字段定义见 `../spec.md` 第二节。表内容必须与现值**逐字一致**，对照如下（实施时以代码复核为准）：

| 模式 | DirName | LogName | ActionName | ScreenLabel | ReportTitle | TimeoutLabel | SuccessCategory | Transfer |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `ModeCommand` | `command` | 命令 | 执行 | 命令模式 | 命令模式 | 执行超时 | 执行成功 | 否 |
| `ModeScript` | `script` | 脚本 | 执行 | 脚本模式 | 脚本模式 | 执行超时 | 执行成功 | 否 |
| `ModeUpload` | `upload` | 上传 | 上传 | 上传模式 | 上传模式 | 传输超时 | 传输成功 | **是** |
| `ModeDownload` | `download` | 下载 | 下载 | 下载模式 | 下载模式 | 传输超时 | 传输成功 | **是** |
| `ModePasswd` | `passwd` | 改密 | 改密 | 改密模式 | 改密模式 | 改密超时 | 改密成功 | 否 |

值的来源：`DirName` = 现 `ModeName()` 的字符串值；`LogName` = `batch.execModeName`；`ActionName` = `output.ActionName`；`ScreenLabel` = `confirm.buildInfoTable`；`ReportTitle` = `output/report.go` 报告段落；`TimeoutLabel` = `confirm` / `report` 的超时行；`SuccessCategory` = `result.SuccessCategory*`；`Transfer` = 现「是 upload/download」那个判据。

**顺手清掉一处过时注释**：`cli/args.go` 的 `helpEntries` 上方写着「模式（四选一）」（实际已是五选一）。

- [x] `cli` 包新增 `allModes` / `ModeInfo` / `modeTable` / `func (Mode) Info()`
- [x] `init()` 自检：遍历 `allModes`，任一格 `LogName` 为空即 `panic`，报错**指名到模式**
- [x] 表内 8 个字段 × 5 个模式的值与上表逐字一致（有一处填错即本票不通过）
- [x] 故意删空一格后跑一次（如 `go run . -h`），确认**启动即 panic 且报错点出该模式**；验完复原
- [x] `cli/args.go` 的「模式（四选一）」注释已清
- [x] `go vet ./...` 干净、`go test ./...` 全绿

## Comments
- 2026-10-07 完成，提交 `b2956d5`。表落在 `cli/args.go`（与 `Mode` / `ModeName` 同处，未新开文件）。
- 自检实测：把 `ModeScript` 的 `LogName` 删空后 `go run . -h`，启动即 `panic`，报错为
  `执行模式名字表缺格：模式 "script" 没登记。新增模式时请同时补 cli.modeTable 与 cli.allModes`；验完已复原。
