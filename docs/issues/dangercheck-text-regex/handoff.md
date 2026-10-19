# 交接：危险命令检测改为纯文本正则（给新会话）

> **这是一份自足的交接文档。新会话不需要任何上文，读它就能接手。**
>
> 现在状态：**工单 01–08 已完成并提交。09 收口，外加一轮「按开发者最新口味再砍规则」的调整，都还没做。**
> 开发者在 2026-10-19 傍晚决定开新会话继续，并明确「本会话后面不再实施，只维护文档交接」——所以本文不假设你知道之前的任何讨论，同时**新会话要做的活比原来多一块**。
>
> ⚠️ **临时文档，09 收口完成后删除**，不归档、不补 `Status:`。

---

## 一、先看这几份（按顺序）

| 顺序 | 文件 | 看什么 |
| --- | --- | --- |
| 1 | `docs/个人开发规范.md` | **最高约束**。尤其「红线 1 授权开工」「红线 5 改缺陷先复现」「测试只写在说定的接缝上」 |
| 2 | `AGENTS.md`（仓库根） | 开工门、子 agent 授权、交付约定（不调文件呈现工具，正文写路径） |
| 3 | `docs/issues/dangercheck-text-regex/spec.md` | 本议题全部决策：定案 A–P、测试接缝、不在范围的事项 |
| 4 | `docs/issues/dangercheck-text-regex/issues/09-integration-and-verification.md` | **你要干的活，勾选清单在这** |
| 5 | `docs/issues/dangercheck-text-regex/audit.md` | 实测底本。第二节是「日常运维命令表」，09 要用 |

规格与工单是事实来源。本文件只补「最新状态、口径、坑」。

---

## 二、现在到哪了

最近提交（`git log --oneline -6`）：

```text
d913caf docs(issues): 更新危险命令检测议题的交接文档（01–08 已完成，只剩 09）
d60a36b docs(readme): 危险命令分析改成按原文匹配，依赖表去掉解析库
60d8b55 feat(dangercheck): 17 条规则按边界表落地，语料 162/162
de24d94 test(dangercheck): 判定层测试不再依赖配置内容，补规则边界表
590cd09 docs(issues): 危险命令检测重写议题补实测底本与语料草稿
1797beb feat(dangercheck): 危险命令检测改按命令原文匹配，规则文件 regex 改数组
```

**工单 01–08 状态都是 `done`；`09` 与 `spec.md` 还是 `ready-for-agent`。**

现状实测（在新会话里重跑一遍确认）：

```bash
cd modules/SSHFleet_Go
export GOTMPDIR=/home/hyl/桌面/Code/SSHFleet/build/atest-1/tmp TMP=$GOTMPDIR TEMP=$GOTMPDIR
go vet ./...                        # 应无输出
go test ./...                       # 应 11 个包全 ok
go test ./internal/dangercheck/ -run TestRuleBoundaries   # 17 条规则的边界
go test ./internal/dangercheck/ -run TestDangerousCorpus  # 语料应 162/162
```

### 代码结构（`modules/SSHFleet_Go/`）

| 文件 | 管什么 |
| --- | --- |
| `internal/dangercheck/judge.go` | `Analyze`（**测试接缝，签名不许变**）、`Match`（`Lines []int` 多行号）、`riskOrder` 四档 |
| `internal/dangercheck/text.go` | 纯文本整备：去注释（保行数）、路径片段归一（保行数，行尾补一个空格给词边界） |
| `internal/dangercheck/rules.go` | 规则加载与校验（定案 N/O）、逐行匹配（**不跨行**） |
| `internal/dangercheck/boundary_test.go` | 17 条规则的「必须命中 / 必须不命中」 |
| `internal/dangercheck/corpus_test.go` | 语料读取（制表符 CSV、带引号处理）+ 逐条比对 |
| `internal/dangercheck/judge_test.go` | 判据测试，**自带规则集、不读配置** |
| `internal/output/danger.go` | 警告框：每块七行、多行号 `行号: 1, 2, 3`、内容超宽截断 |
| `config/keywords_dangerous.conf` | 17 条规则（禁止 6 / 高 5 / 中 6），`regex` 是数组，另有 `reason` |

`parse.go` 已删除，`mvdan.cc/sh` 依赖已摘除。语料在 `modules/SSHFleet_Go/testdata/dangerous_cases.tsv`。

---

## 三、你要做的：09 收口

### 3.1 已经验过的（可直接写进 09 的 Comments）

在新会话里**重跑一遍**再写，别只抄：

```bash
# 1) 造一个可跑的沙盒（不进版本库）
mkdir -p build/atest-1/e2e && cp -r modules/SSHFleet_Go/config build/atest-1/e2e/
cp modules/SSHFleet_Go/config/keywords_dangerous.conf build/atest-1/e2e/config/
cd modules/SSHFleet_Go && go build -o ../../build/atest-1/e2e/SSHFleet .
cd ../../build/atest-1/e2e
# 沙盒的 SSHFleet.conf 里把 encrypt 改成 false，用明文密码造清单，避免凭据预检查拦下来
sed -i 's/^encrypt      = true/encrypt      = false/' config/SSHFleet.conf
printf '10.255.255.1,1,root,x\n' > nodes.csv        # 不可达 IP：只验流程，不真连
```

| 验证 | 命令 | 期望 |
| --- | --- | --- |
| forbidden 拒绝 | `./SSHFleet -c "rm -rf /etc" -f nodes.csv --yes` | 打警告框「禁止」，`echo $?` = **1** |
| high 不阻断 | `./SSHFleet -c "reboot" -f nodes.csv --yes` | 打完整警告框（级别「高」），命令照常下发，`$?` = 0 |
| 多命中行号 | 脚本里三行 `rm -rf /opt/a` `/opt/b` `/opt/c`，`-s` 跑 | 一处块，`行号: 1, 2, 3` |
| 注释不误拦 | 脚本只有 `# rm -rf /` 与 `echo hi # reboot` | **不弹框** |
| 行号定位 | 脚本第 2 行 `rm -rf /etc` | 报 `行号: 2` |
| 日常表 22 条 | 见 3.2 | 0 条落 forbidden；11 条会提醒（下一节有清单） |

### 3.2 日常运维命令表（audit.md 第二节 22 条）的实际结果

0 条落 forbidden。**会提醒的 11 条**，逐条抄下来：

```text
yum remove telnet          → 高 / 强制删包
rpm -e --nodeps oldpkg     → 高 / 强制删包
chmod 777 /tmp/upload      → 中 / 权限修改
chmod u+s /usr/local/bin/x → 中 / SUID·SGID
iptables -F                → 高 / 清空防火墙
userdel -r tempuser        → 中 / 删除账户·用户组
groupdel tempgroup         → 中 / 删除账户·用户组
rm -f /var/log/app.log     → 中 / 删除关键路径文件
shutdown -r +10            → 高 / 系统重启
reboot                     → 高 / 系统重启
init 6                     → 高 / 系统重启
```

**这不是漏改**：这些动作按 04–06 的判决就该看一眼，`reboot` 是从「禁止」专门降到「高」的。09 里「日常表 0 条落 forbidden」已经满足；**要不要再砍这 11 条是开发者的取舍，不要自己砍。** 如果开发者没给新指示，就照现状收口，并在 Comments 里写一句「余 11 条提醒为规则设计使然，开发者 2026-10-19 未要求进一步削减」。

### 3.3 还没做的

1. **09「五、文档与现实一致」**：手册里提到的每个规则名，在 `config/keywords_dangerous.conf` 里都要真实存在。逐个 `grep` 核对。
2. **09「六、语料的可信度」**：抽 5–10 行，读「理由」与期望是否讲得通（`TestCorpusLoads` 已机器校验「理由非空、不含正则字符」）。另外 `corpus_test.go` 里 `TestCorpusLoads` 会打印每条规则覆盖多少条，贴进 Comments。
3. **09「七、收口」**：
   - `docs/issues/dangercheck-text-regex/spec.md` 第 3 行 `Status: ready-for-agent` → `done`
   - `issues/09-integration-and-verification.md` 第 3 行同样改 `done`
   - `CHANGELOG.md` 当天段（`## [10.0.0] - 2026-10-19`）补一段验收记录（只写使用者能感知的，不写测试细节）
   - 删除本文件 `handoff.md`
4. 最后跑一遍全量测试，提交（中文提交信息，`类型(作用域): 简短描述`）。

---

## 三点五、09 之外新增的一块：按最新口味再砍规则

**这是开发者 2026-10-19 傍晚在原对话里明确下的调整，优先级高于 04–06 工单原来的判决。** 动手前必须按规范读 `docs/个人开发规范.md` 并要一句「授权开工」。

### 3.5.1 开发者原话的意思（两条）

1. 「**iptables -F、shutdown -r +10、reboot、init 6 这四个是高危，直接禁止；其他的都属于正常操作，不需要拦截**」。
2. 「reboot 是专门从禁止降到高的，**不能降**」——即 05 工单里「批量重启是正当用途、降到 high」那条判决，被开发者推翻了，回 forbidden。

### 3.5.2 落地口径（照这张表改）

| 动作 | 现在 | 改成 |
| --- | --- | --- |
| `reboot`、`shutdown -r ...`、`init 6` / `telinit 6` | 高 / 系统重启 | **禁止**（重启族整条进 forbidden） |
| `iptables -F`、`ip6tables -F` | 高 / 清空防火墙 | **禁止** |
| `shutdown -c` 等撤销写法 | 不拦 | **仍然不拦**（撤销误操作要能撤销，这条不变） |
| `yum remove`、`rpm -e --nodeps` | 高 / 强制删包 | **删掉这条规则**（正常操作） |
| `chmod 777`、`o+w`、`a+rwx` | 中 / 权限修改 | **删掉这条规则** |
| `chmod u+s`、`4755` 等 | 中 / SUID·SGID | **删掉这条规则** |
| `userdel`、`groupdel` | 中 / 删除账户·用户组 | **删掉这条规则** |
| `rm -f /etc/hosts`、`rm -f /var/log/app.log` 等 | 中 / 删除关键路径文件 | **删掉这条规则** |

**不要自己扩大范围**：开发者点名的只有上表这些。`递归删除`（高）、`屏蔽关键服务`（高）、`停止·禁用服务`（中）他没提，**保持原样**，不要顺手砍也不要顺手升档。

### 3.5.3 取消规则数下限（开发者 2026-10-19 定）

原话：「不设限了，设限那是之前 python 版本那时候的决策，现在已经不适用了，现在都是按需配置了，甚至哪一档是 0 就像 low 那样，都不拦截」。

要做的：

- `internal/dangercheck/rules.go` 删掉 `minRules` 与「规则只启用了 N 条，至少要 N 条」那条报错；
- 删掉 `dangercheck_test.go` 里的 `TestLoadRulesEnforcesMinRules`；
- `spec.md` 与工单里关于「条数下限」「本轮不动 minRules」的说法，按「同一决策的细节修正」就地改，并在 Comments 记一条**取代关系**（谁、哪天、取代了哪一段）。

### 3.5.4 连带要改的东西

- **语料**（`testdata/dangerous_cases.tsv`）：被改成禁止的重启族、iptables 行改期望；被删掉的规则对应的行改成 `none` 或删除，每行都要有理由；
- **边界表**（`boundary_test.go`）：删掉已删规则的条目，重启族与 iptables 改到 forbidden 的断言里；
- **手册**（`docs/1-使用手册.md`、`4-进阶知识手册.md`）：规则条数、级别说明、示例里引用的规则名都要跟着改；
- **CHANGELOG**：10.0.0 段是当天未封口的，就地改描述，不要另起一段。

## 四、已定、别再重问的决策

### 4.1 规格里的定案（详见 `spec.md`）

A `regex` 数组；B 整份原文匹配、不跨行、行号按偏移反算、多命中全列；C 语法解析退役、路径归一保留；D 检测与下发读同一份正文；E 手册同步；F 规则过筛子 38→17；G 本轮只删不增；H 旗标由规则自己列全；I 允许质疑语料、不凑绿；J `mkfs`/`dd` 不分主次盘；K 新增 `reason`（30 显示列上限，超了加载报错）；L 内容超宽截断、框线等宽；M `low` 保留为合法档位、出厂不预置；N 停用规则只校验 `name`；O 加载报错要能定位/能理解/能处置；P 不按机器分组。

### 4.2 开发者口头定过、但规格里没写的（**同等效力**）

这些是实施过程中问出来、开发者拍板的，务必遵守：

| 项 | 结论 |
| --- | --- |
| 授权口径 | 只有「授权开工」四个字算数，一次管一个阶段；01–08 已授权并做完，**09 仍未获授权**，开工前要问 |
| 规则名 | 统一用 **`杀死所有进程`**（audit 里写「杀光进程」，以代码与工单为准） |
| `echo /etc \| xargs rm -rf` | 期望为 `none`，理由：「目标在 rm 前面，纯文本正则不认」 |
| 手册 D2 两处 | 已一并改：`--yes` 下照常打印警告框，只是不问 |
| **刻意绕过不追** | 变量拼路径、命令拆分、`cd /tmp && rm -rf ./*`、参数列表（`subprocess.run(["rm","-rf",...])`）**不要求命中**，接受漏报。开发者的原话意思是：拦得住普遍写法就够了，刻意的各种变形成本太高 |
| **`/var/log` 本身算禁止** | 「批量执行里没有正当删日志目录的场景」；但 `/var/log/app.log` 这类**目标变成文件**的归 medium |
| 判断口径 | 看**目标最终是目录还是文件**，不看路径前缀。`/etc/nginx` → medium，`/root/.ssh` → forbidden（管理密钥） |
| 规则写法 | **枚举可接受**，重点是系统级目录；**不破坏规则文件结构**（不加字段、不做两遍后处理） |
| **规则再砍一轮** | 重启族与 `iptables -F` 进禁止；`yum remove` / 权限修改 / SUID·SGID / 账户删除 / 关键路径文件五条规则整条删掉。详见「三点五」 |
| **取消条数下限** | `minRules` 删掉；某一档可以是 0 条，像 `low` 一样 |

---

## 五、最容易翻车的地方

1. **Go 正则的坑**（这轮踩了很久）：
   - 不支持 `(?!...)` / `(?=...)`，编译直接报错。
   - 集合 `(?:[\s;&|)]|$)` 后面接引号结尾**会匹配失败**。引号要显式放进集合：`[\x22\x27\s;&|)]`。排查 126 条误判时找到的根因就是这个。
   - 长枚举（`(?:etc|usr|var|lib|bin|sbin|boot|var/log|...)`）本身没问题，问题都在收尾的组合上。
2. **TOML 单引号字符串里不能写 `'`**：用 `\x27`、`\x22`，别把引号直接塞进字符类，否则字符串提前截断。
3. **整备文本**：`prepare()` 去注释、归一路径，并在**每行末尾补一个空格**给词边界；行数不变，行号不错位。所以规则里用 `\s` 收尾是安全的。
4. **规则文件**：启用规则数必须 ≥ 10，否则加载失败；停用规则只校验 `name`。
5. **语料是制表符 CSV**：多行脚本用双引号包住整格、内部 `"` 写 `""`；用 `csv.Reader` 读，不要按行切。
6. **测试放哪**：判据测试自带规则集、不读配置；凡与配置相关的断言只放 `boundary_test.go` 和 `corpus_test.go`。改配置不该把判据测试带红。
7. **构建临时目录必须在工作区内**：`GOTMPDIR` / `TMP` / `TEMP` 都指到 `build/atest-1/tmp`。写工作区外会被安全软件静默删，构建莫名失败。
8. **不要碰** `docs/issues/result-category-order/` 与 `internal/output/report.go`（另一个会话在做另一件事）。

---

## 六、新会话开工建议

1. 按「一」读四份材料；
2. 跑「二」里的四条命令确认基线；
3. 向开发者要一句「授权开工」（**09 与「三点五」的规则再调整是两件事，建议分开要授权、分开提交**）；
4. 按 3.1 重跑端到端、按 3.3 收口；
5. 提交并汇报证据（命令 + 实际输出）。

沙盒里已经备好：`build/atest-1/e2e/`（可跑的二进制 + config 副本 + nodes.csv），`build/atest-1/probe/`（第一轮的复现探针）。`build/` 不进版本库，按规范**按天清理**：当天与昨天建的一律留着。
