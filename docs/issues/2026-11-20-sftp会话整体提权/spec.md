# SFTP 会话整体提权：--sudo 直接写最终路径，撤掉暂存目录与 mv

- **Status**：done
- **日期**：2026-11-20
- **关联**：ADR-0063 决定 1、决定 2；ADR-0026（空间预检只剩目标侧）
- **前提**：无。本议题是《超时收尾与残留》的前置。

## Problem Statement

使用者以 `--sudo` 上传时，文件并不是直接写到目标目录：先落到目标机 `/tmp` 下一个 `chmod 777` 的暂存目录，再用 `sudo mv` 搬到最终位置。这带来三个使用者看得见的麻烦：

1. 从开始写到搬走之间，目标机上任何账号都能往那个目录里写——几千台的场景下，这不是理论风险；
2. 暂存目录与目标目录不在同一个分区时，`mv` 退化成复制加删除，文件被写两遍，中途失败还会在最终位置留下半成品；
3. 暂存分区装不下单个最大文件时，工具直接判「空间不足」，哪怕目标目录所在分区还有大量空间。

## Solution

`--sudo` 时整条 SFTP 会话以 root 跑，每个文件直接写到它的最终路径。使用者看到的变化：

- 上传不再占目标机的 `/tmp`，也不再出现「暂存分区空间不足」这一类失败；
- 目标机上不会留下 `chmod 777` 的目录，写的过程中也没有别的账号能插进来；
- 落位文件的属主从登录账号变成 root（ADR-0063 决定 1 已接受）；
- 探不到 `sftp-server`、或 sudo 拒绝时，明确报失败，不悄悄退回旧路。

## User Stories

1. 作为运维，我用 `--sudo` 上传时，希望文件直接出现在目标路径，而不是先经过一个谁都能写的暂存目录。
2. 作为运维，我希望目标目录与 `/tmp` 不同分区时，上传不必把文件写两遍。
3. 作为运维，我希望 `/tmp` 很小但目标分区很大时，上传照样能成。
4. 作为运维，当目标机上找不到 `sftp-server`（或 sudo 不让我提权）时，我希望结果里明确写出这件事，而不是一个含糊的「上传失败」。
5. 作为运维，我不希望在目标机上留下工具建的任何目录。

## Implementation Decisions

### 提权形态

`--sudo` 且登录账号不是 root 时，不再把 `sftp.NewClient(c.conn)` 当作读写通道，而是：

1. 一条 exec 会话跑 `sudo -n sh`，探测脚本从 stdin 递进去，避免引号套引号；输出就是 `sftp-server` 的绝对路径。
2. 另起一条 exec 会话跑 `sudo -n <路径>`，把这条会话的 stdout 交给 SFTP 客户端的读端、stdin 交给写端（`sftp.NewClientPipe`）。

非 sudo 路径不变，仍是 `sftp.NewClient(c.conn)`。

### 探测与失败

候选路径依次为 `/usr/lib/openssh/sftp-server`、`/usr/libexec/openssh/sftp-server`、`/usr/lib/ssh/sftp-server`，都不在时退到 `command -v sftp-server`。

- 探到：打印路径，退出 0。
- 探不到：退出 127。
- sudo 拒绝：`sudo` 自己退出非 0 且不是 127，stderr 是它的原话。

后两种都按失败记，环节名统一为 `client_failed`（这类失败连「开始传」都做不到，与「目标机不支持 SFTP」是同一档），描述里分开写清是哪一种：`sftp-server not found under sudo` 与 `sudo refused to run sftp-server`。原文附在后面。

不做退回：探不到就失败，不回到暂存目录那条路（ADR-0063 决定 1「没有退路」）。

### 客户端形状

`*sftp.Client` 在提权路径下还挂着一条会话，不关它就会漏一条通道。用一个窄类型收住：内嵌 `*sftp.Client`，多存那条 `*ssh.Session`，`Close` 时先关客户端再关会话。上传、下载、回滚、空间预检的参数类型从 `*sftp.Client` 换成它，行为不变。

### 只留一条写路径

撤掉 `putFileViaSudo` 与它带的一切：暂存目录、`chmod 777`、每文件的 `sudo mkdir -p`、`sudo mv`、上传开头那次 `sudo rm -rf` 清暂存根。

两种模式都走同一条写路径：中间目录由 SFTP 自己 `MkdirAll`（会话已是 root，不必再发一条 shell 命令）。

### 空间预检只剩一侧

目标目录所在分区照旧用 `StatVFS` 探，提权时按 root 的视角算可用量（`Bfree`），非提权按登录账号（`Bavail`）。暂存侧那一支连同「`/tmp` 空间不足」整条判据一起撤掉（ADR-0026 的暂存侧作废）。

### 退役的符号与配置

- 代码：`tmpRoot`、`tmpProbePath`、`maxFileSize`、`sideTmp`、`transferTmpFailed`、`transferSudoFailed`、`transferTmpSpaceShort`、`putFileViaSudo`、`randomHex`。
- 配置：`keywords_error.conf` 里「`/tmp` 空间不足」与「sudo失败`」两组整块删除；「目标机不支持 SFTP」那一组的注释与 tip 补上「sudo 下找不到 sftp-server 或 sudo 拒绝」这一种。

## Testing Decisions

**接缝**：`execute.Run`——执行站入口，递一张纸、收一张纸（ADR-0037 已定的三种接缝之一）。测试只从这一处进，断言那张纸与假目标机上的文件，不碰内部函数。

**提权这条路只有真目标机验得了**：假目标机在测试进程内，没有 sshd、没有 sudo，也没有 `sftp-server`。它住在 `internal/execute` 的集成用例里，按既有约定靠环境变量开关，缺变量就 skip；能跑的人用 WSL 那份 sshd（账号在 NOPASSWD sudo 组内），目标目录取一个 700、属主 root 的目录。

**自定义区单测**：空间预检撤掉暂存侧之后只剩一条判据——按「够」「不够」「预检与失败后再查」三种写法各留一条，断言报文原文，不经过真机器。

**判据**：既有红线用例不动。红线区只引用 `execute.Run`，本次不改它的名单与指纹。

## Out of Scope

- **传输超时与超时后的清理**：由《超时收尾与残留》承接，本议题一个字不碰 `timeout_transfer`。
- 枚举改 NUL 分隔（ADR-0063 决定 4）。
- 单文件重试与写完核大小（ADR-0063 决定 9、10、11）。
- 下载前本地重名（ADR-0063 决定 7）。
- 既有两条回滚的删法（`rolled_back`／`rollback_partial` 仍是逐个删）。
- 下载侧的路径预检命令：它是 exec 上的 `sudo test -e`，与 SFTP 身份无关，本次不动。
