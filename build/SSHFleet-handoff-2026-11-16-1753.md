# 交接：SSHFleet（2026-11-16 17:54 定稿）

**给下一个会话。** 只写「现在」「还剩什么」「怎么接着干」；沿革在 git log、议题 `## Comments` 与 ADR 里，不复述。

**这份取代今天四份**（`…-1048.md`、`…-1511.md`、`…-1602.md`，以及本轮的口头清单）：审计对账、红线收窄、`04-H1`、`12-J3`、`06-J3` 裁定、码页三条、keytool POSIX 侧、路径规范化结案，全都办完了。

**开发者 2026-11-16 17:53 原话：「推，写个交接文档，我要换开发环境继续了，那个没有当前的沙盒」。** 也就是说：**下一台机器上没有本轮的 `build/` 沙盒**，第 5 节列了要重建什么。

---

## 〇、大方向

**终点是切换**，判据三句没变：

1. **新工程完全顶替老工程**——发布、打包、手册、日常改动都只走 `modules/SSHFleet2_Go`；
2. **老工程可以删了**——原地留着当退路，按 ADR-0038 等下一次大改之后再删；
3. **重构承诺兑现**——「改一句口吻只改一处」「一条判据只有一份」「站间依赖单向」在新工程里真的成立。

**切换前的账已经对平**：审计 110 条零悬空（58 已办、52 判留），`docs/issues/未收尾清单.md` 清零。下一个人不必再翻旧审计——它就是 110 条的唯一总账。

---

## 一、验收快照（2026-11-16 17:54 实测）

| 项 | 实测 |
| --- | --- |
| 工作区 | 分支 `master`，`HEAD = ed18d96`，干净 |
| 版本 | `v1.0.0`（tag 在，两远端都有）；`v12.2.0` 是更早那一版 |
| 测试 | `go test -count=1 ./...` 19 包全绿（`sshfleet2` 38s、`guard` 28s） |
| 静态 | `go vet`、`go vet -tags integration`、`gofmt -l` 全干净 |
| 红线 | 46 条在岗，`TestProjectIsClean` 零违规、放行 0 条、包清单 19 个 |
| 未收尾清单 | 「要复核」空，只剩一条「已判不补」（改密 `no_prompt`） |

---

## 二、这一轮办了什么（按提交号查）

| 提交 | 内容 |
| --- | --- |
| `e7ff160` | 退出码那几个数搬进 `internal/exitcode` 的自定义区；红线只判零／非零与「两种失败同码」（ADR-0056 转 accepted） |
| `4b040c7` | `parse/tables.go` 的分类名改调 `message.CategoryPasswdExpired`（审计 `04-H1`） |
| `b2f4d86` | `release.py` 的 `pick_go` 补 `~/.local/opt/go/bin/go`（审计 `12-J3`） |
| `8456596`／`83db527`／`22e6006`／`0e35c81`／`eab9528` | 审计对账：账表、三张单、`06-J3` 判「认下」 |
| `f8f4527`／`8168824`／`eca1a2e`／`ed18d96` | 未收尾清单收口：`no_prompt` 判不补、keytool 在 WSL 跑过、码页三条验完、路径规范化结案 |

**账表在哪**：`docs/issues/2026-11-16-1602-审计对账/spec.md`——110 条一条一行，结论＋落点／代价。**判留的 52 条不另开 ADR**，代价就写在表里，这是先例（判定表那 28 条同办）。

---

## 三、还剩什么

### 1. 发版（开发者做，执行者不参与）

`python tools/release/release.py`。`--dry-run` 已走通：读未发布区 3 条、最近发布 `v1.0.0`、算出 `v1.0.1`，八步列成「只演不练」。**注意 dry-run 那条路用的是裸 `go`，不调 `pick_go`**，所以 `12-J3` 那个候选只有在真发版时才被验到。

### 2. 两处明摆着的尾巴（都不是切换阻塞项，没拆单）

- **ADR-0047 的「系统原文」类别是过渡**：`main.go:136-140` 还在用它（`archive.Create` 失败那一句）。`archive` 与 `report` 那两句中文迁进类别表之后这个类别就没调用者，届时删掉——ADR 原文写「另开一单」，单没开。
- **`GLOSSARY.md:40` 与代码不符**：词表说构造点是「那一处函数」，代码里四个（判定表判「改词表那一句即可」，本批没改）。改词表要开发者点头。

### 3. 三条「判了不修、只留代价」的机制洞

都在账表里，别当遗漏：`01-S2`（`console.Print` 谁都能调，判据没加）、`07-M4`（进度面板的字已全取自 `message`，但没有判据拦着新字不长在站里）、`02-J3`（出口白名单只数个数）。

### 4. 老工程 `modules/SSHFleet_Go`

原地留着当退路，**只读不改**，等下一次大改后再删（ADR-0038）。

---

## 四、这一轮定下的口径（别改回去）

1. **不上屏的字不往 `message` 搬**：`parse` 15 处执行期日志、`execute` 55 条日志与报文，判留。先例是 `atoi` 那两条。
2. **报文与日志是两件事**：`sshfleet.<env>.<stage>: <detail>` 由 `execute/message.go` 的 `msg()` 拼，`detail` 是英文、原样进 `report.txt`；日志那 55 条是写进 `SSHFleetExec.log` 的中文。
3. **只报真需要他拍板的事，一次一件，说完就干。** 能按先例判的自己判（这一轮 `06-J3` 就是照 `06-H2` 的先例判的「认下」）。
4. **判据落在了代码上再下结论**：这一轮推翻了两条审计自己的判断——`05-F1`「冒号漂开」不成立（老版就是屏上半角、日志全角）、`91-B`「26 个未判」不成立（老新交集收全早判完了）。**别照摘要干活，先核代码，必要时核老工程源码。**
5. **改红线＝议题＋ADR 两样齐**（规范一.1 第七条）；ADR 的「决策人」写清哪句是他的原话、哪句是执行者推导（0054／0056 就是这么写的）。
6. **授权开工只认四个字**，或他明确说「都你决定」「按你说的做」。

---

## 五、跑法与现场（**新环境要重建的，都在这节**）

### 5.1 测试

```bash
cd modules/SSHFleet2_Go
mkdir -p ../../build/gotmp
GOTMPDIR="$PWD/../../build/gotmp" GOPROXY=https://goproxy.cn,direct go test -count=1 ./...
```

临时根由测试自己钉到 `build/tmp/test/<包>`，`GOTMPDIR` 仍要指进 `build/`。全量约两分钟。

**`go vet -tags integration` 要单独跑**，不带标签的 `go test ./...` 看不见那一族（那一族要真机，缺环境会 SKIP）。

### 5.2 POSIX 侧的测试（`keytool` 三条进程级用例、`ext4` 相关行为）

Windows 上那三条是 `t.Skip`。**WSL 里默认没有 Linux 的 go**——PATH 里那个 `go` 是 `/mnt/c/Program Files/Go/bin/go`（Windows 二进制，在 WSL 里跑等于按 `windows` 编，只会全 SKIP）。这一轮的做法：把 `go1.26.10` 解到 `~/sdk/go`（阿里云镜像，用户目录，没动 apt），然后：

```bash
export PATH=$HOME/sdk/go/bin:$PATH
export GOTMPDIR=/mnt/c/<repo>/build/gotmp
export GOPROXY=https://goproxy.cn,direct
cd /mnt/c/<repo>/modules/SSHFleet2_Go && go test -count=1 -v ./internal/keytool
```

**换机器这一份 go 不会跟着走**，要重解一次。

### 5.3 手跑一个二进制（要 config、要码页）

`build/rev_20261116/`（本轮的手跑沙盒，不进版本控制）里有编好的 `sshfleet.exe` 与一份可用 `config/`。重建法：`go build -o build/rev/sshfleet.exe sshfleet2`，再把 `config/` 整个拷过去（`SSHFleet.conf` 与三份 `keywords_*.conf` 缺一不可）。

码页要在**真 cmd 窗口**里看（`chcp 936` 后再跑），Git Bash／PowerShell 的码页与它无关。

### 5.4 审计对账用过的扫描器（本轮新写，值得留）

`build/对账_2026-11-16/scan_cn.go`：按包统计生产代码里的中文字符串字面量（`go run scan_cn.go <模块根> [list]`）。`list` 给出 `文件:行` 与字面量本身。查「某个字还长在站里没有」用它，比 grep 准——它走 `go/ast`，不数注释。

### 5.5 `build/` 的规矩

`build/` 不进版本控制，按天清（当天的与昨天的留着，第三天开工扫一眼删更早的）。**唯一例外是这份交接文档**：开发者点名临时追踪，用完即删——所以它是 `git add -f` 进来的，**别把这个做法推广到别的文件**（规范一.1 第三条禁绕 `.gitignore`）。

**用完这样删**（先取消追踪，再删文件，两件都要做）：

```bash
git rm --cached "build/SSHFleet-handoff-2026-11-16-1753.md"
rm "build/SSHFleet-handoff-2026-11-16-1753.md"
```

---

## 六、会咬人的坑

- **`gofmt -l` 会把整个模块根列出来**：改完 `main.go` 记得 `gofmt -w main.go`。
- **改红线指纹**：改完跑 `go test ./internal/guard`，报错里会印出「该抄的那一行」，照抄即可；**不要手算符号表**。改名单是红线，走议题＋ADR。
- **Git Bash 里 python 写 `/tmp` 会失败**（`FileNotFoundError`）：备份用 `cp` 落到 `build/tmp/`。
- **选项表行序 ≠ 报错次序**：`parse/options.go` 表里 `--timeout` 在 `--number` 之前，报错里是 `-n` 先。派生名单要按显式 `batchOrder` 排。
- **纸不加格**（ADR-0034 决定四）：别把「起不来」塞进 `Plan`。
- **契约里加裸 `string` 要动 `guard/paper.go` 的名单**；红线测试一改，`guard/redline.go` 的指纹登记要跟着改。
- **子 agent 不许编译**：派只读活时 prompt 里必须写「只读、不许 `go build`／`go test`／`go vet`」。
- **`cmd.exe` 不能从 PowerShell 工具里调**；码页这类事用 `chcp.com` 或让开发者开真窗口。
- **`release.py --dry-run` 要求工作区干净**，脏了就报「[失败] 工作区有未提交的改动」，先看清是谁改的再动。

---

## 七、建议技能

| 技能 | 什么时候调 |
| --- | --- |
| `/implement` | 拿到「授权开工」后按工单走 |
| `/commit-then-diff-review` | 每笔提交前后核对 |
| `/code-review` | 对一批改动走两轴审查（派子 agent 时写明只读、不许编译） |
| `/to-spec`＋`/to-tickets` | 新开议题、拆工单 |
| `/domain-modeling` | 写 ADR、改词表 |
| `/handoff` | 再交接时 |

**给下一个会话一句话**：切换前的账已经平了（110 条零悬空、未收尾清单清零、19 包全绿）；剩下的是发版与删老工程两件，都不是执行者能自己拍的；**新环境没有本轮的 `build/` 沙盒，第 5 节是重建清单**。
