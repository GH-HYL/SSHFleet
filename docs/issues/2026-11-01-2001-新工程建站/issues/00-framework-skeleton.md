# 00: 框架骨架——把十七个包立起来，只有形状，没有实现

**要做的**：新工程从「目录不存在」变成「能编译、包清单对得上迁移地图、每个包一句话说清自己只干什么」。

这一单的产物不是功能，是**形状**：后面每一块搬迁都照着它长。做完之后，接手的人打开新工程就知道该往哪一格放，不必先读一遍 ADR，也不必问「这里能不能打印」。

**它是动手实施之前的一道预备**，不算十个阶段里的任何一个。它交付之后，阶段 01 才开工——阶段 01 起就是真搬代码了。

**Blocked by**：无，可立即开工。**实施仍需「授权开工」四个字**——这四个字管这一单，交付后再动要重新授权。

**Status**：done

**编号说明**：`00` 表示「动手实施之前的预备」，不占阶段号；阶段编号仍从 `01` 起。

## 范围

建目录、建模块、建包、写包注释、写包清单规则表。**不写任何业务实现**——全单唯一的函数是那个空 `main`，与规则表的一个读取函数。

新工程根 `modules/SSHFleet2_Go/`，模块名 `sshfleet2`，Go 版本跟老工程（`go 1.26.0`）。十六个包住 `internal/` 下，塔尖是根上的 `package main`。走 `internal/` 的代价是路径长一层；换来编译器挡住仓库外的 import，多一道编译期的墙，且与老工程同形，搬代码时路径能一一对上迁移地图。

包名 `keytool`（密钥管理）与 `guard`（结构检查与红线区同住）由这一单定；改名要说明为什么原来那个说不准。

**规则表的形态**：放在 `guard` 里的一份 Go 源文件，**只有数据与一个读它的函数，不写任何判断**。不走外部数据文件——那要多一条读路径和它自己的出错分支（文件不在、格式不对、打包时漏了），而这张表按 07 号工单本来就算「能让检查闭嘴的东西」，改它一样要按约束类文件走，写成代码并不因此多付代价。

### 包清单（十七个，一句话一句义，取自迁移地图）

| 包 | 只干这一件 | 判据 |
| --- | --- | --- |
| `parse` | 按顺序读配置、读命令行、读清单，装成一份计划；帮助里「有哪些选项」与版本那一行的事实也由它给 | ADR-0032 第四、五条 |
| `gate` | 按规则判这件事该不该做，下「放行／拒绝」的结论（ADR-0034 已把三档收成两档，迁移地图那行是早一版口径） | 同上 |
| `execute` | 并发跑，采集中途事件，把每台收成一条事实 | 同上 |
| `settle` | 算总账 | 同上 |
| `report` | 写报告文件、两张表格、屏幕统计 | 同上 |
| `contract` | 只放三张纸的类型与常量，一个函数都不许有 | ADR-0032 第二条 |
| `console` | 往终端写字（唯一出口） | 第四条 |
| `message` | 把事实拼成一句话并摆好（唯一拼句处、唯一改字处） | 第四条 |
| `mask` | 认出秘密、把它遮住；清单的脱敏副本也由它写 | 第四条 |
| `ask` | 摆出问题、收一行回答 | 第四条 |
| `archive` | 建本次运行的落盘目录、发路径字符串 | 第四条 |
| `exitcode` | 种类 → 退出码 | 第四条 |
| `log` | 写运行日志 | 第四条 |
| `credential` | 读主密钥、解密、报它齐不齐 | 第四条 |
| `keytool` | 跑那三条不进流水线的密钥小命令 | ADR-0036 决定三 |
| `guard` | 结构检查本体与红线区同住一处 | ADR-0032 第四条「怎么核」、ADR-0037 |
| `main`（塔尖） | 分叉、搬信封、退 | ADR-0032 第五条 |

## 验收

- [x] 新工程根下 `go build ./...` 与 `go vet ./...` 都过；`go.mod` 里 `require` 一条都没有
- [x] `go list ./...` 数出十七个包，与上面那张表逐一核过；多一个少一个都要当场说清为什么
- [x] 每个包一个 `doc.go`（塔尖是根上的 `main.go`），只含包注释与 `package` 行。**唯一例外是 `guard`**：它另有一份只放数据的规则表文件
- [x] 注释第一句是迁移地图右列那句话（地图把一块拆成好几行的，取这一包只干的那一件）；第二句写「判据见 ADR-0032 第 X 条」
- [x] 塔尖的 `main` 是本单唯一的函数，**函数体为空**（`func main() {}`）：不打印、不读任何东西、不 `os.Exit`（写终端那类检查立起来的第一天，不许先红自己）
- [x] `guard` 里有包清单规则表，**十七条**，一条不少；每条写清三样：包名、哪一类（五站／横切／契约／密钥管理／检查／塔尖）、允许依赖谁
- [x] 交付时把「迁移地图每一行 → 落在哪个包」的对照贴在工单 `## Comments` 里，作为「包清单对得上迁移地图」的凭据
- [x] 契约包是空的：没有类型、没有函数、没有常量——纯壳
- [x] 没有 `config/`、没有 `_test.go`、没有业务逻辑、没有任何第三方依赖
- [x] 老工程一个字没动（`git status` 里 `modules/SSHFleet_Go/` 无改动）

## 不在这单

三张纸的格子（随阶段 04）、任何检查逻辑（阶段 01）、配置与对账测试（阶段 02）、行为测试（没有行为可测）。

## 做完之后

把「谁经手的、干了什么」记到本议题 `map.md` 的「进度与经手」；然后拆阶段 01 的议题——**拆法不在这一单里预写**，那时才知道手上有什么。

## Comments

### 2026-11-01：交付

`go build ./...`、`go vet ./...`、`gofmt -l .` 三项干净；`go list ./...` 数出十七个包；`go.mod` 里 `require` 零条；模块根没有多出来的可执行文件。

**迁移地图 → 新包对照**（「包清单对得上迁移地图」的凭据）：

| 老位置 | 落点 |
| --- | --- |
| `main.go` 步骤 1／3／6／7 前置；步骤 3 帮助分支 | `parse` |
| `config/config.go`、`config/passwd.go` | `parse` |
| `cli/args.go`、`source.go`、`answer.go`、`interpreter.go`、`script.go`、`remark_*.go` | `parse` |
| `cli/check.go`、`cli/passwd.go` | `gate` |
| `nodelist/csvread.go`、`fields.go`、`precheck.go` | `parse` |
| `verdict/keywords.go`（读表那半） | `parse` |
| `verdict/verdict.go` | `execute` |
| `common/csvtext.go`（CSV 那半与判二进制那半）、`common/ip.go` | `parse` |
| `main.go` 步骤 5／7；`dangercheck/rules.go`、`text.go`、`judge.go`、`check.go`、`confirm/confirm.go` | `gate` |
| `main.go` 步骤 8；`batch/batch.go`、`collect.go`、`pool.go`、`progress.go` | `execute` |
| `ssh/client.go`、`connect_message.go`、`exec.go`、`command.go`、`shape.go`、`message.go`、`stages.go`、`types.go` | `execute` |
| `ssh/sftp.go`、`space.go`、`interactive.go`、`matcher.go`、`outputtap.go`、`passwd*.go`、`progress.go` | `execute` |
| `output/progress.go`、`reporter.go`、`terminal.go`（结果行那半） | `execute` |
| `result/statistics.go` | `settle` |
| `output/output.go`、`report.go`、`xlsx.go`、`terminal.go`（统计块那半） | `report` |
| `output/archive.go`（建目录那半与复制那半）、`link_unix.go`、`link_windows.go` | `archive` |
| `common/errtext.go`、`systemreason.go`、`width.go`、`bytes.go`；`output/danger.go` | `message` |
| `common/interactor.go` | `ask` |
| `common/color*.go`、`console_*.go` | `console` |
| `common/mask.go`、清单脱敏副本 | `mask` |
| `log/log.go`、`main.go` 步骤 2 | `log` |
| `credential/*`（读密钥、解密码那半） | `credential` |
| `credential/*`（生成密钥、看状态、转换凭据那半）、`main.go` 步骤 4 | `keytool` |
| `main.go` 步骤 0、退出与收尾 | `main` |
| ——（地图里没有对应行） | `guard`、`contract` |

`guard` 与 `contract` 在地图里没有来源行：前者由 ADR-0032 第四条「检查自己住单独一个包」要求，后者由 ADR-0032 第二条要求。这两包不是从老代码搬来的，是从结构里长出来的。

**补记（2026-11-01，审计后）**：`exitcode` 也不该算「有来源行」——老工程没有这个包，它是老 `main.go` 里那张退出码表的去处。04 号已追加更新记录把「步骤 0、退出与收尾」拆开：码表归 `exitcode`，分叉与退归塔尖。这张对照表原来漏了它。

**一处偏离地图原文**：`gate` 的包注释写「放行／拒绝」两档，地图与 15 号工单写的是三档——ADR-0034 的更新记录已把闸门收成两个出口，照新口径写。
