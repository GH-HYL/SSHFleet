# 轴 2 回执：结论边界（解释器与失效形态）

**台账**：本轴 8 问：已答 8；不可判 2（U1 表内 8 条解释器、U2 classic sudo 机制，均为部分项）；硬伤 5（H1–H5）；判断题 7（J1–J7）。

**授权边界**：只读分析。除本回执外，未改任何文件；探针脚本与输出仅落 `build/audit-2026-10-22-axis2/`。未 `git add` / `commit` / 删除文件。

**结论口径汇总**（按四种口径，不凑数）：

- **硬伤 H1–H5**：perl 行数 7→19；"永远按字节"被 python/node 推翻；3.7 的 awk/wc 行按实现分档；3.4 "未生成→回落"在 uutils 上不成立；"永远成功/永远安静"字面被 Python<3.7 与 perl `use utf8` 打穿。
- **判断题 J1–J7**：bash 警告文本后缀；6.2 的"ls 逐字节"范围；6.3 的 SUSE/信创未验证；6.5 的自评；决定 1 的选项；决定 2 的选项；3.2 机制列未标证据形态。
- **不可判 U1–U2**：表内 8 条解释器缺失；classic sudo / `env_check` 边界不可复现。另有 U3（量化产物无持久化）记在判点③。
- **未发现问题（明写）**：Q1 的六解释器 I/O 形态（写中文、读中文名、输出非 ASCII、退出码 0、字节原样）；Q2 的外部命令 0 行（ls/free/df/awk/grep/sort/sed/cut/wc/tr/date/id）；Q3 的 `cut -c`、`grep -o .`、`[[:alpha:]]`/`[^[:print:]]`、`sed`/`tr` 行；Q6 的 3.3 行 1–2 与行 3 机制；Q7 的 U17 约束与 `interpreterTable` 非 shell 条目；Q8 的决定 2 "不改"本身。

---

## 0. 方法与环境（本轮 12:50–13:00 亲测）

所有探针用 `env -i` 清空环境，只显式给 `LANG` / `LC_ALL` / `PATH` / `TERM` / `HOME`，stdout 与 stderr 分开落盘，字节用 `od -An -tx1` 比对。命令与原始输出见文末产物索引。

| 项 | 本机实测 |
| --- | --- |
| libc | glibc 2.44（Ubuntu GLIBC 2.44-1ubuntu2；不声称是 26.04 真机） |
| bash | 5.3.9(1)-release |
| `/bin/sh` | → `/usr/bin/dash`（本机；CentOS 7 是 → bash，本轮无该机） |
| dash | 无 `--version` 输出；以 `/usr/bin/dash` 实体存在 |
| python3 | 3.14.8 |
| perl | 5.42.3 |
| node / nodejs | 24.21.0（两个名字指向同一二进制 `/usr/bin/node`） |
| awk | mawk 1.3.4 20260302（`gawk` 未安装） |
| coreutils | **uutils 0.12.0**（`cut` / `sort` / `ls` / `wc` / `tr` 均同版本） |
| grep / sed | GNU grep 3.12 / GNU sed 4.9 |
| sudo | **sudo-rs 0.2.14-1ubuntu4**，不是 classic sudo；运行被拒：`sudo: 无效的配置：/etc 必须属于 root` |
| locale -a | `C` / `C.utf8` / `POSIX` / `zh_CN.utf8` / `zh_SG.utf8`，**无 `en_US.UTF-8`** |
| locale-archive | `localedef --list-archive` 只有 `zh_CN.utf8` / `zh_SG.utf8`；`C` 不在其中，`C.utf8` 是 `/usr/lib/locale/C.utf8` 目录，也不在 archive |

> 版本注记：ADR 9.3 写 Ubuntu 26.04 是 uutils **0.8.0**，本机是 **0.12.0**。两者可能有行为差；下文凡依赖 uutils 的结论都写明版本，不外推到 0.8.0。

---

## Q1. `LC_ALL=C` 下六种解释器写中文、读中文文件名、输出非 ASCII 的形态与退出码

**总答**：六种解释器在 `LC_ALL=C` 下**写中文、读中文文件名、输出非 ASCII 全部成功，退出码 0，stderr 0 行，输出字节与 `zh_CN.utf8` 逐字节相同**。C 不破坏 I/O；差别只在"长度/字符语义"这一层，且各解释器不同。

实测表（`probe-locale.log`）：

| 解释器 | 写中文 | 读中文文件名 | 输出非 ASCII | 退出码 | stderr | 长度语义 |
| --- | --- | --- | --- | --- | --- | --- |
| bash 5.3.9 | 成功，字节原样 | `cat 中文文件.txt` 成功 | 原样 | 0 | 0 | `${#s}` = 16（字节），UTF-8 下 = 8（字符） |
| sh（→dash） | 成功 | 成功 | 原样 | 0 | 0 | `${#s}` = 16，UTF-8 下仍 16 |
| dash | 成功 | 成功 | 原样 | 0 | 0 | 同上；dash 不做多字节 |
| python3 3.14.8 | 成功 | `open` / `listdir` 成功 | 原样 | 0 | 0 | `len("中文")` = 2（Unicode 码点），与 locale 无关 |
| perl 5.42.3 | 成功 | `open "<:raw"` 成功 | 原样 | 0 | 0 | `length("中文")` = 6（字节），UTF-8 下仍 6 |
| node 24.21.0 | 成功 | `readFileSync` 成功 | 原样 | 0 | 0 | `"中文".length` = 2（UTF-16 码元），与 locale 无关 |

关键原始输出（`probe-locale.log`）：

```text
===== bash-C =====
exit=0 stdout_bytes=36 stderr_bytes=0 stderr_lines=0
中文
中文内容
len=16 bytes=16
===== bash-zh_CN.utf8 =====
len=8 bytes=16
===== python3-C =====
len= 2
utf8_mode= 1 fsenc= utf-8 stdout= utf-8
listdir= ['中文文件.txt', 'plain.txt']
===== perl-C =====
len=6
setlocale=C
===== node-C =====
len=2
```

写中文文件名也全部成功（`probe-write.log`）：bash `printf x > 中文新.txt`、python3 `open("中.py.txt","w")`、perl `open(">","中.perl.txt")`、node `writeFileSync` 均退出 0，`ls` 的 `od` 字节与 `zh_CN.utf8` 相同。

**但"成功"按解释器版本分档（H5 的一部分）**：Python < 3.7 没有 PEP 538/540，`LC_ALL=C` 下 `sys.stdout.encoding` 是 ASCII，打印非 ASCII 直接抛异常。本机用 3.14 关闭 PEP 538/540 复现：

```text
$ env -i LC_ALL=C PYTHONUTF8=0 PYTHONCOERCECLOCALE=0 python3 -c 'print(chr(0x4e2d)+chr(0x6587))'
stdout_enc= ascii
UnicodeEncodeError: 'ascii' codec can't encode characters in position 0-1: ordinal not in range(128)
exit=1

$ ... python3 -c 'open("中文文件.txt")'
UnicodeEncodeError: 'ascii' codec can't encode characters in position 0-3: ordinal not in range(128)
exit=1
```

同一份 3.14 默认（UTF-8 模式）下，`stdout_enc=utf-8`、打印成功、退出 0。故：**`C` 下 Python 的形态取决于版本，`interpreterTable` 的 `python` 条目（`python2` / `python3.6` 经 `InterpreterBaseName` 都归一到 `python`）没有按版本分档**。Python 2.7 / 3.6 本体本轮不可判（环境无该二进制），但机制已复现。

---

## Q2. `LC_ALL=en_US.UTF-8` 而该 locale 未生成：各解释器 stderr 行数与内容

实测（`probe-locale.log`、`probe-extra.log`、`probe-python-exc.log`）：

| 解释器 | stderr 行数 | 内容 |
| --- | --- | --- |
| bash 5.3.9 | **1** | `bash: warning: setlocale: LC_ALL: cannot change locale (en_US.UTF-8): No such file or directory` |
| sh（→dash） | 0 | 空 |
| dash | 0 | 空 |
| python3 3.14.8 | 0（启动时） | 空；但见下"用户代码调用 setlocale" |
| perl 5.42.3 | **19** | 见下完整 19 行 |
| node 24.21.0 | 0 | 空 |
| `locale` 命令（参照） | **3** | `Cannot set LC_CTYPE / LC_MESSAGES / LC_ALL to default locale: No such file or directory` |
| `ls`/`free`/`df`/`awk`/`grep`/`sort`/`sed`/`cut`/`wc`/`tr`/`date`/`id` | 0 | 空（用 dash 外层隔离；用 bash 外层会多出 bash 自己那 1 行） |

**perl 的实际 19 行**（不是 ADR 说的 7 行）：

```text
 1  perl: warning: Setting locale failed.
 2  perl: warning: Please check that your locale settings:
 3      LANGUAGE = (unset),
 4      LC_ALL = "en_US.UTF-8",
 5      LC_CTYPE = (unset),
 6      LC_NUMERIC = (unset),
 7      LC_COLLATE = (unset),
 8      LC_TIME = (unset),
 9      LC_MESSAGES = (unset),
10      LC_MONETARY = (unset),
11      LC_ADDRESS = (unset),
12      LC_IDENTIFICATION = (unset),
13      LC_MEASUREMENT = (unset),
14      LC_PAPER = (unset),
15      LC_TELEPHONE = (unset),
16      LC_NAME = (unset),
17      LANG = "en_US.UTF-8"
18      are supported and installed on your system.
19  perl: warning: Falling back to the standard locale ("C").
```

**"0 行"不等于"没有失效"**：

- python3 启动静默，但用户代码一调 `locale.setlocale(locale.LC_ALL, "")` 就抛 `locale.Error: unsupported locale setting`；未捕获时 **7 行 traceback + 退出码 1**（`probe-python-exc.log`）。ADR 3.5 只数"启动警告行"，测不到这类失败。
- perl 的 `setlocale(LC_ALL,"")` 返回 `undef`（C 下返回 `"C"`）。启动那 19 行是可见的，但如果将来用 `PERL_BADLANG=0` 之类抑制，`undef` 就是静默失败。
- `locale` 命令 3 行、退出码 0；靠退出码也判不出来。

**外层 shell 的加行效应（推断，无 CentOS 实测）**：工具形态是 `sh -c 'export …; <sudo ><解释器> …'`（ADR 6.4 自述）。本机 `/bin/sh` = dash → 外层 0 行；CentOS 7 的 `/bin/sh` = bash → 外层本身加 1 行，**即使内层是 python/node 这种自己不报警的解释器**。3.5 的表只有 "bash" 列，没有区分"外层 sh 是 bash"与"配置的解释器是 bash"；6.1 的"取决于它的 `/bin/sh` 是谁"提到了，但 3.5 未量化。

**H1（硬伤）**：3.5 第 155/157 行写 perl "**7** 行"且"构成是固定的"，6.1 第 249 行写"每次 perl 多 **7** 行"。实测 perl 5.42.3 是 **19 行**，构成随 `LC_*` 类别数变化，不是固定 7 行。ADR 的两个目标发行版会给出不同数字；"7 行 = 旧版 perl（如 CentOS 7 的 5.16）形态"是**推断**（CentOS 7 的 5.16 本体本轮未测）。实际输出见上。

**J1（判断题）**：ADR 第 162 行引用的 bash 警告是 `… cannot change locale (en_US.UTF-8)`，本机 bash 5.3.9 实际多一个后缀 `: No such file or directory`。选项：(a) 保留引用、标注 bash 版本；(b) 改成"形如 …（版本不同后缀可能不同）"。代价都很小；不改则逐字复现会对不上。

---

## Q3. 3.7 代价表与 `interpreterTable`（15 条）的覆盖矩阵

先给结论：**3.7 的表没有一行是直接针对解释器的**。表的行是 `awk` / `cut` / `${#s}` / `grep -o .` / `wc -m` / `sort` / `[[:alpha:]]` / `sed` / `tr`——其中只有 `${#s}` 是 shell 语言特性，其余是用户脚本里调用的外部命令。`interpreterTable` 的 15 条里，**3.5 明确点名只有 bash 与 perl 两条（2/15）**；3.7 直接点名 0 条；6.1 的"永远按字节"名义上覆盖 15 条，实测成立/不成立/不可判如下。

### 3.1 覆盖矩阵（行 = `interpreterTable` 15 条）

| # | 条目（command.go:324–338） | 本机可用 | 3.5 警告行数覆盖 | 3.7 代价表覆盖 | 6.1"永远按字节" | 本机实测 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | bash | 是 | **是**（1 行） | **是**（`${#s}` 行） | 名义覆盖 | **成立**：C 16 字节 / UTF-8 8 字符 |
| 2 | sh | 是（→dash） | 否 | 名义（`${#s}`） | 名义覆盖 | **成立**，但 dash 恒按字节，非 C 所致；0 行警告 |
| 3 | dash | 是 | 否 | 名义（`${#s}`） | 名义覆盖 | 同上 |
| 4 | ksh | **否** | 否 | 名义 | 名义覆盖 | **不可判（环境）** |
| 5 | zsh | **否** | 否 | 名义 | 名义覆盖 | **不可判（环境）** |
| 6 | mksh | **否** | 否 | 名义 | 名义覆盖 | **不可判（环境）** |
| 7 | ash | **否** | 否 | 名义 | 名义覆盖 | **不可判（环境）** |
| 8 | python（python3 归一到此） | 是（3.14.8） | 否 | 否 | 名义覆盖 | **不成立**：`len=2`，UTF-8 模式；<3.7 在 C 下直接 exit 1 |
| 9 | luajit | **否** | 否 | 否 | 名义覆盖 | **不可判（环境）** |
| 10 | lua | **否** | 否 | 否 | 名义覆盖 | **不可判（环境）** |
| 11 | node | 是（24.21.0） | 否 | 否 | 名义覆盖 | **不成立**：`"中文".length=2`，与 locale 无关 |
| 12 | nodejs | 是（同一二进制） | 否 | 否 | 名义覆盖 | 同 node：**不成立** |
| 13 | perl | 是（5.42.3） | **是**（7 行，实测 19） | 否 | 名义覆盖 | `length=6` 两 locale 同；警告 19 行 |
| 14 | ruby | **否** | 否 | 否 | 名义覆盖 | **不可判（环境）** |
| 15 | php | **否** | 否 | 否 | 名义覆盖 | **不可判（环境）** |

计数：3.5 明确覆盖 **2/15**（bash、perl）；3.7 直接覆盖 **0/15**（`${#s}` 只对 7 条 shell 名义适用，实测只验了 bash）；6.1 名义 **15/15**，实测成立 4 条（bash/sh/dash/perl）、不成立 3 条（python/node/nodejs）、不可判 8 条（ksh/zsh/mksh/ash/luajit/lua/ruby/php）。

**空白点**：8 条非 shell 解释器（python、luajit、lua、node、nodejs、perl、ruby、php）的**语言内建字符串语义**（`len` / `length` / `.length` / `strlen` / `#`）在 3.7 表里一行都没有。ADR 第 304 行说"须写进配置手册，否则下一个写 `cut -c1-4` 切中文的人会踩到"——手册只防住了 shell + coreutils 用法，没防住 Python/Perl/Node 用户。

### 3.2 3.7 每一行按实现的成立情况

| 3.7 行 | ADR 声称 | 本机实测（实现/版本） | 判定 |
| --- | --- | --- | --- |
| `awk length / substr / index` | 处理中文**不同** | **mawk 1.3.4**：C 与 zh_CN 都是 `length=9`、`substr(1,2)` 都是 `e4 b8`（半个字） | **不成立（mawk）**；gawk 未安装，不可判 |
| `cut -c` | 处理中文**不同** | **uutils 0.12.0**：C → `e4 b8`（2 字节）；zh_CN → `e4 b8 ad e6`（2 字符） | **成立** |
| `${#s}` | 处理中文**不同** | bash 5.3.9：C 16 / UTF-8 8；dash：16 / 16 | **只对 bash 成立**；dash 不成立；其余 shell 不可判 |
| `grep -o .` | 处理中文**不同** | GNU grep 3.12：C → 6 行单字节；zh_CN → 2 行 | **成立** |
| `wc -m` | 处理中文**不同** | **uutils 0.12.0**：C → 3；zh_CN → 3 | **不成立（uutils）**；GNU 未安装，不可判 |
| `sort`（无 `-n`） | 处理中文"码点序，一致" | uutils 0.12.0：C → 张李王（码点）；zh_CN → 李王张（拼音） | **zh_CN 下不成立**；ADR 比较的是 C vs en_US（未生成），该对比**不可判** |
| `[[:alpha:]]` / `[^[:print:]]` | 处理中文**判定会变** | GNU grep 3.12：C 下中文不命中 alpha、命中 non-print；zh_CN 下相反 | **成立** |
| `sed` / `tr` / `grep -c .` | 一致 | GNU sed 4.9 / uutils tr 0.12.0：两 locale 字节相同 | **成立** |

**H3（硬伤）**：3.7 的 `awk` 与 `wc -m` 两行不是 locale 的性质，是**实现**的性质。mawk 根本不做多字节，`awk` 行在 Debian/Ubuntu 默认 awk 上不成立；uutils 的 `wc -m` 在 C 下也按字符数，`wc -m` 行在 Ubuntu 26.04（ADR 9.3 自己说它是 uutils）上不成立。ADR 的表没有"实现"这一列，与 9.3 的自述内部冲突。原始输出见 `out-commands/commands.log`。

**H4（硬伤，与 Q2/Q4 交叠）**：3.4 第 136 行说 `en_US.UTF-8`"未生成时**告警并回落**"。实测 uutils 0.12.0 的 `cut` 在 `LC_ALL=en_US.UTF-8`（未生成）下**不回落**：`cut -c1-2` 仍输出 `中文` 两字符（`e4 b8 ad e6 96 87`），且 0 行告警；`xx_YY.UTF-8`、`en_US` 这类根本没生成的 locale 名也一样按多字节处理。uutils 只看 locale 名是否 `C` / `POSIX`，不校验存在——9.3 已经为 `sort` 记下这条，但它同样适用于 `cut`。GNU grep 则确实回落（C 与未生成 en_US 都是 1 行命中）。所以"未生成→回落"是 **GNU/glibc 系工具**的形态，不是 uutils 的形态。

**J2（判断题，与 3.8 交叠）**：3.7 说 `sort` 处理中文"码点序，一致"。本机 zh_CN 下 uutils 给的是拼音序（李王张），C 下是码点序（张李王）。ADR 比较的是 C vs **en_US.UTF-8**（不是 zh_CN），而 en_US 本机未生成，故该对比不可判；但"码点序"作为一般陈述不成立于真实中文 locale。选项：(a) 把该格限定为"仅比较 C 与 en_US.UTF-8，两者对 CJK 都近似码点"；(b) 直接删掉"一致"二字，标"未验证"。

---

## Q4. 3.4 "C 的失效形态是不存在" 与 6.1 四句"永远"逐条实测

### 4.1 3.4 的"存在性/失效形态"

| 断言 | 实测 | 判定 |
| --- | --- | --- |
| `C` 100% 存在、glibc 编译内置、不在 `locale-archive` | `localedef --list-archive` 只有 `zh_CN.utf8` / `zh_SG.utf8`；`locale -a` 有 `C` / `POSIX`；`setlocale(LC_ALL,"C")` 返回 `"C"`，`"POSIX"` 也返回 `"C"`，`en_US.UTF-8` 返回 `NULL` | **成立**（glibc 2.44） |
| `C` 零警告 | 六种解释器在 C 下 stderr 全 0 行 | **成立**（限 locale 相关警告） |
| `C.UTF-8` glibc 2.35+ 才有；CentOS 7（2.17）没有 | 本机 2.44 有 `/usr/lib/locale/C.utf8`；2.17 无环境 | 本机部分成立；CentOS 7 部分**不可判（环境）** |
| `en_US.UTF-8` 未生成时"告警并回落" | GNU grep/bash/perl 回落；**uutils cut 不回落**（仍按字符、无告警） | **按实现分档，H4** |

`C` / `POSIX` 不在 archive、但 `locale -a` 会列出它们——这两件事不矛盾；ADR 的"不存在"应读作"不存在未生成这种失效"，不是"不在 `locale -a` 输出里"。本轮按前者判成立。

另：3.4 的"`C.UTF-8` glibc 2.35+ 才有"是 upstream 时间线；据 Debian 打包历史，Debian 系在更早版本以补丁提供 `C.UTF-8`（本轮无产物核，仅作线索），本机 2.44 无法区分该差异，故不作为硬伤，记**不可判（域外/无产物）**。这条与 6.3 的排除理由相关：若"2.35+"不成立，`C.UTF-8` 在 Debian 系的可获得性比 ADR 说的更早，但"要工具去改目标机 / 跨发行版不统一"两条理由仍独立存在。

### 4.2 6.1 四句"永远"

| 句子 | 实测 | 判定 |
| --- | --- | --- |
| **永远成功** | `setlocale("C")` / `("POSIX")` 恒非 NULL；但 Python <3.7 在 C 下打印中文 `exit=1`（模拟复现） | **作为 locale 加载成立；作为用户脚本结果不成立（H5）** |
| **永远安静** | 六种解释器 C 下 stderr 0；但 `perl -Mutf8 -e 'print "中文"'` 在 C 下仍出 1 行 `Wide character in print`（与 locale 无关） | **locale 相关警告成立；字面全称不成立（H5）** |
| **永远是英文** | `dpkg -s nosuchpkg`：C → 英文；zh_CN → 中文。`free -m`：C → 英文表头；zh_CN → 中文表头 | **成立**（有翻译包时 C 回退英文；无翻译包时任何 locale 都英文） |
| **永远按字节** | python3 `len("中文")=2`；node `"中文".length=2`；uutils `wc -m` 在 C 下也是 3；perl `length=6` 两 locale 同（字节，但非 C 所致）；bash/dash 字节 | **不成立（H2）**；仅 bash/perl 这类字节语义解释器成立 |

**H2（硬伤）**：6.1 第 251 行"`C` 则永远成功、永远安静、永远是英文、永远按字节"是四句全称。前两句在"locale 相关"范围内成立，后两句作为字面全称被 python / node / uutils `wc -m` 直接推翻。尤其 python 与 node 都在 `interpreterTable` 内（第 331、334–335 行），不是表外用户程序。

**H5（硬伤）**：`永远成功` / `永远安静` 也被两类实测打穿——Python <3.7 的非 ASCII 输出退出 1；perl `use utf8` 在 C 下仍 1 行 stderr。前者是决定 1 可能引入的**行为回归**，不只是"警告换语义"的代价；后者说明"安静"不是 C 的属性。

---

## Q5. 本机缺失的解释器：一律不可判（环境），不外推

缺失：`ksh` / `zsh` / `mksh` / `ash` / `luajit` / `lua` / `ruby` / `php`（8 条）。`nodejs` 与 `node` 是同一二进制，已测。

对这 8 条，本回执不给"应当按字节/按字符"的结论。可以说的只有表内事实（`command.go` 的 `programArg`）：`luajit`/`lua`/`node`/`nodejs`/`perl`/`ruby` 用 `-e`，`php` 用 `-r`，`ksh`/`zsh`/`mksh`/`ash` 用 `-c`、`shell=true`。这些是入口形态，不是 locale 语义。**U1**：不得把"Lua 字符串是字节数组""Ruby 默认 UTF-8"这类语言常识写成实测结论；本轮没有二进制，全部记不可判（环境）。

另：Python 2.7 / 3.6 也没有二进制，Q1 的版本分档是"机制复现"，目标版本本体记不可判（环境）。

---

## Q6. 3.2 的两个发行版结论、3.3 的三行观测：证据形态与可复现性

### 6.1 证据形态（按 ADR 12.1 第 3 条的标准）

| 位置 | 声称 | 自身标注的证据形态 | 产物 | 判定 |
| --- | --- | --- | --- | --- |
| 3.2 CentOS 7.9 机制 | 红帽 `/etc/sudoers` 的 `env_keep`（含 `LANG`/`LC_ALL`/`LC_ALL`）+ `env_check` | **未标注**（读起来像看了 sudoers 文件，但没说"实测/源码/文档"） | 无文件名 | **J7**：未按 12.1 第 3 条标注 |
| 3.2 Ubuntu 26.04 机制 | **仅靠** `env_check`，`/etc/sudoers` 里生效的 `env_keep` 一条都没有 | **未标注**；"一条都没有"是单机缺席断言 | 无 | **J7**：同上；"仅靠"只覆盖一台机器 |
| 3.2 CentOS 5/6 | 1.6.9 / 1.7.x 硬编码保留，1.8.5 起删除 | **明确"源码推断"**（第 110 行；12.1 第 3 条再次点名） | 无 | **诚实**，这是全文标注最好的地方 |
| 3.2 `env_check` 边界 | `%` / `/` 触发丢弃；`a b`/`a$b`/`a:b`/`a.b`/`C` 存活 | "实测边界（CentOS 7.9）" | 无 | 本机**不可判（U2）** |
| 3.3 三行 | 有翻译包 + zh_CN → 中文；同机 C → 英文；无翻译包（CentOS 7 的 `.mo` 为 0）→ 任何 locale 都英文 | **"实测"** | 无 | 行 1–2 本机复现；行 3 机制复现，CentOS 7 本体不可判 |

**J7 的选项**：(a) 在 3.2 的机制列显式标注证据来源（"读 sudoers 文件" / "实测" / "文档"）；(b) 把"红帽预置 env_keep""Ubuntu 仅靠 env_check"降级为"待验证"，不参与决定 2 的依据。代价：文字返工；收益：符合 12.1 第 3 条，复审时不必再猜哪些是实测。若不改，读者会把两行都当成已实测的发行版结论，而实际只有两台机器、无产物。

### 6.2 本机复现 3.3 的行 1–2

```text
$ LC_ALL=zh_CN.utf8 dpkg -s nosuchpkg
dpkg-query: 系统没有安装软件包 nosuchpkg，因而没有相关的信息   [exit=1]
$ LC_ALL=C dpkg -s nosuchpkg
dpkg-query: package 'nosuchpkg' is not installed ...            [exit=1]
$ LC_ALL=en_US.UTF-8 dpkg -s nosuchpkg
dpkg-query: package 'nosuchpkg' is not installed ...            [exit=1]
```

`en_US.UTF-8` 未生成时 dpkg **不吐 setlocale 警告**，提示语与 C 相同（英文），退出码同为 1。这支持 3.3 的"`C` 与 `en_US.UTF-8` 对提示语效果相同"，也支持 3.5 的"非 bash/perl 的 C 程序 0 行"。

行 3 的机制本机也能复现：`ls`（uutils）没有 `coreutils.mo`，在 `zh_CN.utf8` 下仍输出英文 `ls: cannot access ...: No such file or directory`。所以"无翻译包 → 任何 locale 都英文"这个机制成立；但"CentOS 7 的 `zh_CN/LC_MESSAGES` 里 0 个 `.mo`"这一具体事实**不可判（环境）**。

### 6.3 `env_check` 的 `%` / `/` 边界能否在本机复现

**不能，U2（不可判：环境 + 域外依赖）**。原因三条：

1. 本机 `sudo` 是 **sudo-rs 0.2.14**，不是 classic sudo 1.8/1.9；即使能跑，也不是 ADR 3.2 说的那套 `env_check` 实现。
2. `sudo -n true` 直接报 `sudo: 无效的配置：/etc 必须属于 root`，退出 1；本机无法执行任何 sudo 测试。
3. 要判断 `%` / `/` 触发丢弃，需要读 `/etc/sudoers` 并构造变量实测；`/etc/sudoers` 不在本轴白名单（ADR 三/六节 + `command.go` 315–464 + 沙盒），读它就是扩域。按纪律停手。

因此 3.2 的边界表（`LANG=en_US.UTF-8%x → 丢失`、`LANG=a/b → 丢失`）本轮**没有本机证据**，只能记"ADR 声称 CentOS 7.9 实测，产物未列"。

**另**：3.2 第 96 行 `env_keep` 列表里 `LC_ALL` 重复了一次（"含 `LANG` / `LC_ALL` / `LC_ALL` 等"），疑为 `LC_CTYPE` 笔误。不影响结论，但引用时会被读成笔误。

---

## Q7. 6.2–6.5 四段理由的事实前提核查

### 6.2（依赖 3.6 / 3.7）

| 前提 | 核查 | 判定 |
| --- | --- | --- |
| "`C` 下 `ls`、`echo`、`printf` 的中文输出逐字节原样透传" | `printf` / `cat` / 单文件 `ls` 的字节与 zh_CN 相同；但多文件 `ls` 的顺序在 C 与 zh_CN 下不同（`Alpha Beta IP _x alpha beta ip` vs `alpha Alpha beta Beta ip IP _x`） | **成立但限"文件名字节"，不适用于整段输出**；J2 |
| "`od` 对比与 `en_US.UTF-8` 完全相同" | 本机 `en_US.UTF-8` 未生成，无法做该对比；C vs zh_CN 的 `printf`/`cat` 字节相同 | **不可判（环境）**；机制上成立 |
| "第二个轴的退化落在 U22『一般这些东西不会是中文』范围之外" | 引用了作者原话；本轮无法核 U22 原文（议题 spec 不在白名单） | **不可判（域外依赖）**；但引用本身与 ADR 文首一致 |
| 3.6 "`free -m` / `df -h` / `top -bn1` 两取值一致" | `df -h /`、`sort`、`sed`、`grep -c` 逐字节相同；`free -m` 在 zh_CN 下表头被翻译（C 英文），且数值随时间变；`top` 未逐字节比对 | **`free` 的"一致"只在 C vs en_US（都英文）成立；对比 zh_CN 不一致**；数值本身不可逐字节复现 |
| 3.6 "`awk %.2f` 逐字节相同" | `awk 'BEGIN{printf "%.2f\n",3.14159}'` C vs zh_CN 相同 | **成立**（mawk；gawk 不可判） |

### 6.3（依赖 3.4 末尾）

- "CentOS 7 不存在 `C.UTF-8`"：本机无 CentOS → **不可判（环境）**。
- "SUSE 的 `C.UTF-8` 是另做的一套，信创系统又各有实现，跨发行版行为无法枚举"：12.2 明说 SLES / 信创"完全未验证"。**J3**：这是推断，却在 6.3 与第十节当事实用；按 12.1 第 3 条应标"推断/未验证"。选项：(a) 在 6.3 句末加"（未验证，见 12.2）"；(b) 把该理由降为"待验证的排除项"，不参与决定。代价都是文字，但影响"`C.UTF-8` 被排除"这条理由的硬度。

### 6.4（依赖 3.1 与解释器约束）

- **U17 约束成立**：`command.go:323–339` 的 15 条里 8 条是非 shell（python/luajit/lua/node/nodejs/perl/ruby/php），`export` 确实不是它们的语法；环境段只能挂在 shell 层。这一条是决定 2 最硬的依据。
- "`export` 已经能穿过 sudo（3.1、3.2）"：证据只在 WSL2 的 Ubuntu 26.04 + CentOS 7.9 两台上；本机 classic sudo 不可用（sudo-rs）。12.2 自己说 SLES / 信创未验证。**"就没有任何理由去动它"里的"任何"超出证据**（J6）。
- "`sudo env K=V` 绕过 sudo 的全部环境过滤——`${}` 注入、`PATH` 劫持都不再受控"：安全论断，无实测/源码引用；**未标注为推断**。它可能成立，但按 12.1 第 3 条应标"推断"。
- "要求 sudoers 允许执行 `env`"：条件句，逻辑成立；无产物。

### 6.5（依赖第十节）

- 第十节的否掉理由：维持 `en_US`（警告）、`C.UTF-8`（CentOS 7 不存在 / 代改目标机 / SUSE 信创不统一）、`sudo env`（安全 + sudoers）、`sudo -E`（`SETENV`）、改 sudoers（每台状态不同）、按发行版分档（破坏逐字可比）、转码（乱码不存在）、过滤警告行（治标）。**大部分是可靠性口径**，与 6.5 的自述基本一致。
- **J4**：但有两处不完全一致：(a) "要么工具代改目标机" 是**范围/操作负担**（不去动 OS 状态），不是"会不会失效"；(b) "SUSE / 信创实现不统一" 是未验证推断（12.2）。所以"没有一条是工作量"作为自评**大体成立、字面不严密**。选项：(a) 把"代改目标机"改写成"越出工具职责边界"；(b) 保留但在句末承认这两条不是纯可靠性。代价：文字；收益：经得起"你自己也在用非可靠性理由"的反问。

### 6.1（横切，见 Q4）

- "2000 台里有多少台没生成 `en_US.UTF-8` 是不可知的"——认识论陈述，成立。
- "每次 bash 多 1 行、每次 perl 多 7 行"——bash 1 行成立；perl 7 行不成立（19，H1）。
- 四句"永远"——H2/H5。

---

## Q8. 第五节决定的枚举义务：决定 1 与决定 2 的技术理由成不成立

### 决定 1（`LANG` / `LC_ALL` → `C`）的理由 6.1

**主体成立，全称部分不成立。**

- 成立：`C` 是 glibc 内置、`setlocale` 恒成功、C 下六种解释器 0 行 locale 警告；未生成 `en_US.UTF-8` 的机器上 bash 1 行 / perl 19 行是实测的污染源。对"同批机器逐字可比"这个目标，"减少一种目标机状态"的方向是对的。
- 不成立：`永远按字节`（python/node/uutils `wc -m`）、`永远成功`（Python <3.7 非 ASCII exit 1）、`永远安静`（perl `use utf8`）、`每次 perl 7 行`（19）。另外 3.4 的"未生成→回落"在 uutils 上不成立，意味着决定 1 的"代价只在已生成机器上出现"这个隐含前提也不成立：按 ADR 9.3 所说的 Ubuntu 26.04（uutils），未生成 `en_US` 时 `cut`/`sort` 仍按字符，C 反而**立即**付出字符语义代价。

**选项与代价**（J5）：

| 选项 | 代价 | 收益 |
| --- | --- | --- |
| (a) 保留 C，改写 6.1：删"永远"，按"解释器 × 版本 × 工具链实现"限定；补 Python <3.7 的兼容性注记 | 文字返工 + 一次复审；行为不变 | 理由链与实测对齐，spec/手册可据此写验收 |
| (b) 保留 C，不改文字 | 无返工 | 已知假全称进入 spec / 手册，后续复审必再翻案 |
| (c) 改回 `en_US.UTF-8` | 未生成机器上重新引入 1/19 行污染（实测） | 已生成机器上保住字符语义 |
| (d) 保留 C + 下发前探测 `locale -a` 再决定 | 违反 U12"别去检测环境"与第八节"取值一次定死，全量下发"；也偏离锚 A1（固定为 C） | 能按机器选值，但作者明令禁止 |

不建议在本轴替 spec 选 (a)/(c)；只指出判点①的判定：**若选 (a)，"用户脚本不受影响"只有在验收条件能区分"解释器版本 + coreutils 实现"时才可验收；否则该条不可验收**。这是差异报告，不是修法。

### 决定 2（拼接形态不变）的理由 6.4

**条件成立。**

- 硬约束成立：U17 作者原话 + `interpreterTable` 的 8 条非 shell 条目，`export` 不是它们的语法；环境段只能挂在 shell 层。这个理由足以支撑"不把环境写进解释器正文"。
- 条件部分：`export` 能穿过 sudo 只在两台 WSL2 机器上测过；本机无法复验 classic sudo；12.2 明说 SLES / 信创未验证。故"没有任何理由去动它"应限定为"在已验的两发行版上，现状可用；未验机群不断言"。
- 备选的代价（`sudo env` 绕过过滤、sudoers 白名单）是合理的安全/兼容论点，但未标注为推断，也没有产物。

**选项与代价**（J6）：

| 选项 | 代价 | 收益 |
| --- | --- | --- |
| (a) 保留现状 + 把 6.4 的"任何"改为"在已验两发行版上"，spec 不承诺未验机群的 locale 到达 | 文字；spec 少一条强承诺 | 与 3.1/3.2 的证据边界一致 |
| (b) 保留现状 + 增加解释器内环境注入（如 python 的 `os.environ` 前置） | 违反 U17 的"解释器可随意配置"，要为每种解释器写适配；且绕过 sudo 过滤 | 未验机群也能到达，但与作者约束冲突 |
| (c) 改 `sudo env K=V` | 安全面 + sudoers 白名单风险（6.4 已列） | 不依赖 `env_check`/`env_keep` |

**结论**：决定 2 的"不改"是稳的；需要改的是理由的**外推范围**，不是决定本身。

---

## 四个判点（按轴任务口径）

### ① 每条决定能否落成可验收工单

| 决定 | 可验收性 | 说明 |
| --- | --- | --- |
| 1. `LANG`/`LC_ALL` → `C` | **可以**，但验收条件必须分档 | 配置断言（两行值）+ 行为断言（在只有 C/C.utf8/POSIX 的机器上 bash/perl stderr 0 行）。**不能**把"用户脚本不受影响"写成无条件验收：Python <3.7 反例（H5），mawk/uutils 的字符语义反例（H3/H4）。 |
| 2. 拼接形态不变 | **可以** | 回归测试断言 `DescribeCommand` 的下发行形态不变；无代码改动。 |
| 3. 0015/0016 各加一行 | **可以** | 文档 diff。 |
| 4. `PATH`/`TERM` 不变 | **可以** | 配置断言。 |
| 5. 乱码不处置 | **不宜作为工单** | 这是范围声明，不是可执行项；验收只能是"无代码改动"。选项：(a) 作为"不在本批范围"的说明，不占待办；(b) 作为一条待办，但验收内容仍是"无代码改动"。 |

### ② 理由链里的全称化过头

已列 H1–H5；另有：

- 3.2 "Ubuntu 26.04 **仅靠** `env_check`"——单机缺席断言，外推到发行版（J7）。
- 6.4 "就没有任何理由去动它"——只在两发行版上有证据（J6）。
- 3.7 各行没有"实现"限定，`awk`/`wc -m` 两行在 mawk/uutils 上直接不成立（H3）。
- 3.4 "未生成时**告警并回落**"——uutils 不回落（H4）。

### ③ 承重量化结论的产物可追性

| 量化结论 | 产物 | 判定 |
| --- | --- | --- |
| 3.5 bash 1 行 / perl 7 行 | 第十一节只说"沙盒产物（`build/` 下，按天清理）"，无文件名/哈希 | **不可判（无产物）**；本机复跑得 1 / **19** |
| 3.8 纯小写 0%（200 组）、大小写混合 85%（170/200） | 无脚本/样本/输出文件名 | **不可判（无产物）**；不在本轴复跑（`build/` 历史产物属轴 3） |
| 3.1 两发行版穿透表 | 无原始输出文件 | **不可判（无产物）** |
| 3.2 `env_keep` 内容 / 边界表 | 无 sudoers 摘录 | **不可判（无产物 + 域外）** |
| 3.6 "逐字节比对" | 无对比脚本/输出 | 本机可复跑一部分（见 Q7），原机不可判 |

这是判点③的核心：**ADR 把"按天清理"的沙盒当成了证据链的终点**。结论本身可能真，但 spec/工单无法据此复核。

### ④ 标注是否诚实

- **做得好的**：3.2 的 CentOS 5/6 明标"源码推断"；12.1 第 3 条主动提醒 3.1 是 WSL2；12.2 列出 SLES/信创未验证；第四节明确区分"实测/未能复查/推断"。这在 ADR 里属于高水位。
- **不够的**：
  - 3.5 没写 perl 版本，导致 7 行被当成通用值（H1）。
  - 3.7 没写实现（mawk/gawk、GNU/uutils）（H3）。
  - 3.4 的"回落"没写实现前提（H4）。
  - 6.1 的四句"永远"没标为"推断/在测试集内成立"（H2/H5）。
  - 6.3 的 SUSE/信创在 12.2 是"未验证"，在 6.3/第十节却当事实（J3）。
  - 6.4 的安全代价没标推断。

**总评**：标注诚实度在"发行版/sudo 版本"维度较高，在"解释器/工具链实现"维度明显不足；而后者正是本轴的问题域。

---

## 锚质疑

**本轮未发现 A1–A5 与实测、源码、上游 ADR 正文或四个判点直接冲突。** 逐条说明：

- **A1（`LANG`/`LC_ALL` → `C`）**：值本身不与任何实测冲突；C 的存在性、零警告、提示语英文均成立。差异在 **6.1 的理由**（"永远按字节/成功/安静"），不在锚。
- **A2（拼接形态不变）**：没有被实测推翻；3.1 的穿透在两台机器上成立。问题是证据只覆盖两台（12.2 自认），属"证据不足"而非"与实测冲突"。
- **A3（0015/0016 各加一行）**：文档动作，无冲突。ADR-0015"多数发行版保留 `LC_*`"被 3.2 的两台机器支持，但"多数"本身仍是外推；这不构成对 A3 的冲突。
- **A4（`PATH`/`TERM` 不变）**：9.2 说 `PATH` 被 `secure_path` 替换、`TERM` 穿过；与"保持出厂值"不冲突。
- **A5（乱码不处置）**：与第四节的"工具零转码"实测不冲突。**边界观察**（不构成锚质疑）：C 下用户脚本 `cut -c`/`awk substr` 会切出非法 UTF-8（实测 `e4 b8`），而第四节自述 xlsx 侧唯一改写是 `strings.ToValidUTF8(s,"\uFFFD")`——这会把半个字变成 `�`。ADR 6.2 把这叫"值被切坏，不是编码被改坏"，在定义上自洽；但用户在 Excel 里看到的是 `�`，是否算 A5 说的"乱码"，是一个**定义边界**，不是事实冲突。留给 spec 裁决：若"乱码"定义为控制台码页现象，则 U+FFFD 不在 A5 范围内；若验收标准包含 Excel 里的 `�`，则 A5 需要重述。两种定义的代价不同，本轴不选。

因此本节**不列锚质疑项**；上述 A5 边界放在这里供裁，不改写锚。

---

## 域外依赖（停手不扩域）

| 问题编号 | 缺哪个文件/能力 | 为什么白名单不够 | 结论 |
| --- | --- | --- | --- |
| U2 / Q6 | classic sudo（1.8/1.9）+ 可读的 `/etc/sudoers` + 非 root 账号 | `/etc/sudoers` 不在"ADR 三/六节 + `command.go` 315–464 + 沙盒"白名单；本机 sudo 是 sudo-rs 0.2.14 且运行被拒 | **不可判（环境 + 域外依赖）** |
| Q5 | `ksh` / `zsh` / `mksh` / `ash` / `luajit` / `lua` / `ruby` / `php` 二进制 | 安装系统包超出沙盒与只读授权 | **不可判（环境）** |
| Q1/Q4 | Python 2.7 / 3.6 二进制 | 同上 | **不可判（环境）**；机制已用 3.14 关 PEP538/540 复现 |
| Q3 | `gawk` / GNU coreutils（GNU `wc`、`cut`） | 本机 coreutils 是 uutils 0.12.0，`gawk` 未装 | **不可判（环境）**；uutils/mawk 侧已测 |
| Q6 / 3.2 | CentOS 7.9、Ubuntu 26.04 真机 | 无目标机 | **不可判（环境）** |
| Q7 / 6.3 | SLES / 麒麟 / UOS / Deepin | 无环境；12.2 自认未验证 | **不可判（域外依赖）** |
| 判点③ | 3.8 的 200 组抽样脚本与输出 | `build/` 历史产物属轴 3，不在本轴白名单 | **不可判（无产物）** |

---

## 产物索引（`build/audit-2026-10-22-axis2/`）

| 文件 | 内容 |
| --- | --- |
| `versions.txt` | glibc / bash / coreutils / grep / sed / awk / sudo 版本 |
| `probe-locale.sh` + `probe-locale.log` | 六种解释器 × C / zh_CN / en_US（未生成）的写读输出、退出码、stderr 行数、hex |
| `probe-commands.sh` + `out-commands/commands.log` | 3.7 各行（awk/cut/grep/wc/[[:alpha:]]/sed/tr/sort）在 C / zh_CN 下的字节对比 |
| `probe-extra.log` | Python 真字节模式、工具形态 sh→bash 警告、perl 19 行、C 是否在 archive |
| `probe-utils-warnings.log` / `probe-utils-warnings2.log` | 外部命令在未生成 en_US 下的 stderr 行数（含 bash 外层污染对照） |
| `probe-setlocale.log` | python `locale.setlocale` 异常、perl 返回值、`locale` 命令 3 行 |
| `probe-python-exc.log` | Python 未捕获 setlocale 异常的 7 行 traceback + exit 1 |
| `probe-python-old.log` | 关闭 PEP538/540 模拟 Python <3.7 在 C 下打印中文 exit 1 |
| `probe-perl-utf8.log` | perl `use utf8` 在 C / zh_CN 下均 1 行 `Wide character` |
| `probe-write.log` | C 下 bash/python/perl/node 写中文文件名、`ls` 字节 |
| `probe-3.6.log` | 3.6 的 `free`/`df`/`awk %.2f`/`cut` 对比；`ls` 多文件排序差异 |
| `probe-fallback.log` | 未生成 en_US 时 cut/grep/wc/bash/awk 的"回落"实测 |
| `probe-uutils-locale.log` | uutils cut 只看 locale 名、不校验存在；sort/wc 的 C vs en_US vs zh_CN |
