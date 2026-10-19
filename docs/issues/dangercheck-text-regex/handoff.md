# 交接：危险命令检测改为纯文本正则（第二轮之后）

> 写给压缩后的会话。**01–08 已完成并提交，只剩 09 收口。**
>
> ⚠️ **临时文档，实施完成后删除**（本文替代第一轮那份，内容已重写；第一轮的交待只留结论）。
>
> 本文只讲四件事：**现在到哪了、还剩什么、哪些决定别再重问、哪里会踩坑**。规格与工单仍是事实来源。

---

## 一、现在到哪了

分支上最近五个提交（`git log --oneline -5`）：

```text
d60a36b docs(readme): 危险命令分析改成按原文匹配，依赖表去掉解析库
60d8b55 feat(dangercheck): 17 条规则按边界表落地，语料 162/162
de24d94 test(dangercheck): 判定层测试不再依赖配置内容，补规则边界表
590cd09 docs(issues): 危险命令检测重写议题补实测底本与语料草稿
1797beb feat(dangercheck): 危险命令检测改按命令原文匹配，规则文件 regex 改数组
```

**工单 01–08 状态全部是 `done`**；`09` 与 `spec.md` 仍是 `ready-for-agent`，本轮没动。

当前实测（截至 2026-10-19 傍晚）：

```text
go vet ./...                        → 无输出
go test ./...                       → 11 个包全 ok
go test -run TestRuleBoundaries     → PASS（17 条规则的必须命中/必须不命中）
go test -run TestDangerousCorpus    → 162/162（严格比对已是默认）
```

**代码落点**（都在 `modules/SSHFleet_Go/`）：

| 文件 | 管什么 |
| --- | --- |
| `internal/dangercheck/judge.go` | `Analyze`（接缝，签名不变）、`Match`（`Lines []int` 多行号）、`riskOrder` 四档 |
| `internal/dangercheck/text.go` | 纯文本整备：去注释（保行数）、路径片段归一（保行数、行尾补一个空格作词边界） |
| `internal/dangercheck/rules.go` | 规则加载与校验（定案 N/O）、逐行匹配（**不跨行**） |
| `internal/dangercheck/boundary_test.go` | 17 条规则的可验证边界表（04–06 的验收条目） |
| `internal/dangercheck/corpus_test.go` | 语料读取（带引号处理的制表符 CSV）+ 逐条比对 |
| `internal/dangercheck/judge_test.go` | 判定层行为测试，**自带规则、不读配置** |
| `internal/output/danger.go` | 警告框七行、`formatLines` 多行号、内容截断 |
| `config/keywords_dangerous.conf` | 17 条规则（禁止 6 / 高 5 / 中 6），`regex` 数组 + `reason` |

`parse.go` 已删除，`mvdan.cc/sh` 依赖已摘除。

---

## 二、还剩什么：只差 09

### 已完成的部分（09 的勾选里已经能打的）

在 `build/atest-1/e2e/` 沙盒里端到端跑过（二进制与 config 副本都在那里）：

| 验证项 | 结果 |
| --- | --- |
| `rm -rf /etc` → forbidden | 打印警告框，**exit=1** |
| `reboot`（high）带 `--yes` | 打印完整警告框（级别「高」），命令照常下发，exit=0 |
| 同一规则命中 3 处 | 警告框 `行号: 1, 2, 3`，合成一块 |
| 只有注释提到 `rm -rf /`、行尾注释提到 `reboot` | 不弹框（注释已剔） |
| 行号定位 | 脚本第 2 行的 `rm -rf /etc` 报 `行号: 2` |
| 日常运维命令表（audit.md 第二节 22 条） | **0 条落 forbidden**；11 条会提醒（见下） |

### 还没做的

1. **09「五、文档与现实一致」**：手册里提到的每个规则名在规则文件里真实存在——逐条核对（规则名清单就是 `config/keywords_dangerous.conf` 里的 17 个 `name`）。
2. **09「六、语料的可信度」**：抽 5–10 行读「理由」与期望是否讲得通（`TestCorpusLoads` 已机器校验「理由非空、无正则字符」）。
3. **09「七、收口」**：`spec.md` 与 09 状态改 `done`；CHANGELOG 当天段补一段验收记录。
4. **删本文件**。
5. **一个需要开发者拍板的事项**：日常表 22 条里仍有 11 条提醒——`yum remove telnet`、`rpm -e --nodeps`、`chmod 777`、`chmod u+s`、`iptables -F`、`userdel`、`groupdel`、`rm -f /var/log/app.log`、`shutdown -r +10`、`reboot`、`init 6`。前 8 条是规则设计使然（这些动作本身就该看一眼），后 3 条是重启降档后的 high。**这不是漏改**；若还嫌多，要开发者点名砍哪条，不要自己砍。

---

## 三、已定、别再重问的决策

### 第一轮（spec.md 里的定案 A–P）

A `regex` 数组；B 整份原文匹配、不跨行、行号反算、多命中全列；C 语法解析退役、路径归一保留；D 与下发侧同一份正文；E 手册同步；F 规则过筛子 38→17；G 本轮只删不增；H 旗标由规则自己列全；I 允许质疑语料、不凑绿；J `mkfs`/`dd` 不分主次盘；K 新增 `reason`（30 显示列上限）；L 内容超宽截断、框线等宽；M `low` 保留为合法档位、出厂不预置；N 停用规则只校验 `name`；O 加载报错能定位/能理解/能处置；P 不按机器分组。

### 第二轮（2026-10-19 对话里开发者口头定的，**同等效力**）

| 项 | 结论 |
| --- | --- |
| 授权口径 | 「授权开工」一次管一个阶段；实现在 `1797beb` 起算 |
| 阶段划分 | 01+02+03 一个阶段；04–08 一个阶段；09 一个阶段 |
| 规则名 | 用 **`杀死所有进程`**（不是 audit 里的「杀光进程」） |
| `echo /etc \| xargs rm -rf` | 期望改 `none`，理由是「目标在 rm 前面，纯文本正则不认」 |
| 手册 D2 两处 | 一并改（`--yes` 下照常打警告框、只是不问） |
| 提交形态 | 分阶段提交 |
| **刻意绕过不追** | `/tmp` 写法、命令拆分、`cd /tmp && rm -rf ./*`、变量拼路径、参数列表（`subprocess.run(["rm","-rf",...])`）——**不要求命中**，接受漏报 |
| **`/var/log` 本身算禁止** | 「批量执行里没有正当删日志目录的场景」；但 `/var/log/app.log` 这类**目标变成文件**的归 medium |
| 同类结构类推 | 判断目标是目录还是文件，不看路径前缀。`/etc/nginx` → medium，`/root/.ssh` → forbidden（管理密钥） |
| 规则写法倾向 | **枚举可接受**，重点是系统级目录；**不破坏规则文件结构**（不加字段、不做两遍后处理） |

---

## 四、最容易翻车的地方

1. **Go 正则的坑**：`regexp` 不支持 `(?!...)` / `(?=...)`（编译直接报错）。另外，集合 `(?:[\s;&|)]|$)` 在引号结尾处**会匹配失败**——引号必须显式放进集合：`[\x22\x27\s;&|)]`。这是这轮排查 126 条误判时找到的根因，改正则时照抄这个形态。
2. **TOML 单引号字符串里不能写 `'`**：写 `\x27`、`\x22`，不要把引号直接塞进字符类（会把 TOML 字符串提前截断）。
3. **整备文本的细节**：`prepare()` 会去注释、归一路径，并在**每行末尾补一个空格**给词边界；所以规则里写 `\s` 收尾是安全的。行数不变，行号不错位。
4. **停用规则只校验 name**，但**启用规则数**必须 ≥ 10（`minRules`），否则加载失败。
5. **语料是制表符 CSV**：多行脚本必须用双引号包住整个单元格、内部 `"` 写 `""`。用 `csv.Reader` 读，**不要按行切**。
6. **测试写在哪里**：判据测试（`judge_test.go`）自带规则集、从不读配置；配置相关的断言只放 `boundary_test.go` 与 `corpus_test.go`。
7. **构建临时目录**：`GOTMPDIR` / `TMP` / `TEMP` 指到 `build/atest-1/tmp`（写工作区外会被安全软件静默删）。
8. **不要碰** `docs/issues/result-category-order/` 与 `internal/output/report.go`（另一件事）。

---

## 五、交接后的建议顺序

1. 先读 `docs/issues/dangercheck-text-regex/issues/09-integration-and-verification.md` 的勾选清单；
2. 把「二、已完成」里没写进工单 Comments 的实测结果补进 09 的 `## Comments`；
3. 做「五、文档与现实一致」「六、语料的可信度」两项核对；
4. 补 CHANGELOG 验收记录 → 改 spec 与 09 状态 → 删本文件 → 提交。
