# 07 · `/dev/shm` 落文件 + 解释器执行：可行性与影响面（探针回执）

- 日期：2026-10-22
- 对象：开发者提的**待定方案**——不论 `-c` / `-s` / `-a`，先把正文落到目标机 `/dev/shm` 的一个文件里，再用配置的解释器执行该文件；理由是「内存文件系统，不用管磁盘满不满，重启就没了」。
- 本回执只做三件事：**读现状、逐项判「改变 / 不改变」、把能测的测出来**。不对「采不采纳」表态，也不给实现方案。
- 授权边界：本轮只在 `build/shm-analysis/` 下写探针与笔记；仓库代码与既有文档未改一字。本文件是本次审计 `raw/` 的指定产物（`01`–`06` 同目录，均为新增）。
- 本机：Ubuntu 26.10 容器，内核 `7.3.0-9-generic`，以 `root` 运行；bash `5.3.9(1)`；`/bin/sh` → `dash`；`sudo` 不可用（无 `/etc/sudoers` 可用身份）；`getenforce` 不存在；AppArmor 已启用但无相关 profile。

> **测量口径的两条限制**（先声明，免得把探针环境的毛病当成产品结论）：
>
> 1. 本机每次 `Bash` 工具调用都会**重建 `/dev/shm`**（调用开始时目录为空，挂载时间戳随之变化）。因此所有涉 `/dev/shm` 的步骤都在**同一次调用**里跑完（`run.sh` / `run2.sh`），不跨调用保留文件。
> 2. 本机 `/dev/stdin` 的 `open()` 在部分调用里返回 `ENOENT`，而同一次调用里 `cat /dev/stdin` 正常、`source /proc/self/fd/0` 正常；同一命令换一次调用又可能成功。判定为**探针运行环境（沙箱/LSM）的不稳定行为，不是目标机的普遍事实**，故 §3.3 的 `$0` 对比用 `/proc/self/fd/0` 完成，并在该节标注。它不影响本回执的结论方向，但**「现状 `-s` 依赖 `. /dev/stdin`」本身是一个值得单独留意的既有前提**（见 §7 未测项）。

---

## 一、现状：四条路径各自怎么把正文送出去

先给一张总表，后面逐条给 `file:line`。

| 路径 | 远端命令行（下发行） | 正文去哪 | 有无 PTY | 体积上限 |
| --- | --- | --- | --- | --- |
| `-c` | `sh -c '<export …; [sudo ]解释器>'` | 会话 stdin | 无 | 流式，无上限 |
| `-s` | `sh -c '<export …; [sudo ]解释器 -c ". /dev/stdin" '脚本名''` | 会话 stdin | 无 | 流式，无上限 |
| `-a` | `sh -c '<export …; [sudo ]解释器 -c/-e/-r "$(printf %s <base64> \| base64 -d)" ['脚本名']>'` | 命令行 argv（base64） | 有（200×50） | 下发行 ≤ 122880 字节 |
| `--no-shell`（`-c`） | 原样命令，无外壳 | 无 stdin | 无 | 流式，无上限 |
| `--no-shell`（`-s`） | `解释器` 或 `解释器 -c '. /dev/stdin' '脚本名'` | 会话 stdin | 无 | 流式，无上限 |

### 1.1 `-c`：正文进会话 stdin

`BuildCommand` 的命令模式返回两样东西：命令行只有外壳，命令原文作为第二个返回值（`internal/ssh/command.go:106-124`；命令分支在 `:123`）：

```
sh -c '<export …; [sudo ]解释器>'
```

内层由 `innerCommand` 拼（`command.go:277-283`）：`envHead(envPrefix) + [sudo ] + interpreter`。解释器后面**没有** `-c`，所以解释器从 stdin 读程序——这就是命令原文字节要走 stdin 的原因。

`RunCommand` 拿到非空 stdin 后单起一个协程写入并关闭（`internal/ssh/exec.go:96-106`）。正文不参与 shell 解析、没有引号套娃、没有 argv 长度上限（`command.go:63-95` 的注释把这三条写成了设计理由）。

### 1.2 `-s`：正文进 stdin，`$0` 靠 source 还原

脚本模式同样返回 `sh -c '…'` + stdin 正文，但内层是 `scriptStdinInner`（`command.go:420-426`）→ `scriptInvocation`（`command.go:435-440`）：

```
<export …; [sudo ]><解释器> -c '. /dev/stdin' '<脚本名>'
```

- `. /dev/stdin` 把 stdin 上的正文读进**当前解释器进程**执行，`-c` 之后的第一个参数就是 shell 的 `$0`，于是原脚本的 basename 被交回去（`scriptNameArg`，`command.go:300-305`）。
- zsh 额外先 `setopt no_function_argzero`（`command.go:328`、`command.go:446-452`）。
- 非 shell 家族不补名字，裸解释器直接读 stdin（`scriptInvocation` 的 else 分支，`command.go:439`）。

正文由 `ScriptBodyOf` 整备：剥 UTF-8 BOM、`TrimSpace`（`internal/ssh/interactive.go:105-112`）。读文件与整备分两处：`cli/check.go:124-141` 读并校验（非空、无 NUL、UTF-8），`batch/batch.go:221-228` 再用 `ScriptMaterialOf` 拼出 `Body`（`cli/script.go:19-24`）。

### 1.3 `-a`：正文 base64 进 argv，PTY 把 stdin 整条让给代填

- 下发行**只在选项/参数合规阶段拼一次**，存进 `Args.Downlink`（`cli/answer.go:186-198`），长度检查、执行、日志都读这同一个值（`cli/answer.go:183-185`、`batch/batch.go:238-253`、`batch/batch.go:274-281`）。
- 拼法（`interactive.go:87-103`）：

```
sh -c '<export …; [sudo ]解释器 <programArg> "$(printf %s <base64> | base64 -d)" ['脚本名']>'
```

`programArg` 由 `ProgramTextArg` 按解释器取：shell/python `-c`，perl/ruby/node/lua `-e`，php `-r`（`command.go:374-387`、`command.go:323-339`）。
- 上限写死 `answerLimit = 122880` 字节，依据是 `MAX_ARG_STRLEN = 131072` 留余量（`cli/answer.go:24-28`）；量的是**下发行**，所以正文实际上限约 `(122880 - 外壳) × 3/4 ≈ 92KB`。
- 解释器必须在表内（`cli/interpreter.go:90-108`、`cli/check.go:149-151`），表外直接报错并列出支持范围。
- PTY：`RequestPty("xterm", 50, 200, nil)`（`interactive.go:169-174`）；stdin 由协程逐条喂代填（`interactive.go:176-194`）。
- **为什么不用 stdin**：一次会话只有一条 stdin，代填要独占（`interactive.go:84-86`）。**为什么不落盘**：注释写明「目标机磁盘满是常态，落 /tmp 会因磁盘失败（ADR-0010）」（`interactive.go:85-86`）。新方案正是要改这两条前提中的第二条。

### 1.4 `--no-shell`：不套外壳、不设环境、不加 sudo

`BuildCommand` 的第一个分支（`command.go:107-116`）：

- 命令模式：`return command, ""`——原样下发，**不喂 stdin**。
- 脚本模式：`return scriptInvocation(interpreter, scriptName), scriptBody`——只剩解释器自己那条调用，正文**照旧走 stdin**；shell 家族仍然是 `. /dev/stdin` + `$0`。
- sudo 被丢弃（`cli/args.go:128-134` 的 `Identity()`；`cli/confirm.go:40-44` 的警告门）。
- `-a` 与 `--no-shell` 互斥，报错文案说明「同一个会话满足不了两种要求」（`cli/answer.go:131-143`）。

### 1.5 「日志里的命令行」与实际下发的串是不是同一个来源

分三种情况，答案不一样：

1. **`-c` / `-s`**：同一个**函数**、同一组入参，但是**两次独立拼装**。`BuildCommand`（`command.go:106-124`）与 `DescribeCommand`（`command.go:206-243`）都调用 `innerCommand` / `scriptStdinInner`，再各自 `"sh -c " + shellQuote(inner)`（`command.go:121-123` 与 `:222-226`）。所以按当前代码是逐字一致的，但它是「同源重建」而不是「同一个变量」——将来只改一处就会漂移。日志里 `命令行:` 只有外壳，**正文在下一行 `stdin:`**，因为正文本来就不在 SSH 命令行里。
2. **`-a`**：字面意义上的同一个值。`Args.Downlink` 拼一次，长度检查量的、`session.Start` 发的、日志打印的是同一个字符串（`cli/answer.go:186-198`、`batch/batch.go:241-253`、`batch/batch.go:274-281`）。`DescribeInteractive` 另外把「原始命令 / 原始脚本 + 处理方式 + 最终命令」三段都打出来（`interactive.go:126-140`），所以**正文在执行日志里是可复原的**（base64 串 + 原文）。
3. **`--no-shell`**：命令模式日志打的就是原命令；脚本模式打 `scriptInvocation(...)`，与实际下发同源（`command.go:206-220`）。

> 这条对评估新方案很关键：**新方案一旦把正文移出命令行，`-a` 日志里「可复原正文」这一性质就会消失**（只剩一个执行后即被删除的路径）。见 §4「不解决」与 §6-H7。

---

## 二、逐项判定：改变什么、不改变什么

每项都写成「因为 A，所以 B」。

### 2.1 `-a` 的 argv / base64 上限、`$0` / `$LINENO` / shebang、日志可读性

**体积上限：改变。**
因为正文不再进 argv，`MAX_ARG_STRLEN`（131072）与 `answerLimit`（122880）都不再约束正文，所以 `-a` 的「约 92KB 正文」上限消失。
但因为改成受 `/dev/shm` 容量约束，所以换来一个**不同的**上限：本机 10MB（`stat -f`：`blocks=2560 × 4096`），常见容器 64MB，物理机默认约为内存一半。**它不是「没有上限」，是把上限从 argv 换成了 tmpfs。**

**`$0` / `$LINENO`：`$LINENO` 不改变，`$0` 改变。**
因为 `$LINENO` 是解释器对「第几行」的计数，与载体无关，实测两种形态都是 `3`。
因为文件形态下 `$0` 是**被执行的路径**（`bash /dev/shm/x.sh` → `/dev/shm/x.sh`；`cd /dev/shm && ./x.sh` → `./x.sh`），而现状 `-s` 是把原 basename 交给 `-c` 后的第一个参数（`command.go:300-305`），所以 `$0` 从「原 basename」变成「路径」。若把文件命名为原名并放进唯一目录，`basename "$0"` 仍等于原名，但**直接比较 `$0` 的脚本会看到不同的值**。
`-a` 的非 shell 分支现在把脚本名落到 `sys.argv[1]`（`command.go:296-299` 的注释；
`scriptNameArg` 只给 shell 家族补），文件形态下变成 `sys.argv[0] = 路径`，这是**语义改善**，但也确实是改变。

**shebang：不改变（只要坚持 `<解释器> <文件>`），但一旦直接执行就改变。**
因为方案的原意是「用配置的解释器执行这个文件」，shebang 只是文件里的注释，不参与选择解释器；所以 shebang 有无、写的是什么，都不影响乙形态。
因为一旦写成 `./file` 或 `sudo ./file`，内核就按 shebang 选解释器（实测：`#!/bin/bash\r` 直接执行 → `/bin/bash^M: 错误的解释器`，rc=126；`bash file` → 正常），所以那条路会**绕过 `[interpreter]` 配置**，与 U17 冲突，并且在 `noexec` 挂载上直接失败（§3.2）。

**日志可读性：改变，且是双向的。**
因为 `-a` 现在把 base64 与原文都打出来（`interactive.go:126-140`），所以事后能从日志复原「到底执行了什么字节」。
因为文件形态日志只能打路径，而路径在执行后按设计要删除，所以**日志不再能复原正文**；除非额外把正文也打进日志（那会把秘密写进归档）。
另一面：因为 PTY 会把整条命令行回显进采集输出（`interactive.go:169-174`；`Start(command)` 在 `:273`），所以现在 120KB base64 会进 `Output`，文件形态的回显只剩一条短命令——**输出污染显著减少**，触发词误匹配的面也变小。

### 2.2 解释器可配置（U17）

**对 `-a`：变容易。**
因为现状 `-a` 要求正文是一段「程序文本」，所以必须知道每家的程序文本参数（`-c` / `-e` / `-r`），也就必须维护 `interpreterTable` 并在表外拒绝（`cli/interpreter.go:96-108`）。
因为文件形态只需要 `解释器 文件` 这一种调用，所以 `tclsh` / `deno` / `awk` 这类表外解释器**也能跑**，U17 的「随意配置」在 `-a` 上第一次真正成立。
代价：`$0` 的补名逻辑不再需要（文件路径天然是 `$0`），但 `interpreterTable` 仍被 `-s` 的 shell 家族判定与 `--no-shell` 使用，不能整表删除。

**对 `-c` / `-s`：不改变。**
因为这两条路本来就不需要程序文本参数，解释器只是「读 stdin 的程序」，所以文件形态对「支持哪些解释器」没有增量。

**另有一处变难：走 shebang 就直接违反 U17。**
因为 shebang 由文件内容决定，不由 `[interpreter]` 决定，所以「直接执行文件」这条路不能用于满足 U17；必须始终显式写解释器。

### 2.3 交互（PTY、代填触发词、stdin 归属）

**不改变：PTY 仍要、触发词仍靠输出匹配、stdin 仍整条给代填。**
因为代填的机制是「读远端输出 → 命中触发词 → 往 stdin 写一条」（`interactive.go:337-386`、`:266-315`），文件只是正文载体，不改变这个回路；实测文件执行不消费 stdin。
因为 `-a` 仍然需要 PTY 才能让远端程序及时 flush 提示（`interactive.go:169-174`），所以这条依赖不变。

**改变（改善）：PTY 回显变短。**
因为 PTY 默认 `ECHO` 开着（`interactive.go:169-170` 注释），所以现在整条 base64 命令会被回显；文件形态命令行只有 `... bash /dev/shm/x.sh`，回显与日志都干净得多。

**改变（新增依赖）：写文件必须走第二个通道。**
因为 PTY 上没有「半关闭 / EOF」——`Ctrl-D` 只是行规程里的一个字节，会被回显、也可能被触发词匹配——所以不能在同一 PTY 会话里「先把正文喂给 `cat > file`，再喂答案」。
因为 `-a` 的 stdin 已经整条让给代填，所以正文只能走**另一条通道**：要么 SFTP 子系统，要么另开一个 exec 会话先写文件。
因为部分 sshd 会关闭 `Subsystem sftp` 或把它 chroot（`internal-sftp` + `ChrootDirectory`），所以新方案把「只要有 exec 就能跑」变成「还要有可用的 sftp / 第二个会话」——这是一条**新的硬依赖**。

**不改变：`-a` 与 `--no-shell`、`-u`、`-d` 的互斥。** 因为互斥的是「同一条会话该长什么样」，与正文载体无关（`cli/answer.go:131-143`）。

### 2.4 语言环境（`export` 穿过 sudo 这件事）

**不改变：`export` 仍然必须在解释器之前，仍然照旧拼在外壳里。**
因为文件只是「正文放在哪」，解释器仍是在 `sh -c 'export …; sudo <解释器> <文件>'` 里被拉起的（`command.go:277-283`），`sudo` 看到的还是同一条命令、同一组前置变量，所以 `env_reset` / `env_check` / `env_keep` 的行为与现在**逐字相同**。
因为 ADR-0024 §3.1/§3.2 的结论是「`LANG`/`LC_ALL`/`TERM` 能穿过 sudo」，所以这个方案**既不需要它、也不能替代它**：文件不会让 `export` 变得不必要，也不会让它更容易或更难穿过。
**推论**：乙形态在 locale 这件事上是**零收益、零损失**；真正的变化只发生在丙形态（见 §5）。

**不改变：`PATH` 在 sudo 下被 `secure_path` 替换**（ADR-0024 §9.2）。因为替换发生在 sudo 内部，与正文载体无关。

### 2.5 编码（文件字节与 stdin 字节是否等价；换行、BOM、CRLF）

**等价的前提：写进文件的必须是 `ScriptBodyOf` 之后的 `Body`，而不是本地文件的原始字节。**
因为现状 stdin 送的是 `Body`（剥 BOM + `TrimSpace`，`interactive.go:110-112`），所以只要文件写入的也是同一串字节，二者就是**同一份内容**。
实测：同一段正文（含 `\t`、`\r\n`、UTF-8 中文、**结尾不带换行**）分别走 stdin 与走文件，输出 `cmp -l` 无差异、`wc -c` 都是 40 字节（§3.4）。

**不改变：sftp 是二进制通道，不做 CRLF 转换；文件内容也不会被工具改写。**
因为写入是 `sftpClient.Create` + `io.Copy`（`internal/ssh/sftp.go:426-441`），字节原样；所以换行、BOM、CRLF 的去留完全取决于「写之前整备成什么」。

**会改变的一种写法：如果绕过 `ScriptBodyOf` 直接上传本地原文件，BOM 就留下来了。**
因为现状会剥 BOM，所以带 BOM 的脚本现在能跑；因为直接上传会保留 BOM，所以第一行变成 `﻿#!/bin/bash`：
- `bash file` / `./file`：报 `﻿#!/bin/bash: 没有那个文件或目录`，但**后面的行继续执行**，最终 rc 可能是 0（实测：rc=0，输出里既有错误行也有 `bom-file-ok`）。
- 走 stdin（现状）：同样报这一行，同样继续（实测）。
也就是说 **BOM 不是文件形态独有的问题，但文件形态若不做整备就会把现状已经修掉的坑重新引回来**。

**CRLF：不改变现状的失败形态，但直接执行会把它升级。**
因为 `ScriptBodyOf` 不归一 CRLF（只剥 BOM + TrimSpace），所以现状 stdin 也把 `\r` 原样送进去，两种形态都保留 CRLF。
实测：`bash file` 与 `bash`（stdin）对全 CRLF 正文的输出都是 `one\r\n two\r\n`（`od -c` 一致）。
但直接执行时 `#!/bin/bash\r` 被内核当成解释器路径 `/bin/bash\r`，rc=126 —— **这是 shebang 路径独有的新失败**，不走 shebang 就没有。

**编码：不改变。** `cli/check.go:124-141` 已经拒绝非 UTF-8 与含 NUL 的脚本，所以进 `Body` 的一定是 UTF-8 文本；sftp 不转码，因此不存在「文件形态引入编码转换」的问题。

### 2.6 失败面（逐条：磁盘满、`/dev/shm` 不存在或太小、`noexec`、`nosuid`、SELinux/AppArmor、并发同名、残留清理、权限、退出码与信号）

| 面 | 现状 | 新方案 | 因为 → 所以 |
| --- | --- | --- | --- |
| **磁盘满** | 不依赖目标机磁盘；stdin 是流 | 不依赖磁盘，但依赖 tmpfs 容量 / cgroup 内存 | 因为 tmpfs 不落盘但占内存，所以「磁盘满」消失、「内存/容量满」出现 |
| **写满时的语义** | 无「写一半」概念 | **会留下截断文件** | 因为 `write()` 在 ENOSPC 时只写成功一部分，所以若先写后执行且不校验长度，会执行半个脚本。实测 11MB 写入只成功 9437184 字节（9.4MB），文件留在盘上 |
| **`/dev/shm` 不存在** | 无影响 | 整体失败 | 因为方案以它为前提，所以精简系统 / chroot / 非 Linux 目标机（无此路径）会直接不可用，需要探测与回退 |
| **`/dev/shm` 太小** | 无影响 | 超过容量即失败（本机 10MB） | 因为 tmpfs 有 `size=`，所以大脚本（安装包常见）会从「能跑」变成「写不下」 |
| **`noexec`** | 无影响 | **不挡**（只要显式 `解释器 file`），但挡 `./file` / shebang | 因为 `noexec` 只禁 `execve`，不禁「解释器 `open()` 文件」，所以实测 noexec tmpfs 上 `bash file` 成功、`./file` 返回 126。方案必须始终显式调用解释器 |
| **`nosuid`** | 无影响 | 无影响（脚本非 setuid） | 因为脚本不靠 setuid，所以本机 `/dev/shm` 的 `nosuid` 不构成问题；若将来落的是二进制则有影响 |
| **SELinux / AppArmor** | 不触碰文件系统 | 新增一次「在 tmpfs 上创建/读取」的访问 | 因为受限域可能拒绝 `tmpfs_t` 上的 `open`/`execute`，所以可能出现「本机手跑正常、客户机被策略拒绝」。本机不可测（无 SELinux、无相关 AppArmor profile） |
| **并发同名** | 无共享路径 | 固定名会互相覆盖 | 因为同名文件是「后写覆盖先写」，所以同机并发两次运行会执行到对方的正文。实测同一秒两个写者，最终只剩 `echo B` |
| **残留清理** | 零残留 | 正常路径可清，异常路径会留 | 因为超时/中断走 `session.Close()`（`exec.go:183`、`interactive.go:295/304`），远端 shell 未必来得及跑 `trap ... EXIT`，所以 SIGKILL/断连/断电必留文件；文件在重启前一直在 |
| **权限** | 不存在远程文件 | 受 umask 与写入方式支配 | 因为 `sftp.Create` 用服务端默认 `0666 & ~umask`（`sftp.go:426`），所以常见 `022` 下是 **644（同机其他用户可读）**；脚本里的密码/令牌因此暴露。实测 644/600 两种都在；`mktemp` 默认 600 |
| **属主** | — | 写入身份决定后续可覆盖性 | 因为登录用户写、sudo 读没问题，所以这条通常安全；但若用 sudo 写，文件归 root，后续非 sudo 运行无法覆盖同名 |
| **符号链接** | — | **可被诱导覆盖任意可写文件** | 因为 `sftp.Create` 与 `cat > path` 都是 follow-symlink + `O_TRUNC`，而 `/dev/shm` 通常 1777，所以攻击者可预置 `predictable.sh -> /home/user/.ssh/authorized_keys`；若写入者是 root，后果等同经典 tmpfile 漏洞。实测：写入穿过符号链接改掉了目标文件 |
| **退出码** | 单一来源（会话退出码） | 同会话内不变；拆会话则变成两个来源 | 因为「写」与「跑」若分成两次 exec，`RunCommand` 只能拿到后者，写失败必须单独判；现状没有这个分叉。实测同会话内 file/stdin 都是 `exit 7` → 7 |
| **信号** | 会话关闭即终止 | 同会话内不变；跨会话不转发 | 因为信号只作用于当前会话的进程组，所以「先写后跑」两个会话时，中断信号不会传到写侧，残留无人清。实测 `kill -TERM $$` 两种形态都是 143 |
| **额外依赖** | 只需 exec 通道 | 还需 sftp 子系统或第二个 exec 会话 | 因为写正文必须有一条不占 stdin、能半关闭的通道，所以 sshd 禁 sftp 的主机会整体失败 |
| **时延 / 失败点** | 一次会话 | 每台多一次握手/写入往返 | 因为 2000 台是逐台建立，所以总时延与失败概率都乘以「多一步」；正文越大越明显（现状 stdin 是流式） |

---

## 三、本机实测（命令 + 输出）

探针与原始输出都在 `build/shm-analysis/`：`run.sh`、`run2.sh`、`results.txt`、`results2.txt`。下面只摘关键段。

### 3.1 `/dev/shm` 挂载参数、可写性、剩余空间

```
$ findmnt -T /dev/shm -o TARGET,SOURCE,FSTYPE,OPTIONS
TARGET   SOURCE FSTYPE OPTIONS
/dev/shm tmpfs  tmpfs  rw,nosuid,nodev,inode64,usrquota
/dev/shm tmpfs  tmpfs  rw,nosuid,nodev,relatime,size=10240k,mode=755,uid=1000,gid=1000,inode64

$ grep -E ' /dev/shm | /tmp ' /proc/self/mountinfo
7059 7058 0:29 / /dev/shm rw,nosuid,nodev - tmpfs tmpfs rw,inode64,usrquota
7070 7063 0:165 / /dev/shm rw,nosuid,nodev,relatime - tmpfs tmpfs rw,size=10240k,mode=755,uid=1000,gid=1000,inode64
7113 7057 0:48 / /tmp rw,nosuid,nodev - tmpfs tmpfs rw,size=3744536k,nr_inodes=1048576,inode64,usrquota
7114 7113 0:166 / /tmp rw,nosuid,nodev,relatime - tmpfs tmpfs rw,size=10240k,mode=755,uid=1000,gid=1000,inode64

$ stat -f -c 'fstype=%T bsize=%S blocks=%b bfree=%f bavail=%a namelen=%l' /dev/shm
fstype=tmpfs bsize=4096 blocks=2560 bfree=2560 bavail=2560 namelen=255

$ df -h /dev/shm; df -i /dev/shm
tmpfs            10M     0   10M   0% /dev/shm
tmpfs          936133     1 936132   1% /dev/shm

$ ls -ld /dev/shm
drwxr-xr-x 2 root root 40 Oct 22 13:51 /dev/shm

$ printf x > /dev/shm/.w && echo writable && ls -l /dev/shm/.w
writable
-rw-rw-r-- 1 root root 1 Oct 22 13:51 /dev/shm/.w
```

事实：

- 有效容量 **10MB**（`blocks=2560`），远小于常见物理机（内存一半）与容器默认（64MB）。
- 挂载选项有 **`nosuid,nodev`**，**没有 `noexec`**。
- 本机 `/tmp` 的**有效**容量也是 10MB（外层 3.7GB 被内层 10MB tmpfs 盖住）——任务背景里「`/tmp` 4GB」在本机不成立，故本回执不拿 `/tmp` 的 4GB 做论据。
- 目录模式是 `755`（不是常见的 `1777`），`uid=1000/gid=1000` 来自挂载选项但 `ls` 显示 root:root（叠加挂载的呈现差异）。**客户机的 `/dev/shm` 通常是 `1777`**，其他用户可读/可建文件，§3.7 的暴露面按 1777 估计。
- 目录与文件系统可写；`PATH_MAX=4096`、`NAME_MAX=255`。

### 3.2 能不能跑：`./script.sh` 与 `bash /dev/shm/x.sh`

```
$ env -i PATH=/usr/bin:/bin /dev/shm/probe.sh a b
zero=[/dev/shm/probe.sh]
lineno=3
pid=21
argc=2
args=[a b]
bash_source=[/dev/shm/probe.sh]
[rc=0]

$ env -i PATH=/usr/bin:/bin bash /dev/shm/probe.sh a b
zero=[/dev/shm/probe.sh]
lineno=3
argc=2
args=[a b]
bash_source=[/dev/shm/probe.sh]
[rc=0]

$ env -i PATH=/usr/bin:/bin sh -c 'echo "echo dash-ok" > /dev/shm/d.sh; chmod 755 /dev/shm/d.sh; /dev/shm/d.sh'
dash-ok
[rc=0]
```

本机 `/dev/shm` **没有 `noexec`**，所以直接执行也能跑。为了把 `noexec` 的边界单独测出来，另挂一个 1MB 的 tmpfs：

```
$ mount -t tmpfs -o size=1m,noexec,nosuid,nodev tmpfs .../noexec
[mount rc=0]
$ env -i PATH=/usr/bin:/bin .../noexec/probe.sh
env: '.../noexec/probe.sh': Permission denied
[rc=126]
$ env -i PATH=/usr/bin:/bin bash .../noexec/probe.sh
zero=[.../noexec/probe.sh]
lineno=3
[rc=0]
$ env -i PATH=/usr/bin:/bin python3 .../noexec/probe.sh
  File ".../noexec/probe.sh", line 2
    printf 'zero=[%s]\n' "$0"
[rc=0]      ← python 能 open 文件，失败在语法（它读到了内容）
```

结论（事实）：**`noexec` 只挡 `execve`，不挡「解释器读文件」**。所以乙形态只要坚持 `<解释器> <文件>` 就不受 `noexec` 影响；受影响的是 `./file` / shebang 那条路。

### 3.3 `$0`、`$LINENO`、`$$`、参数传递：stdin vs 文件

同一份 `probe.sh`，五种跑法：

```
-- 工具现状形态：sh -c 'bash -c ". /dev/stdin" "probe.sh"' a b （正文走 stdin）
probe.sh: 行 1: /dev/stdin: 没有那个文件或目录      ← 本机探针环境异常，见文首口径
[rc=1]

-- 同上，source 目标换成 /proc/self/fd/0（绕开本机异常）
zero=[probe.sh]
lineno=3
pid=40
argc=0
args=[]
bash_source=[/proc/self/fd/0]
[rc=0]

-- bash -s probe.sh a b < body （另一种 stdin 形态）
zero=[bash]
lineno=3
pid=42
argc=3
args=[probe.sh a b]
bash_source=[]
[rc=0]

-- 文件：bash /dev/shm/probe.sh a b
zero=[/dev/shm/probe.sh]
lineno=3
pid=44
argc=2
args=[a b]
bash_source=[/dev/shm/probe.sh]
[rc=0]

-- 文件：直接 exec /dev/shm/probe.sh a b
zero=[/dev/shm/probe.sh]
lineno=3
pid=45
argc=2
args=[a b]
bash_source=[/dev/shm/probe.sh]
[rc=0]
```

差异归纳：

| 形态 | `$0` | `$LINENO` | `$#` | `BASH_SOURCE` |
| --- | --- | --- | --- | --- |
| 现状 `-s`（`. /dev/stdin` + 名字） | 原 basename（`probe.sh`） | 3 | 0 | `/dev/stdin`（实际值受 source 目标影响） |
| `bash -s 名字 a b` | `bash` | 3 | 3 | 空 |
| 文件 `bash /dev/shm/x.sh a b` | 路径 | 3 | 2 | 路径 |
| 文件 `cd /dev/shm && ./x.sh a b` | `./x.sh` | 3 | 2 | `./x.sh` |

`$$` 是进程号，跨形态不可比（每次都是新进程）；`$LINENO` 三种形态一致。

`/dev/stdin` 异常取证（同一次调用内）：

```
$ bash -c 'echo fd0:$(readlink /proc/self/fd/0); cat /dev/stdin >/dev/null; echo cat-ok' < mini.sh
fd0:pipe:[...]
cat: /dev/stdin: No such file or directory
cat-ok
```

即：`readlink` 能解析、`cat` 打不开、`source /proc/self/fd/0` 能开——**同一路径在不同系统调用/不同进程里表现不一致**，属探针环境问题。另一次调用里 `dash -c '. /dev/stdin'` 与 `bash --noprofile --norc -c '. /dev/stdin'` 又都成功。故本回执不据此判断「现状 `-s` 在客户机会不会坏」，只记录为未测项（§7）。

### 3.4 输出字节等价（`od -c`）

正文（`bytes.sh`）：

```
printf 'A\tB'
printf 'CRLF\r\n'
printf '中文UTF-8\n'
printf 'no-trailing-newline'
```

跑法一（stdin）与跑法二（文件）各重定向到文件，然后：

```
$ cmp -l out-stdin.bin out-file.bin; echo cmp_rc=$?
cmp_rc=0

$ od -c out-stdin.bin
0000000   A  \t   B   C   R   L   F  \r  \n 344 270 255 346 226 207   U
0000020   T   F   -   8  \n   n   o   -   t   r   a   i   l   i   n   g
0000040   -   n   e   w   l   i   n   e

$ od -c out-file.bin
0000000   A  \t   B   C   R   L   F  \r  \n 344 270 255 346 226 207   U
0000020   T   F   -   8  \n   n   o   -   t   r   a   i   l   i   n   g
0000040   -   n   e   w   l   i   n   e

$ wc -c out-stdin.bin out-file.bin
40 out-stdin.bin
40 out-file.bin
```

事实：**只要写进文件的是同一串 `Body` 字节，stdin 与文件的输出逐字节相同**，包括 `\r`、UTF-8、制表符与「结尾无换行」。

### 3.5 BOM / CRLF / shebang

```
$ printf '\xef\xbb\xbf#!/bin/bash\necho bom-file-ok\n' > /dev/shm/bom.sh; chmod 755 /dev/shm/bom.sh; /dev/shm/bom.sh
/dev/shm/bom.sh: 行 1: ﻿#!/bin/bash: 没有那个文件或目录
bom-file-ok
[rc=0]

$ printf '\xef\xbb\xbf#!/bin/bash\necho bom-file-ok\n' > /dev/shm/bom.sh; bash /dev/shm/bom.sh
/dev/shm/bom.sh: 行 1: ﻿#!/bin/bash: 没有那个文件或目录
bom-file-ok
[rc=0]

$ printf '\xef\xbb\xbf#!/bin/bash\necho bom-stdin-ok\n' | bash -c '. /proc/self/fd/0'
/proc/self/fd/0: 行 1: ﻿#!/bin/bash: 没有那个文件或目录
bom-stdin-ok
[rc=0]

$ printf '#!/bin/bash\r\necho crlf-file\n' > /dev/shm/crlf.sh; chmod 755 /dev/shm/crlf.sh; /dev/shm/crlf.sh
run.sh: /dev/shm/crlf.sh: /bin/bash^M: 错误的解释器: 没有那个文件或目录
[rc=126]

$ printf '#!/bin/bash\r\necho crlf-file\n' > /dev/shm/crlf.sh; bash /dev/shm/crlf.sh
crlf-file
[rc=0]

$ printf 'echo one\r\necho two\r\n' | bash | od -c
0000000   o   n   e  \r  \n   t   w   o  \r  \n

$ printf 'echo one\r\necho two\r\n' | bash -c '. /proc/self/fd/0' | od -c
0000000   o   n   e  \r  \n   t   w   o  \r  \n
```

事实：

- BOM：`bash file` 与 stdin **都**报第一行、**都**继续执行到 rc=0。差别不在载体，在「有没有剥 BOM」；现状 `ScriptBodyOf` 会剥，所以现状不报这一行。
- CRLF：`bash file` 与 stdin 的输出一致（`\r` 原样保留）；**只有直接执行**才因为 `#!/bin/bash\r` 而 rc=126。

### 3.6 空间耗尽（只写不执行）

```
$ dd if=/dev/zero of=/dev/shm/fill.bin bs=1M count=11
9+0 records out
9437184 bytes (9.4 MB, 9.0 MiB) copied
dd: IO error: No space left on device
-rw-rw-r-- 1 root root 10465280 Oct 22 13:51 /dev/shm/fill.bin
Filesystem      Size  Used Avail Use% Mounted on
tmpfs            10M   10M     0 100% /dev/shm

$ printf 'echo x\n' > /dev/shm/after-full.sh; echo write_rc=$?
printf: 写入错误: 设备上没有空间
write_rc=1

$ /bin/rm -f /dev/shm/fill.bin; df -h /dev/shm
tmpfs            10M   20K   10M   1% /dev/shm
```

事实：ENOSPC 时**已经写入的部分留在文件里**（10465280 字节），只靠返回码判断会留下「看起来存在、内容截断」的脚本。若流程是「写 → 不校验长度 → 执行」，就会跑半个脚本。

### 3.7 权限 / umask / 其他用户可读性

```
$ (umask 022; printf 'echo secret-password\n' > /dev/shm/m022.sh)
$ (umask 077; printf 'echo secret-password\n' > /dev/shm/m077.sh)
$ ls -l /dev/shm/m022.sh /dev/shm/m077.sh
-rw-r--r-- 1 root root 21 Oct 22 13:51 /dev/shm/m022.sh
-rw------- 1 root root 21 Oct 22 13:51 /dev/shm/m077.sh
```

- 默认 umask 022 → **644，同机任何用户可读**；脚本里的口令、内嵌 token 一览无余。
- 实测 `mktemp` 建的文件是 600（`-rw-------`），目录是 700；这比手写 `> file` 安全，但仍取决于实现是否用 `mktemp`。
- 用 `su` 切到 uid 1000 失败（`su: 认证失败`，本机无该账号口令），故「其他用户实际能否读」未直接复现；644 的位已足够说明暴露面。
- 本机 `/dev/shm` 是 755；客户机通常 1777，**其他用户可读目录并可抢注文件名**（见下条）。

### 3.8 解释器：python 三种形态、perl 带参数

```
-- stdin（现状：裸 python3 读 stdin）
argv0=[]
file=[<stdin>]
argv=[['']]

-- 文件（python3 file.py x）
argv0=[/dev/shm/a.py]
file=[/dev/shm/a.py]
argv=[['/dev/shm/a.py', 'x']]

-- 现状的 -a 形态：python3 -c '<程序文本>' scriptname
argv0=[-c] file=[None] argv=['-c', 'scriptname']

-- 解释器带参数（perl -w）文件形态
$ env -i PATH=/usr/bin:/bin perl -w /dev/shm/p.pl
perl-ok
[rc=0]
```

事实：

- 文件形态让 `sys.argv[0]`、`__file__` 从空 / `<stdin>` 变成真实路径，且能自然接收参数——这是**改善**。
- 现状 `-a` 的 `python3 -c <正文> 脚本名` 确实把脚本名塞进了 `sys.argv[1]`（`command.go:296-299` 已把这个 wart 写进注释），文件形态消除了它。
- `interpreter` 带参数（`perl -w`）时，文件形态就是 `perl -w file`，语义清楚；现状 `-a` 是 `perl -w -e <正文>`，也能跑，但每家的程序文本参数都得在表里维护。

### 3.9 退出码 / 信号 / 残留 / 并发同名

```
$ printf 'exit 7\n' > /dev/shm/ec.sh; bash /dev/shm/ec.sh; echo file_rc=$?
file_rc=7
$ printf 'exit 7\n' | bash -c '. /proc/self/fd/0'; echo stdin_rc=$?
stdin_rc=7

$ printf 'kill -TERM $$\n' > /dev/shm/sig.sh; bash /dev/shm/sig.sh; echo file_sig_rc=$?
已终止
file_sig_rc=143
$ printf 'kill -TERM $$\n' | bash -c '. /proc/self/fd/0'; echo stdin_sig_rc=$?
已终止
stdin_sig_rc=143

-- 失败时的残留（写后执行失败，不清理）
$ printf '#!/bin/bash\r\nexit 0\n' > /dev/shm/leftover.sh; bash /dev/shm/leftover.sh; echo rc=$?; ls -l /dev/shm/leftover.sh
rc=0
-rw-rw-r-- 1 root root 20 Oct 22 13:51 /dev/shm/leftover.sh

-- 并发同名
$ (printf 'echo A\n' > /dev/shm/same.sh & printf 'echo B\n' > /dev/shm/same.sh & wait); cat /dev/shm/same.sh
echo B

-- 唯一名字
$ mktemp -p /dev/shm sshfleet.XXXXXXXX.sh
/dev/shm/sshfleet.rYvWwqoB.sh
-rw------- 1 root root 0 ...
```

事实：同一会话内，退出码与信号行为与载体无关（7→7、143→143）。新引入的是**残留**（不清理就会留）与**同名覆盖**（固定名必被覆盖）。

### 3.10 source 语义 vs 文件语义（现状与乙的实质差异）

```
$ printf 'echo before\nreturn 3\necho after\n' > /dev/shm/ret.sh
$ bash -c '. /proc/self/fd/0' < /dev/shm/ret.sh; echo source_rc=$?
before
source_rc=3
$ bash /dev/shm/ret.sh; echo file_rc=$?
before
/dev/shm/ret.sh: 第 2 行： return: 只能在函数或者被 source 的脚本中使用 "return"
after
file_rc=0
```

```
$ bash /dev/shm/who.sh
zero=/dev/shm/who.sh
base=who.sh
dir=/dev/shm
src=/dev/shm/who.sh

$ printf '...' | bash -c '. /proc/self/fd/0' 原始名.sh
zero=原始名.sh
base=原始名.sh
dir=.
```

事实：现状的 `-s` 是 **source**，文件形态是 **执行**。差别不只是 `$0`：

- 顶层 `return N`：现状**立即返回并把 N 当退出码**；文件形态**报错、继续执行后续行、最终退出码可能是 0**（上例 `file_rc=0`）。这是输出污染 + 退出码语义变化。
- `BASH_SOURCE`：`/dev/stdin` → 文件路径。
- `dirname "$0"`：`.` → `/dev/shm`。**两者都找不到脚本本地的伴随文件**（现状也没有伴随文件），但路径不同，依赖它的脚本行为会变。

### 3.11 符号链接跟随（安全面）

```
$ ln -sf "$PWD/victim.txt" /dev/shm/predictable.sh
$ printf 'echo attacker-body\n' > /dev/shm/tmpwrite.sh
$ cat /dev/shm/tmpwrite.sh > /dev/shm/predictable.sh
$ echo "victim=[$(cat victim.txt)]"
victim=[echo attacker-body]
```

事实：对 `/dev/shm` 里的符号链接做 `> path` 会**穿过链接改写目标文件**。`sftpClient.Create`（`sftp.go:426`）发的是 `SSH_FXF_WRITE|CREATE|TRUNC`，服务端同样 follow-symlink，没有 `O_NOFOLLOW`/`O_EXCL`。在 1777 的 `/dev/shm` 上，这就是可预测文件名 + 符号链接的经典 tmpfile 面；若写入者以 root 运行，后果更重。

### 3.12 唯一目录 + 原名（保留 basename 的一种落法，只作事实记录）

```
$ d=$(mktemp -d -p /dev/shm sshfleet.XXXXXXXX); printf 'echo zero=$0\n' > "$d/原始名.sh"; chmod 700 "$d"; bash "$d/原始名.sh"
zero=/dev/shm/sshfleet.D2YDS6pk/原始名.sh
700 /dev/shm/sshfleet.D2YDS6pk
664 /dev/shm/sshfleet.D2YDS6pk/原始名.sh
```

事实：目录 700、内部文件 664（受 umask 影响）。`basename "$0"` 能拿回原名，但 `$0` 本身是完整路径。若脚本用 `$0` 做字符串比较（而不是 `basename`），结果不同。

---

## 四、这个方案**不解决**的清单

以下问题与「正文放在 stdin 还是文件」正交，落文件一点都帮不上，必须另行处置：

1. **locale 取值本身**（`C` 还是 `en_US.UTF-8`）。文件不决定 locale，`[env]` 的取值照旧由配置决定（ADR-0024 决定 1）。
2. **`export` 穿过 `sudo` 的能力**。`export` 仍在 `sudo` 之前、仍在同一层外壳里（`command.go:277-283`），`env_reset`/`env_check`/`env_keep` 的行为不变；能穿过的机器照旧穿过，不能的照旧不能。
3. **`PATH` 被 `secure_path` 覆盖**（ADR-0024 §9.2）。这是 sudo 内部行为，与载体无关。
4. **目标机 locale 未生成 / 翻译包缺失**。选 `C` 时永远存在；选 `en_US.UTF-8` 而机器没生成时，照旧每次 bash 多一行、perl 多 7–19 行警告（ADR-0024 §3.5）。文件形态不生成 locale。
5. **字符语义（按字节 vs 按字符）**。`awk`/`cut -c`/`${#s}`/`sort` 在 `C` 下的行为由 locale 决定，不由载体决定（ADR-0024 §3.7/§3.8）。
6. **`-a` 的 PTY 下非 BMP 字符变 `?`**（ADR-0024 §9.1，coreutils 的显示宽度判定）。文件形态不改变 PTY，也不改变 coreutils。
7. **目标机没有配置的解释器**。文件形态仍要解释器存在；它甚至更需要解释器（现状非 shell 家族可以裸读 stdin，文件形态必须显式调用）。
8. **目标机 sshd 不允许 exec / 命令白名单**。不解决；反而新增 sftp 依赖，限制更多。
9. **`--no-shell` 的「原样下发」契约**。不解决，且**不相容**：要落文件就得注入「解释器 + 可能的环境」，那正是 `--no-shell` 承诺不做的事（`command.go:107-116`、`cli/args.go:128-134`）。
10. **`-u` / `-d` / `--change-password` 三条路径**。它们不经过可配解释器，也不经过这段正文（`internal/cli/interpreter.go:1-10` 的注释），零收益。
11. **输出侧的 CR/CRLF 归一与乱码显示**。`trimOuterBlankLines` 已独立处理（`exec.go:145-167`），与正文载体无关。
12. **脚本本地的伴随文件**。脚本被放进 `/dev/shm` 后，`dirname "$0"` 指向 `/dev/shm`，那里没有它的 `lib/`、`conf/`；现状至少 `$0` 是 basename、`dirname` 是 `.`（当前工作目录）。两种形态都拿不到伴随文件，文件形态也不会让它变好。
13. **「同批机器结果逐字可比」**。这是 locale/PATH/字符语义共同决定的（ADR-0024 的核心诉求），正文载体不在其中。

---

## 五、三条候选形态的对比（只列事实与代价，不替作者选）

- **甲：现状**——`-c`/`-s` 走 stdin，`-a` 走 base64 argv + PTY。
- **乙：`/dev/shm` 落文件 + 解释器执行，环境段照旧 `export`**。
- **丙：`/dev/shm` 落文件 + 解释器执行，不注入任何环境**（完全跟随目标机）。

| 维度 | 甲 现状 | 乙 文件 + 环境 | 丙 文件 + 无环境 |
| --- | --- | --- | --- |
| 正文通道 | `-c`/`-s`：stdin；`-a`：argv(base64) | 文件（SFTP 或第二个 exec 会话） | 同乙 |
| 体积上限 | `-c`/`-s` 流式无上限；`-a` 下发行 122880 字节（正文约 92KB） | `/dev/shm` 容量（本机 10MB；容器常见 64MB；物理机约内存一半） | 同乙 |
| 目标机依赖 | 只需 exec 通道 | 可写的 `/dev/shm` **+** 可用 sftp（或第二个会话） | 同乙，**再减去**对 `export` 的依赖 |
| `ps` 可见性 | `-a` 正文在 argv，同机可见 | 正文不在 argv | 同乙 |
| 落盘可见性 | 无 | 文件；0644 时同机可读，重启前一直在 | 同乙 |
| PTY 回显 | `-a` 回显整条 base64 | 回显短命令（改善） | 同乙 |
| `$0` | 原 basename（shell 家族） | 路径；`basename` 仍是原名 | 同乙 |
| 位置参数 | 不支持（`$#=0`） | 可支持 | 可支持 |
| 脚本顶层 `return` | 立即返回，退出码=参数 | 报错后继续，退出码可能变 0 | 同乙 |
| 解释器范围 | `-a` 限 `interpreterTable` 15 条；`-c`/`-s` 不限 | 任意能读文件的解释器（`-a` 的表内限制消失） | 同乙 |
| 与 U17 的关系 | `-a` 受表约束 | 更宽松；但**不能走 shebang**（会绕过配置） | 同乙 |
| 交互 / 代填 | PTY + stdin 全给答案 | PTY + stdin 仍全给答案；正文须另走 sftp | 同乙 |
| 环境 | `export` 在解释器前；locale 由配置定 | **完全不变** | **不注入**：locale/PATH/TERM 全跟随目标机 |
| 同批可比性 | 由配置统一（ADR-0024 的初衷） | 不变 | **失效**：每台机器各按自己的 locale/PATH 跑 |
| `sudo` | 环境能穿过的照旧穿过 | 不变 | 仍受 `env_reset`；且不注入就没有可穿的东西 |
| 原有坑 | `-a` 体积、base64 可读性 | 全部消失 | 全部消失 |
| 新增失败面 | — | 容量/不存在/只读/ENOSPC 截断/`noexec`(仅直接执行)/SELinux/sftp 禁用/并发同名/残留/644 泄露/符号链接/多一次往返 | 同乙 **+** 2026-10-07 的 `PATH` 坑与「locale 取决于机器」回归 |
| 残留 | 无 | 需 `trap` 清理；超时/断连/SIGKILL 必留 | 同乙 |
| 日志可复原正文 | `-a` 可（base64+原文） | 只剩路径，执行后失效 | 同乙 |

**丙 的额外事实**（不是评价）：

- 不注入环境后，`sudo` 仍会 `env_reset`；但此时没有 `LANG`/`LC_ALL`/`TERM` 可被保留，程序看到的是目标机默认值。
- `PATH` 回到非交互 sshd 的精简值（常为 `/usr/bin:/bin`），`sudo` 的 `secure_path` 只影响 sudo 之后的命令；`/usr/local/bin`、`/opt/...` 里的解释器可能 `command not found`——这正是 `command.go:11-20` 记录的、当初引入 `export` 的那次 bug。
- `TERM=dumb` 消失，彩色与转义序列会进入输出，影响「逐字比对」。
- 与 ADR-0024 的**决定 1（取值）** 并不直接冲突（丙 只是不注入，不规定取值），但与**决定 2（拼接形态不变）** 以及「同批机器逐字可比」的初衷正面冲突；这一点是事实，取舍留给作者。

---

## 六、新引入的风险（按后果排序）

- **H1 截断写入被执行。** ENOSPC / 连接中断会留下「长度不足但存在」的文件；若「写 → 执行」之间没有长度校验或原子 rename，就会跑半个脚本（安装类脚本尤其危险）。实测：11MB 只写成功 9.4MB，文件仍在。
- **H2 符号链接 / 可预测文件名 → 任意文件覆盖。** `/dev/shm` 通常 1777，`sftp.Create` 与 `> path` 都 follow-symlink；以 root 写入时可覆盖任意 root 可写文件。实测已复现穿透写入。必须随机名 + `O_EXCL`/`O_NOFOLLOW`（`pkg/sftp` 的 `Create` 不提供，需要 rename 或先 `Lstat`）。
- **H3 644 残留泄露。** 默认 umask 022 + 脚本内嵌口令/令牌 → 同机任意用户可读，且重启前一直在。实测 644。现状不落盘，没有这个面。
- **H4 新增 sftp 硬依赖。** `Subsystem sftp` 被关、被 chroot、或客户只放行 exec 时，`-a` 从「能用」变「不能用」。现状只需 exec。
- **H5 `/dev/shm` 容量/存在性/只读。** 本机 10MB；容器 64MB 常见；精简系统可能没有。现状无此依赖。
- **H6 并发同名覆盖。** 固定名必被后写者覆盖；同机并发两次运行会执行到对方的正文。实测只剩后写内容。
- **H7 语义变化：source → 执行。** 顶层 `return` 从「立即返回」变成「报错继续」，退出码可能从 N 变 0；`$0` 从 basename 变路径；`BASH_SOURCE` 变路径。实测 `source_rc=3` vs `file_rc=0`。
- **H8 日志失去正文可复原性。** `-a` 现在能从 base64 + 原文复原；文件形态只剩一个执行后即删除的路径。排查「到底跑了什么」会失去证据。
- **H9 失败点与时延翻倍。** 每台多一次 sftp 握手/写入；2000 台的总时延与失败概率都乘以「多一步」。
- **H10 `noexec` 只在直接执行时爆。** 若实现走 `./file` 或 shebang，hardened 机器（CIS 常见 `noexec,nosuid,nodev`）会 126；走 `<解释器> <文件>` 则不受影响。实测两者行为差异明确。
- **H11 BOM/CRLF 的回退。** 若绕过 `ScriptBodyOf` 直接上传原始文件，现状已修掉的 BOM 首行报错会回来；若走 shebang，CRLF 会把「注释里的 `\r`」升级成「解释器路径带 `\r`」（126）。实测两者都有。

---

## 七、未测 / 不可判

1. **真实目标机的 `/dev/shm`**：容量、是否 `noexec`、是否 1777、是否被 systemd 的 `tmp.mount` 覆盖。本机只有 10MB / `nosuid,nodev` / 755。
2. **SELinux / AppArmor 的实际拒绝**：本机无 SELinux，AppArmor 无相关 profile；只能从机制上说「新增了一个 tmpfs 访问面」。
3. **sudo 路径**：本机 `sudo` 不可用，无法实测「登录用户写文件 → sudo 解释器读」的属主/权限组合；也无法实测 `secure_path` 对新形态的影响（按 ADR-0024 §9.2，预期不变）。
4. **不同 sshd 的 sftp 可用性**：未测 `Subsystem sftp` 关闭、`internal-sftp` + `ChrootDirectory`、只允许 exec 的配置。
5. **2000 台并发下的时延与 sftp 连接数**：未做规模测试。
6. **真实脚本对 `$0` / `return` / `dirname "$0"` 的依赖分布**：只证明了语义差异存在，无法量化有多少脚本会因此改变行为。
7. **本机 `/dev/stdin` 异常的真实成因**：同一命令在不同调用里时好时坏；`cat` 与 `source /proc/self/fd/0` 正常，`source /dev/stdin` 间歇性 `ENOENT`。判定为探针环境问题，但**现状 `-s` 恰好依赖 `. /dev/stdin`**，建议另行在真实目标机上复核一次（与本方案无关的既有前提）。
8. **`pkg/sftp` 的 `Create` 在服务端的确切 flags**：依据库行为与实测符号链接穿透判断为 follow-symlink + `O_TRUNC`，未逐版本核对源码。

---

## 八、复现材料

- 探针脚本：`build/shm-analysis/run.sh`（主套件）、`build/shm-analysis/run2.sh`（补充）。
- 原始输出：`build/shm-analysis/results.txt`（325 行）、`build/shm-analysis/results2.txt`。
- 产物：`out-stdin.bin` / `out-file.bin`（`cmp` 用）、`probe.sh`、`a.py`、`bytes.sh`。
- 复现命令：`cd build/shm-analysis && bash run.sh && bash run2.sh`。
- 注意：`/dev/shm` 在本机会随每次工具调用重建，`run.sh` 必须在一次调用内跑完；`run2.sh` 的 `su` 段在本机会因无口令失败（已在文中标注）。
