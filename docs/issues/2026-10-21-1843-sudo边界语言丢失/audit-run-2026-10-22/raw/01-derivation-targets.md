# 轴 1：派生落点（A1 / A4 与第七节「影响」）

**本轴 7 问：已答 7，不可判 1（Q4；Q3 另有一个子项不可判，见 §域外依赖），硬伤 2（H1、H2），判断题 3（J1、J2、J3）。**

- 被审文件：`docs/adr/0024-env穿过sudo边界.md`（411 行）
- 审计日期：2026-10-22
- 授权：只读。本文件是本轴唯一写入；未建探针目录（无需实测即可判的部分已判，需要 sudo 的部分判不可判）。
- 判据基准：A1–A5 五条锚（不改写、不并入普通发现）。
- 环境：Ubuntu 26.10；`sudo` 实测为 **sudo-rs 0.2.14-1ubuntu4**；`sudo -n true` 退出码 1。

---

## 0. 结论速览

| 编号 | 问题 | 结论 |
| --- | --- | --- |
| H1 | Q2 / 决定 1：第七节的落点只点名了**唯一不随包发布**的那份配置 | 【硬伤】工单照写 → 验收通过 → 使用者拿到的仍是 `en_US.UTF-8` |
| H2 | Q7 决定 3：文首「收窄 ADR-0015 影响一节」与决定 3「不收窄正文」互相矛盾 | 【硬伤】同一文件对同一件事两种说法，验收判据取决于以哪句为准 |
| J1 | Q4 / 9.2：`secure_path` 结论无命令、无版本、无输出 | 【判断题】补证据 / 降级标注，二选一 |
| J2 | Q5：`command.go:11-15` 的「强制成同一个 UTF-8」在 A1 之后变成假话 | 【判断题】改注释 / 不改，两个选项的验收都能写 |
| J3 | Q7 决定 4：`PATH`/`TERM` 只能验「字面未变」，不能验「在 sudo 下生效」 | 【判断题】接受弱验收 / 追加真机验证项 |
| — | Q1 / Q3 / Q6 | 该问：未发现问题 |
| — | Q4 主体 | 【不可判（环境）】 |

**锚质疑：未发现。** 五条锚与文件正文、源码、环境事实均无直接冲突（A3 与文首的冲突是**文件内部**矛盾，不是锚与证据冲突，故记 H2，不动锚）。详见 §锚质疑。

---

## 1. Q1：`[env]` 出厂值由哪几份文件决定？谁读哪一份？

### 1.1 决定出厂值的是三份文件，不是一个

```
$ md5sum modules/SSHFleet_Go/config/SSHFleet*.conf
13fd0c15248a6d3d8363c23a56b6c386  modules/SSHFleet_Go/config/SSHFleet.conf
d2e6213352045436b097683fa7574265  modules/SSHFleet_Go/config/SSHFleet-for-linux.conf
cb1af9555973b98db2bed1ab9e4792ad  modules/SSHFleet_Go/config/SSHFleet-for-windows.conf
```

三份 md5 都不同，但差异**只在 `[account]` 与 `[credential]`**（`diff` 命中的行：9–11、31）。`[env]` 段逐字节相同：

```
--- SSHFleet.conf
45:[env]
46:LANG   = "en_US.UTF-8"   # 兜底的语言环境
47:LC_ALL = "en_US.UTF-8"   # 统一字符集与排序（优先级最高，覆盖其余 LC_*）
48:PATH   = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"   # 补一组标准目录（非交互会话的 PATH 常只剩 /usr/bin:/bin）
49:TERM   = "dumb"          # 统一成哑终端，抑制彩色与转义序列

--- SSHFleet-for-linux.conf    （45–49 行，逐字相同）
--- SSHFleet-for-windows.conf  （45–49 行，逐字相同）
```

**判：出厂值有三份来源文件，且 `[env]` 部分完全等价。** 任何「只改一份」的写法都会留下一份旧值。

### 1.2 程序运行时读哪一份

`internal/config/config.go:1`（包注释）：

> `// Package config 承载主干第 1 步：加载 ./config/SSHFleet.conf（TOML）。`

同一文件 `config.go:132-136` 的报错文案把这条钉死：

> `"这个位置没有配置文件\n" + "      应该在这里：%s\n" + ... + "提示：请在本工具的解压目录下执行；配置文件必须放在该目录的 config 文件夹里"`

**判：运行期只读工作目录下的 `./config/SSHFleet.conf`，路径单一、无第二入口。**
旁证（**不在本轴白名单，仅 grep 命中、未展开阅读**）：`main.go:90` `configPath = "./config/SSHFleet.conf"`、`internal/cli/interpreter.go:21` `const configShown = "config/SSHFleet.conf"`。三处指向同一路径，无一支持「从别处再读一份」。

### 1.3 打包后包内是哪一份

`build-linux.sh:99-103`：

```bash
# 可执行文件与出厂配置按平台各取各的
cp -f "$OUT/SSHFleet.exe" "$WPKG/"
cp -f "$CFG/SSHFleet-for-windows.conf" "$WPKG/config/SSHFleet.conf"
cp -f "$OUT/SSHFleet" "$LPKG/"
cp -f "$CFG/SSHFleet-for-linux.conf" "$LPKG/config/SSHFleet.conf"
```

`build-windows.bat:104` / `:143` 同口径（`copy ... SSHFleet-for-windows.conf → PKGDIR\config\SSHFleet.conf`、`SSHFleet-for-linux.conf → LPKGDIR\config\SSHFleet.conf`）。

而工作区那份**明确不随包**，`build-linux.sh:16-17` 原文：

> `#   各自复制成包内的 config/SSHFleet.conf。工作区里那份 config/SSHFleet.conf 是`
> `#   作者本机在用的配置，不随包发布。`

`build-windows.bat:8-9` 同义。另核：两个脚本的公共拷贝段（`build-linux.sh:92-97`）不含 `SSHFleet.conf`。

**判：使用者拿到的是 `-for-linux` / `-for-windows` 两份模板改名后的副本；仓库根的 `SSHFleet.conf` 永远不进包。**

### 1.4 有没有第二来源（默认值常量 / 启动期生成 / 回写）

- 无默认值常量：`config.go:4` 原文 `**所有字段必填**（字段要出现，值可留空），不写死任何默认值：缺字段与非法取值都在这层报错`；`config.go:181` 原文 `预检查通过即代表配置自足，运行期一律取配置里的值`。
- 无 `go:embed`：全仓 `--include="*.go"` 检索 `go:embed` **0 命中**。
- Go 侧无 `en_US` 字面量：全仓 `en_US` 命中里，`.go` 文件仅 `internal/config/config_test.go:11` 与 `internal/ssh/command_test.go:15/57/58/299/354/381`（均为测试夹具，不读真实 conf）。
- 无启动期生成：`[env]` 只做校验与归一（`validateEnv`，`config.go:261-279`），删空键空值、不改写值。
- 无回写：Go 侧的 `os.WriteFile` 命中都在 `internal/credential/*`、`internal/output/*`，无一处写 `SSHFleet.conf`。

**Q1 结论：该问：未发现问题。** 出厂值来源单一、可枚举，就是那三份文件的 `[env]` 段；无代码侧第二来源、无生成、无回写。（`SSHFleet.conf` 的「唯一性」问题留给 Q2。）

---

## 2. Q2：第七节的改动面是否覆盖「出厂值」的全部落点？

### 2.1 原文（`0024:301`）

> `- **配置**：`config/SSHFleet.conf` 的 `[env]` 段，`LANG` 与 `LC_ALL` 两行由 `"en_US.UTF-8"` 改为 `"C"`，注释同步改口（现在的「统一字符集与排序」不再准确，因为 `C` 不提供 UTF-8 字符语义）。`

第七节只点名**一份**文件，而这份正是 1.3 里被两个打包脚本排除在包外的那份。

### 2.2 存在「工单照它写、验收通过、使用者拿到不同值」的路径

**路径（五步，全部有原文支撑）**：

1. 工单按 §7 写 → 只改 `modules/SSHFleet_Go/config/SSHFleet.conf:46-47`（及 47 行注释）。
2. 验收在该文件 `grep` 到 `LC_ALL = "C"` → **通过**。
3. 打包时 `build-linux.sh:101/103`（`build-windows.bat:104/143`）取的是 `SSHFleet-for-*.conf`，与第 1 步改的文件**不是同一个**。
4. 包内 `config/SSHFleet.conf` 的 `[env]` 仍是 `en_US.UTF-8`（三份 `[env]` 段逐字节相同，见 1.1 的 diff 范围）。
5. 使用者解压后运行，`config.go:1/132-136` 让他读的正是包内这一份 → 下发 `export LANG="en_US.UTF-8" LC_ALL="en_US.UTF-8"`，§3.5 要消灭的「未生成 locale 时每次 bash 多 1 行、perl 多 7 行警告」**原样保留**。

**这是一条真路径，不是假想**：两个打包脚本的注释都主动声明「工作区那份不随包」，等于把 §7 指的那份排除在交付物之外。

### 2.3 定性

**【硬伤】**（H1）。理由：判点①（有落点、无悬空承诺）在这一处被破坏——落点写了一个、实际需要三个，且被写的那个**不进入使用者手里**。这不是措辞可权衡的问题：按 §7 逐字执行，ADR 的核心承诺（出厂值改为 `C`）在交付物上**未发生**。

**本条不替议题 spec 出修法**，只报差异。判点③（产物可追）在验收侧的要求另记：§7 没要求「改完看包内」，因此验收只能停在仓库工作区；补验收时需要一条能看见包内容物的命令（例如 `tar -xzOf <linux包> <顶层>/config/SSHFleet.conf | grep -A4 '^\[env\]'` 与 `unzip -p <win包> <顶层>/config/SSHFleet.conf`），否则同类假通过会重演。

---

## 3. Q3：老配置口径能不能判死？旧配置与新出厂值并存时行为写没写清？

### 3.1 原文（`0024:302`）

> `- **老配置**：使用者的既有配置里若仍写着 `en_US.UTF-8`，本 ADR 不影响其行为——取值由配置决定，不是代码写死。是否改随使用者。**不提供自动迁移**。`

### 3.2 「不提供自动迁移」可以判死——源码支持，且三种旧配置形态都可判

- 依据：`config.go:181` `运行期一律取配置里的值`；`config.go:203` 必填项只要求 `{"env", definedStr(md, "env")}` 存在，**不校验 `[env]` 里具体有哪几个键**；`config.go:261-266` 空键/空值按「没有这一条」丢弃。
- 因此旧配置的三种形态行为都唯一可判：
  - 仍写 `en_US.UTF-8` → 原样下发旧值（ADR 承诺成立）；
  - 已写 `C` → 下发 `C`；
  - 删掉 `LANG`/`LC_ALL` 行 → 该条不导出（不报错、不补默认值）。
- 可写出的验收判据：拿一份仍含 `en_US.UTF-8` 的旧配置启动，日志/report 的「命令行」一行应逐字出现 `LANG="en_US.UTF-8" LC_ALL="en_US.UTF-8"`；不出现启动报错、不出现「已自动改为 C」。这条判据不依赖目标机，本地可判。

**判：「不提供自动迁移」这一口径可判死，无悬空承诺。**

### 3.3 「并存」的两种读法：一种写清了，一种没写

- **读法 A：同一份配置文件里旧行与新值并存** —— 不成立（TOML 同段同键重复会解析失败，`config.go:140-143` 走「解析 TOML 失败」分支），且 `[env]` 是 map，键唯一。此读法下无需写。
- **读法 B：使用者的旧文件与包内新模板并存** —— ADR 未写。运行期只有一个 `./config/SSHFleet.conf`（`config.go:1`），**不存在合并、不存在多文件叠加、不存在环境变量覆盖**（全仓 Go 侧无 `Getenv` 读配置路径、无 `SSHFleet.conf` 的 `Setenv` 用例）。所以「并存」实际只剩一个问题：**升级解压时，包内新模板会不会覆盖使用者改过的旧文件**。
- 这个问题取决于交付方式（覆盖解压 / 解到新目录 / 只替换可执行文件），**不在本轴白名单**，且不属 ADR 正文可裁定的事 → 记 §域外依赖，结论「不可判（域外依赖）」。

### 3.4 一个可判但 ADR 没说的点（不单列编号，归入本条）

`0024:302` 说「取值由配置决定，不是代码写死」——这句在源码上成立，但它**同时意味着 §7 只改模板对老用户无效**：老用户升上来若不手改，`en_US.UTF-8` 的警告面照旧。ADR 已明说「是否改随使用者」，所以这不是事实错误；但它与 H1 叠加后，最终效果是「新用户也可能拿到旧值」——两者的差别只在**模板有没有改全**。

---

## 4. Q4：9.2 的 `secure_path` 陈述，依据是什么？

### 4.1 原文（`0024:325-327`）

> `### 9.2 `PATH` 在 sudo 下被 `secure_path` 替换`
> `两个发行版都实测到：工具设的 `PATH` 在 `sudo` 之后被 `secure_path` 覆盖。这意味着「`/usr/local/bin` 里的程序在 sudo 下找不到」是一个**静默的**坑。此事与 locale 同源（都在 `sudo` 边界上），但处置方式不同，本 ADR 不处理。`

### 4.2 依据来源：标注为「实测」，但零可追产物

- 正文给的依据只有「两个发行版都实测到」这一句。**无命令、无 `sudo -V` 版本、无 `/etc/sudoers` 的 `Defaults secure_path=` 原文、无输出片段。**
- 第十一节把可复现材料指向 `build/`（`0024:350`：`沙盒产物（build/ 下，按天清理）`）。本机 `build/` 现存目录最早为 2026-09-29、最新为 10-22，**未见 `i18n` / `locale` / `sudo` 取证目录**（`ls build/` 命中的是 `architecture-review-*`、`atest-*`、`audit-2026-10-22*`）。按天清理意味着即便曾存在也已被清。
- **判点③**：这条结论在「承重」意义上弱（9.2 自述「不在本决策范围内」），但它是决定 4 与 A4 的唯一事实支点，属**支点无产物**。

### 4.3 可判程度：不可判（环境），且比「sudo 坏了」更深一层

两条独立的环境障碍，任一条都足以阻断：

1. **sudo 不能执行任何命令**：
   ```
   $ sudo -n true
   sudo: 无效的配置：/etc 必须属于 root
   $ sudo -n true >/dev/null 2>&1; echo $?
   1
   ```
2. **本机 sudo 不是 ADR 依据的那个实现**：
   ```
   $ sudo -V
   sudo-rs 0.2.14-1ubuntu4
   $ grep -E "^(PRETTY_NAME|VERSION_ID)" /etc/os-release
   PRETTY_NAME="Ubuntu 26.10"
   VERSION_ID="26.10"
   ```
   ADR 的依据是 classic sudo（`0024:74-76`：Ubuntu 26.04 **1.9.17p2** / CentOS 7.9 **1.8.23**）。本机是 **sudo-rs**，且 `sudo -V` **只打印一行版本**，不打印 `env_keep` / `env_check` / `secure_path` 任何一段——即「用 classic sudo 的机制表（`0024:96-97`）去解释」这件事，在 sudo-rs 上连取证入口都没有。

**结论：出厂 `PATH` 在 sudo 下是否生效 → 【不可判（环境）】。** 不得用任何非 sudo 路径冒充（例如在本地 `sh -c 'PATH=…; echo $PATH'` 只能证明 export 生效，证明不了 sudo 之后是否被覆盖）。

### 4.4 要判它，应在目标机（非 root 账号）跑的命令

```bash
# 1) 看实现与配置段（classic sudo 才打印这三段；sudo-rs 只打印版本行）
sudo -V | grep -nE 'secure_path|env_keep|env_check'
grep -rn 'secure_path\|env_keep\|env_check' /etc/sudoers /etc/sudoers.d/ 2>/dev/null

# 2) 用工具真实形态，只过一层 sudo，看 sudo 之后看到的 PATH
sh -c 'export PATH="/opt/probe:$PATH"; sudo sh -c "printf %s \"\$PATH\""'
```

判读：输出以 `/opt/probe:` 开头 → 9.2 不成立（PATH 穿过）；输出等于 `secure_path` 值 → 9.2 成立，并应把该行原文贴进证据。

### 4.5 附带（不判，移交轴 2）

本机 Ubuntu 26.10 的默认 sudo 是 **sudo-rs 0.2.14**，与 `0024:349` 记的「Ubuntu-26.04 使用 sudo 1.9.17p2」不是同一实现。若目标机群里有 Ubuntu 25.10+ 默认装 sudo-rs 的机器，`0024:96-97` 的「两种保留机制」表（`env_keep` / `env_check`）在该实现上可能整段不适用。**三、六节与 3.1/3.2 不在本轴域**，此处只报环境事实，判断归轴 2。

### 4.6 标注诚实度：J1

**J1【判断题】9.2 的标注方式与本 ADR 自己的口径不一致。**

- 现状：`0024:327` 用「实测到」这种断言式措辞，但正文别处凡是实测都给了表格/输出（3.1、3.5、3.8），唯独 9.2 只有一句话。读者无法区分它是「实测」「源码推断」还是「沿用上游 ADR-0015 的说法」。
- 选项 A（补证据）：在 9.2 里补 `sudo -V` 版本、`Defaults secure_path=` 原文、一行 `printf %s "$PATH"` 输出。代价：需要一台 classic sudo 的目标机；本轮做不到，只能标「待补」。
- 选项 B（降级标注）：把「两个发行版都实测到」改为「本轮未复查，依据为早前排查记录（无产物留存）」。代价：9.2 从「已知事实」降为「待验证」，但 A4（PATH 不动）不依赖它成立，所以降级不推翻任何决定。
- **两选项都不影响 A4**：A4 是「保持出厂值不变」，无论 secure_path 成立与否，`PATH` 这一轮都不改。J1 只影响「为什么可以不改」的说服力。

---

## 5. Q5：注释面（`command.go:11-26`、`config.go:104`）要不要同步改？

### 5.1 逐条比对

| 位置 | 原文 | 与 A1 的关系 |
| --- | --- | --- |
| `command.go:11-15` | `//   - locale（LC_ALL / LANG）：…强制成同一个 UTF-8 才可比、可归类。` | **会变成假话**。A1 之后值是 `C`，`C` 不是 UTF-8（ADR §3.4 自己写的「按字节」）。 |
| `command.go:16-19` | `PATH：…sudo（/usr/sbin、/usr/local/bin）与 python3…会 command not found` | 与 A4 无关（A4 不动）；但与 9.2 有张力（见 4.x）：若 sudo 下 PATH 被 `secure_path` 覆盖，这条理由在 sudo 路径上不成立。属 Q4 的遗留，不是 A1 的落点。 |
| `command.go:22` | `具体是哪几条、还能加什么，见 config/SSHFleet.conf 与《进阶配置手册》` | 值中立，不需改。 |
| `config.go:104` | `// 出厂配置给的是统一 locale / PATH / TERM 几条；拼装见 ssh.ExportPrefix。` | 值中立（只说「几条」，不说取值），**不需改**。 |

**判：`config.go:104` 与 A1 无关，改与不改都不影响验收；`command.go:11-15` 与 A1 直接冲突，是 §7 未覆盖的落点。**

### 5.2 J2【判断题】改 / 不改

- **选项 A：改 `command.go:11-15`（建议）**
  - 改法边界（不出修法，只给验收能判的边界）：该段不再出现「UTF-8」这种字符集承诺，改为「统一成 `C`：按字节、无 locale 依赖」这类与 3.4 同义的表述。
  - 验收：`sed -n '9,26p' internal/ssh/command.go | grep -c 'UTF-8'` → 0；且 `git diff` 只触及注释行（无代码行变化）。
  - 代价：注释是 `ExportPrefix` 唯一的设计理由说明，改的时候必须一并说清「为什么不再追 UTF-8」，否则下一个复审者会以为是笔误。
- **选项 B：不改**
  - 验收：无（不改就没有可验收项）。
  - 代价：配置是 `C`、注释说「强制 UTF-8」，两者相反；且 `command.go:22` 把人指向手册，手册若已改，三处（配置 / 代码注释 / 手册）互相矛盾。复审者按注释推断，最容易得出「A1 没落地」的错判。
- 两选项下 `config.go:104` 都不动；若选 B，建议至少把 `command.go:22` 的指向保留（它仍成立）。

---

## 6. Q6：第七节有没有认领「字符语义须写进配置手册」？

**有。** 原文（`0024:304`）：

> `- **字符语义**：`awk` / `cut -c` / `${#s}` / `sort` 在处理中文时按字节（3.7、3.8），须写进配置手册，否则下一个写 `cut -c1-4` 切中文的人会踩到。`

- 认领的是**「须写进配置手册」这件事**，落点声明存在（有「哪一册、哪一节」不写，正是留给实施）。
- 具体能不能指到某册某节，**归轴 5**，本轴不判。

**该问：未发现问题。** 唯一提示（不判）：§7 只认领了「字符语义」这一项新内容，没有认领「手册里现有 `en_US.UTF-8` 表述的清理」；全仓 grep 在 `docs/3-进阶配置手册.md:288`、`docs/4-进阶知识手册.md:40`、`:82` 命中旧值与「强制 UTF-8」的说法。手册属轴 5 域，此处仅记录存在、不作判断、不引用为结论。

---

## 7. Q7：第五节五条决定逐条枚举 + 逐条判「能否落成可验收工单」

按「锚 A1–A5 按顺序一一对应」核对：决定 1↔A1、决定 2↔A2、决定 3↔A3、决定 4↔A4、决定 5↔A5，**顺序与对应关系成立，无错位**。

### 决定 1（`0024:235`）`[env]` 出厂值 `LANG`/`LC_ALL` 改为 `C`

- **能否落成可验收工单：能，但落点按 §7 写会漏两份 → 与 H1 同一处硬伤。**
- 落点（实测，三份）：`SSHFleet.conf:46-47`、`SSHFleet-for-linux.conf:46-47`、`SSHFleet-for-windows.conf:46-47`，加三份的第 47 行注释。
- 判据（可写、可机检）：
  - `grep -rn 'en_US.UTF-8' modules/SSHFleet_Go/config/` → **0 命中**（这一条同时覆盖三份，比逐份 grep 更难作假）；
  - `grep -n '^\(LANG\|LC_ALL\)' modules/SSHFleet_Go/config/*.conf` → 6 行全部 `"C"`；
  - 包内核验：`tar -xzOf <linux包> <顶层>/config/SSHFleet.conf | grep -A4 '^\[env\]'` 与 `unzip -p <win包> <顶层>/config/SSHFleet.conf` 同样 6 行全 `C`（这条是补 §7 缺失的那一环）。
- 悬空检查：判据全部落在仓库内文件与构建产物上，**不依赖目标机、不依赖 sudo** → 无悬空承诺。
- 备注：现有测试不读真实 conf（`grep -rn "_test.go"` 对 `config/SSHFleet` 与 `Load(` **0 命中**），所以这条决定**不能靠 `go test` 验收**，只能靠上述 grep / 包内核验。

### 决定 2（`0024:236`）拼接形态不变

- **只读不判**（归轴 4）。本轴仅记录两处读到的形态，供轴 4 比对，不作结论：
  - 源码形态：`command.go:277-283` `innerCommand` = `envHead(envPrefix) + sudo + interpreter`，外层 `command.go:121/123` `"sh -c " + shellQuote(inner)`；与 A2 的 `sh -c 'export …; <sudo ><解释器> …'` 同形。
  - A2 未提的第三条路径：`command.go:109-116` 的 `--no-shell` 分支完全不设环境、不加 sudo。**不算冲突**（A2 说的是「非 sudo 与 sudo 两条路径」），但「形态不变」的枚举里没有它；归轴 4 判。

### 决定 3（`0024:237`）不收窄、不废除 0015/0016 正文，只各加一行更新记录

- **能否落成可验收工单：理论上能（判据 = 两份 ADR 末尾各出现一条指向 0024 的更新记录，且除该行外两文件零改动），但被本文件内部矛盾卡住 → H2。**
- **H2【硬伤】原文互斥**：
  - 文首 `0024:24`：`> **本 ADR 的地位**：它**收窄** ADR-0015 的「影响」一节，**不推翻**其主体——`
  - 决定 3 `0024:237`：`3. **不收窄、不废除 ADR-0015 与 ADR-0016 的正文**，只各加一行更新记录指向本 ADR。`
  同一件事（0015 的「影响」一节是否被收窄）出现「收窄」与「不收窄」两种说法。派生 spec 时无法判断该不该动 0015 的那段正文：按文首要动，按决定 3 不能动。验收人也无法判「改完之后 0015 的正文是合法的还是违规的」。
- **本条只报差异、不改写**：以哪句为准不是本轴的裁定；`0015/0016` 的正文与更新记录归轴 4。
- 提示（不判，仅移交）：全仓 grep 命中 `docs/adr/0015-远端环境前缀进配置.md:69` 已存在一条 2026-10-21 的更新记录（提到「由 `en_US.UTF-8` 改为 `C`」）。该文件属轴 4，本轴未展开阅读。

### 决定 4（`0024:238`）`PATH` 与 `TERM` 保持出厂值不变

- **能否落成可验收工单：能，但只能验到「字面未变」→ J3。**
- 判据（可写）：
  - `git diff -- modules/SSHFleet_Go/config/` 只应出现 `LANG`/`LC_ALL` 两行 + 一行注释；`PATH`/`TERM` 两行**不得出现在 diff 里**；
  - 逐字节锚点（现状原值）：`PATH   = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"`、`TERM   = "dumb"`，三份都要保持；
  - 顺带一条反向判据：`grep -rn '^TERM' modules/SSHFleet_Go/config/*.conf` → 3 行全 `"dumb"`（防「顺手统一改成 `xterm-256color`」）。
- **J3【判断题】验收强度**
  - 选项 A（接受弱验收）：判据止于「字面未变 + 不在 diff 里」。代价：完全验不出 `PATH` 在 sudo 下是否真的生效（9.2 的事），「保持出厂值」被验成「保持字面」，而 A4 的原意包含「明知它可能无效也先不动」。
  - 选项 B（追加一条真机验证项）：加「目标机非 root 账号跑 4.4 的两条命令，记录 `PATH` 是否穿过 sudo」。代价：需要目标机与可用 sudo（本轮不具备），且这条验证**不改变**决定 4 的结论，只是把 9.2 从「无产物」变成「有产物」。
  - 两选项都不改写 A4；差别只在工单里有没有「待补证据」这一项。

### 决定 5（`0024:239`）乱码不作为本决策的处置对象

- **能否落成可验收工单：能，形式是「反向验收」（验没有改动）。**
- 判据（可写）：
  - `git diff --stat` 不触及采集/输出/码页相关文件（`outputtap.go`、`terminal-output.txt` 写出路径、xlsx 写出路径）；
  - 全仓 `grep -rn 'chcp\|codepage\|SetConsoleOutputCP\|936\|65001' --include="*.go"` 仍为 0 命中（与 §4 一致，若工单期间出现命中即违约）；
  - 第四节保留（声明不被删）。
- **该问：未发现问题。** 否定式决定可验收，判据不依赖目标机。

---

## 8. 锚质疑

**未发现。** 逐条核对过程与理由：

| 锚 | 与被审文件 / 源码 / 环境 / 判点比对 | 冲突？ |
| --- | --- | --- |
| A1 `LANG`/`LC_ALL` → `C` | 与决定 1（`0024:235`）同义；文首 `0024:26` 同义；源码无写死值（`config.go:4/181`），改配置即可落地 | 无 |
| A2 拼接形态不变 | 与决定 2（`0024:236`）、`0024:28` 一致；源码 `command.go:277-283` 同形 | 无（`--no-shell` 是第三路径，不是反证，归轴 4） |
| A3 不改 0015/0016 正文、只加更新记录 | 与决定 3（`0024:237`）一致 | 无。**注意**：与文首 `0024:24`「收窄」冲突，但那是文件内部矛盾，不是锚与证据冲突 → 记 H2，不改锚 |
| A4 `PATH`/`TERM` 不动 | 与决定 4（`0024:238`）、§7（无 PATH/TERM 条目）一致 | 无。9.2 只说明「可能无效」，不构成「应改」的反证 |
| A5 乱码不处置 | 与决定 5（`0024:239`）一致；第四节是说明而非处置 | 无 |

**本轴没有发现需要动用「锚质疑」出口的证据。** Q4 的两处问题（依据无产物、环境不可判）落在「9.2 的支撑强度」，不落在任何一条锚上。

---

## 9. 域外依赖

### 9.1 D1：升级解压时「包内新模板是否覆盖使用者旧文件」

- **问题编号**：Q3 子项（读法 B）。
- **缺哪个文件**：交付/升级说明（README 的升级步骤、手册里的「如何升级」一节）、或发布流程文档。
- **为什么白名单不够**：白名单只给了三份 conf、两个打包脚本、`internal/config/*`、`command.go:1-314` 与 `issue-tracker.md`。打包脚本只说明「包内放哪一份」，不说明「使用者怎么解压、覆盖不覆盖」；`internal/config` 只说明「运行期读一个路径」。覆盖与否是**部署动作**，不是代码或配置的属性。
- **结论**：该子项记「**不可判（域外依赖）**」。不自行扩域，不读 README / 手册 / 发布文档。
- **不影响主结论**：Q3 主问题（「不提供自动迁移」能否判死）已判为可判死（3.2）。

（无其他域外依赖。Q4 的阻碍是**环境**不是文件，按「不可判（环境）」记，不列入本节。）

---

## 10. 四个判点的落点对照

| 判点 | 本轴发现 |
| --- | --- |
| ① 每条决定能否落成可验收工单（有判据、有落点、无悬空承诺） | 决定 1 落点不全（H1）；决定 3 验收判据被文内矛盾卡住（H2）；决定 1/4/5 的判据本身可写、且不依赖目标机 |
| ② 理由链里有没有全称化过头的事实主张 | `0024:327`「两个发行版都实测到」（9.2）在无命令/无版本/无输出下使用全称口吻（J1）；`0024:301` 用单数文件名指代「配置」这一改动面，是**以偏概全的落点**（H1） |
| ③ 承重的量化结论有没有产物可追 | 9.2 **无产物**（`build/` 按天清理，现存无对应目录）；决定 4 的验收若只到字面，则「PATH 在 sudo 下无效」永远无产物（J3） |
| ④ 标注是否诚实 | 9.2 未标「实测 / 源码推断 / 未验证 / 沿袭上游」（J1）；§7 未标「三份文件」这一事实（H1）；注释面 `command.go:11-15` 在 A1 之后成为未标注的假陈述（J2） |

---

## 11. 证据附录（本轮实跑，原样粘贴）

```
$ md5sum modules/SSHFleet_Go/config/SSHFleet*.conf
13fd0c15248a6d3d8363c23a56b6c386  modules/SSHFleet_Go/config/SSHFleet.conf
d2e6213352045436b097683fa7574265  modules/SSHFleet_Go/config/SSHFleet-for-linux.conf
cb1af9555973b98db2bed1ab9e4792ad  modules/SSHFleet_Go/config/SSHFleet-for-windows.conf

$ diff modules/SSHFleet_Go/config/SSHFleet.conf modules/SSHFleet_Go/config/SSHFleet-for-linux.conf
9,11c9,11        # port / user / password
31c31            # secret_dir
（无其他差异 → [env] 段三份完全一致）

$ sudo -V
sudo-rs 0.2.14-1ubuntu4

$ sudo -n true
sudo: 无效的配置：/etc 必须属于 root
$ sudo -n true >/dev/null 2>&1; echo $?
1

$ grep -E "^(PRETTY_NAME|VERSION_ID)" /etc/os-release
PRETTY_NAME="Ubuntu 26.10"
VERSION_ID="26.10"

$ grep -rn --include="*.go" -E "go:embed" modules/SSHFleet_Go/     # 0 命中
$ grep -rn --include="*_test.go" -E "config/SSHFleet|Load\(" modules/SSHFleet_Go/   # 0 命中
$ grep -rn "en_US" . | grep -v build/ | grep -v release/ | grep -v .git/
（.go 命中仅 config_test.go:11、command_test.go:15/57/58/299/354/381；配置命中仅三份 conf:46-47）
```

引用到的源码 / 脚本行（本轴白名单内）：
`config.go:1`、`:4`、`:104`、`:132-136`、`:140-143`、`:181`、`:203`、`:261-266`；
`command.go:11-26`、`:109-116`、`:121-123`、`:277-283`；
`build-linux.sh:16-17`、`:92-97`、`:99-103`；`build-windows.bat:8-9`、`:104`、`:143`。

不在白名单、仅 grep 命中未展开阅读：`main.go:90-91`、`internal/cli/interpreter.go:21`、`docs/adr/0015-…md:69`、`docs/3-进阶配置手册.md:288`、`docs/4-进阶知识手册.md:40/82`、`command_test.go`（归轴 4）。
