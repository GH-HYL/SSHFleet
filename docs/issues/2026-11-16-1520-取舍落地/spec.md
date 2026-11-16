# 规格：九条「动」的落地（架构取舍判定之后）

**状态**：done（八张单全部落地；04 号单的进程级验法要等 POSIX 侧，见 `docs/issues/未收尾清单.md`）
**底本**：`docs/issues/2026-11-16-1449-架构取舍判定/spec.md` 的判定表（留 28、动 11）；逐条细则在 `docs/issues/2026-11-09-0945-切换前回顾审计/audit-run-2026-11-09/报告.md` §2.5 与 `回执/`

## Problem Statement

判定表把 39 条取舍判完了：28 条留（代价写在表里），11 条判「动」。判「动」的那几条只有一句话摘要，没有工单——下一个人拿不到「改哪里、改到什么算完、会不会动屏上的字」。其中两条落在红线区，按规范一.1 第七条要先有议题与 ADR，否则不许动。

## Solution

一条一个工单，八张。两条红线的先在 `docs/adr/` 立条、写明是哪个条件变了，工单挂在 ADR 后面。**本规格只到「拆单」**：工单落地另起一轮，拿到授权再动手。

**核现场的结论与判定表有一处不同**：**04-J5 已办**。判定表按 2026-11-09 的审计摘要判「动」，核现场发现 `c80bd72 refactor(判据): 五处各写一份收成一处` 已经把解释器归一收进 `internal/criterion/interpreter.go` 的 `InterpreterBaseName`：`parse/downlink.go:251` 与 `message/confirm.go:67` 都调它，反斜杠那条分叉也在注释里给了判断（只认 `/`，理由是「解释器喂给目标机，那里的分隔符就是 `/`」）。所以九条变八条，判定表那一行改标已办。

其余八条逐条核过现场，摘要与位置仍成立：

| 单 | 现场 | 核出的实况 |
| --- | --- | --- |
| 01 02-J6 | `guard/redline_tip_test.go:16`、`redline_tip_endings_test.go:71,114` | 三处都按 `../../../../` 认仓库根，且产物落 `build/建站02红线_2026-11-01` |
| 02 11-J10 | `guard/redline_report_test.go:26-29` | 反射钉 `message.StatisticsLine`，同文件另两节判的是报告站自己 |
| 03 04-J6 | `parse/argv.go:547,553` | 两份手抄名单仍在，`:546` 那句「不另抄一串」就在它们上面 |
| 04 11-J8 | `keytool/run_confirm_test.go:25` | 产物落 `build/建站09_AB_2026-11-06_1535`，那是 03 轴取证目录 |
| 05 08-7 | `mask/mask.go:13-20` | 导出理由是「给类七认」，而类七用 `strings.Repeat("*", 4)` 自拼，不读这个符号 |
| 06 08-8 | `contract/danger.go:20-25` | 全零引用；闸门用的是 `DangerReport` + `message.DangerBox(report, bool)`（`gate/run.go:343-369`） |
| 07 12-J1 | `README.md:115` | 写 `modules/SSHFleet_Go/`，同句还写「Go 1.27+」，而 `go.mod` 是 `go 1.26.0` |
| 08 12-J2 | `tools/release/release.py:174` vs `:241,248` | `precheck` 查三份配置，打包拷五份；缺的两份走 `shutil.copy2` 的 traceback |

## Implementation Decisions

- **基建复用，不新开机制**：认仓库根用各包已有的 `buildTmp()`（`internal/*/tmpdir_test.go`，从包目录上溯找 `go.mod`，落到 `build/tmp/test/<包>`）。八条里有两条要它，都不必造新东西。
- **红线两条先立 ADR、状态 proposed**：议题已有（判定表那份），ADR-0053／0054 只补「哪个条件变了」。开发者点头后转 accepted，单才解封——红线区默认不能改，一句「行」不算（规范一.1 第七条）。
- **屏上字节**：只有 04-J6 会碰屏上字（报错里参数的次序），按「先钉用例、再改实现，用例一字不动仍绿」保证逐字不变；其余七条不碰使用者看得见的字。12-J1 属说明性文字（规范一「说明性文字不受限」），不受字节不变约束，但它随包分发，记 `CHANGELOG` 的 `Changed`。
- **老工程只读**：`modules/SSHFleet_Go` 一个字都不改（ADR-0038）。

## Testing Decisions

- 每条单的判据都落到「跑哪条命令、看见什么算过」，不写「看起来对」。
- 红线两条的判据本身一字不改：02-J6 另加「模块复制到别处仍绿」；11-J10 另加「把 `message` 的签名改掉，报告红线不再红」——用 `-overlay` 挂探针，不往包里写文件。
- 全批收尾：`GOTMPDIR="$PWD/../../build/gotmp" go test -count=1 ./...` 十七包全绿，`go vet`、`go vet -tags integration`、`gofmt -l` 干净。

## Out of Scope

- **09-3 的收尾测试缺口**：塔尖的 `fatal → os.Exit` 不在现有接缝上，盖住它要先定一个新接缝（规范三.3：没说定就别写）。另起一张。
- **06-J4 那条施工记录 ADR**（判定表「留」里点名要补的）：是记录不是改动，与这批无关。
- **推远端**：开发者 2026-11-16 明说不推送，两个远端都不动。
