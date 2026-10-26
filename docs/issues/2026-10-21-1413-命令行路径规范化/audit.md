# Windows 侧基线：路径取名与后缀规则（2026-10-26）

Status: ready-for-human

**这份文件是什么**：工单 02（`issues/02-path-ownership-libs.md`）的验收在 Linux 上验不出来——`path` 与 `filepath` 的差别按 `GOOS` 在**编译期**就定了，交叉编出来的 Windows 二进制在 Ubuntu 上跑不了，`GOOS=windows go test` 只能编译不能执行。所以「验」这一步交给 Windows 环境里的一台机器（同一个仓库，经 git 同步）。

**本文两用**：第二节起是**要跑什么**；第五、六节是**结果底本**，由 Windows 侧的执行者填。改前跑一次填第五节，代码改完再跑一次填第六节，两栏对照即为工单 02 的证据。

**谁能改这个文件**：只有 Windows 侧的执行者在填结果时能改，且只改第五、六节与 `Status:`/`Comments`。代码一行不许动。

---

## 一、边界

- **只验证，不改代码。** 你要做的是「测量并如实报告」，不是「修好它」。
- 唯一允许写的是仓库 `build\win-verify\`（沙盒，已 gitignore）与**本文件**。沙盒跑完别删，改后那一轮还要用。
- **不许跑 `build-windows.bat`**：它先 `rmdir /s /q release\` 再打整包，会把 `release\` 的发布快照换掉。本轮只要一个 exe。
- **不许用 WSL / Git Bash 里的 go**：必须 Windows 原生 go，`go env GOOS` 要回 `windows`。WSL 里编的是 linux，`filepath` 行为完全不同，测了等于没测。
- 仓库里的 `config\SSHFleet.conf` 是作者本机配置（Linux 路径 + 加密凭据），**不要动它**，按 4.2 另拷一份到沙盒。
- 拿不到 SSH 目标机（D4/D5 要）就写「未做」，**不许编、不许拿假数据凑**。
- **不要靠确认屏按 y/n**：那是全屏交互，你按不了键。一律用 `--yes`，从工具日志里读值（见第四节）。

---

## 二、先确认代码处于「改前」状态（30 秒，先做这个）

```powershell
Select-String -Path modules\SSHFleet_Go\internal\cli\args.go  -Pattern "func normalizePath"
Select-String -Path modules\SSHFleet_Go\internal\ssh\sftp.go  -Pattern "filepath.Base\(remotePath\)"
```

**两条都命中 = 改前状态，继续。**

命中不了 = 代码已被改过（或你在别的分支上）。**立刻停止**，把 `git log -1 --format="%h %ad %s"` 与这两条的输出写进 5.1 与 5.5，不要往下跑——在错误的代码状态上测出来的数没有用。

背景（愿意看就看）：

- 要验的工单：`docs/issues/2026-10-21-1413-命令行路径规范化/issues/02-path-ownership-libs.md`
- 规格：`docs/issues/2026-10-21-1413-命令行路径规范化/spec.md`
- 决策：`docs/adr/0025-命令行路径原样交给系统.md`

---

## 三、任务 A / B / C

### 3.1 A：环境自检

```powershell
$env:GOOS=""; $env:GOARCH=""
go version
go env GOOS GOARCH GOPATH
Set-Location <仓库根>
git rev-parse --short HEAD
git status --short
```

判据：`go version` ≥ 1.27（`go.mod` 写的是 `go 1.27`）；`go env GOOS` 是 `windows`；`git status --short` 应为空（有改动先报，不要自己清理）。

> **坑**：这个 shell 若之前跑过 `build-windows.bat`，`GOOS` 会残留在 `linux` 上。第一行就是清它；清不掉就问作者。

### 3.2 B：构建

```powershell
Set-Location <仓库根>\modules\SSHFleet_Go
New-Item -ItemType Directory -Force ..\..\build\win-verify | Out-Null
$env:GOOS="windows"; $env:GOARCH="amd64"
go build -o ..\..\build\win-verify\SSHFleet.exe .
```

判据：exe 存在，`.\SSHFleet.exe --help` 能打出帮助（首行下方应有版本号）。

首次构建可能联网拉依赖；报下载失败先 `go mod download`，仍失败照实报，不要自己换代理。

### 3.3 C：测试基线（**本次最高优先级**）

```powershell
Set-Location <仓库根>\modules\SSHFleet_Go
$env:GOOS=""
go mod download
go test ./... 2>&1 | Tee-Object -FilePath ..\..\build\win-verify\test-baseline.txt
```

要特别注意：

- `internal/cli` 的 `TestParseDoesNotRewriteWindowsStylePath` 应当是 **SKIP**（它自己判 `runtime.GOOS == "windows"` 就跳过），不是 PASS 也不是 FAIL；
- 任何 `FAIL` / `build failed` / `no such tool` 一律照实记；
- 有 SKIP 的用例也记名字，那同样是基线；
- 带 `[警告]` 之类非测试行也照抄（Linux 侧有一条主密钥指纹不一致的警告，Windows 上可能没有或不同）。

**不许自己判断「这个红应该是环境问题，忽略它」。** 是不是环境问题由改动后的对比决定，不由你提前裁掉。输出贴进 5.2，**不要提交 txt**。

---

## 四、任务 D / E：五条验收与两条顺手确认

### 4.1 沙盒准备

```powershell
Set-Location <仓库根>\build\win-verify
New-Item -ItemType Directory -Force config,scripts\a,scripts\dir.v2,dl | Out-Null
Copy-Item ..\..\modules\SSHFleet_Go\config\SSHFleet-for-windows.conf config\SSHFleet.conf -Force
"echo hello" | Set-Content -Encoding ascii scripts\a\b.sh
"hi"         | Set-Content -Encoding ascii scripts\a\b.txt
"deploy"     | Set-Content -Encoding ascii scripts\dir.v2\deploy
"127.0.0.1,22,root,x" | Set-Content -Encoding ascii nodes-dummy.csv
```

> **必须用 `-Encoding ascii`，不要用 `utf8`。** Windows PowerShell 5.1 的 `utf8` 会写 BOM，BOM 进了 csv 第一列（IP 变成 `\ufeff127.0.0.1`）、进了 sh 首行（远端会报 `echo: command not found`）。这几个文件内容全是 ASCII，用 `ascii` 最省事。

再改 `config\SSHFleet.conf` **两行**：`[account]` 的 `password` 填明文密码；`[credential]` 的 `encrypt = true` 改 `encrypt = false`。

> 程序固定从 `./config/SSHFleet.conf` 读配置（相对当前目录），所以**所有命令都在 `build\win-verify` 这一层执行**。

### 4.2 为什么看日志而不是看屏

程序会把解析结果写进 `history\SSHFleetTools.log`，其中 `r='...'` 是自动备注、`s=` 脚本、`u=` 上传、`d=` 下载、`p=` 目录。`--yes` 会跳过确认屏直接去连机器——D1–D3 用连不上的 `127.0.0.1` 正好：连接失败不影响那几行日志，而那才是要看的。

> **但日志不是万能的**：那一行写在「选项/参数合规性检查通过」之后（`main.go:275`）。**命令被合规检查拦下时，日志里根本没有 `r=`**，只有终端上的报错文案。所以：D1–D3 在改前都合规，走日志；E1 的改后会失败，那时只能看报错原文。

### 4.3 D1–D3：不需要目标机

```powershell
$here = (Get-Location).Path

.\SSHFleet.exe -f nodes-dummy.csv -s "$here\scripts\a\b.sh" --yes
.\SSHFleet.exe -f nodes-dummy.csv -u "$here\scripts\a\b.txt" -p /opt/ --yes
.\SSHFleet.exe -f nodes-dummy.csv -s "$here\scripts\dir.v2\deploy" --yes
```

每条跑完，**立刻**抄这两样（别三条全跑完再抄，日志是追加的，混在一起分不清哪条是哪条）：

```powershell
if (Test-Path history) { Get-ChildItem history | Sort-Object LastWriteTime | Select-Object -Last 1 -ExpandProperty Name }
if (Test-Path history\SSHFleetTools.log) { Get-Content history\SSHFleetTools.log -Tail 30 }
```

| # | 看哪一格 | 顺带记一格 | 改后应该是（判据，不是现在的预期） |
| --- | --- | --- | --- |
| D1 | `r='...'`（自动备注） | `s='...'`（脚本路径） | `r='b.sh'`；`s=` 原样是 `D:\...\b.sh` |
| D2 | `r='...'` | `u='...'` | `r='b.txt'`；`u=` 原样是 `D:\...\b.txt` |
| D3 | 含「脚本后缀」的那行 Trace：`非交互模式（--yes）跳过确认：脚本后缀 "…" 未配解释器…` | `s='...'` | 后缀是空串，即 `""` |

**`s=` / `u=` 这两格比备注更有说服力**：它们记的是进入后续环节的实际值。按代码推断，改前它们会是 `D:/.../b.sh`（`\` 已被换成 `/`），改后应当是 `D:\...\b.sh` 原样——这正是工单 01 要的「逐字相同」。**这是推断，以你测出来的为准。**

`history\` 下的归档目录名（形如 `2026-10-26_11-02-33_脚本_b.sh`）也可以抄，但它只在执行阶段才建；连接失败时可能没有，没有就跳过。

> D3 若日志里没有那行 Trace，照实报告，不要自己下结论（可能是后缀被当成已配置，或这次没走回退分支）。

### 4.4 D4–D5：要目标机

```powershell
"IP,22,root,PASSWORD" | Set-Content -Encoding ascii nodes.csv   # 换成真值，别留占位符

.\SSHFleet.exe -f nodes.csv -d /tmp/app.log -p .\dl\ --yes
.\SSHFleet.exe -f nodes.csv -d "/tmp/a\b.log" -p .\dl\ --yes
```

目标机上先造文件：

```bash
echo x > '/tmp/app.log'
echo y > '/tmp/a\b.log'
```

记录：终端明细里的文件名、`dl\` 里实际落了什么、有没有失败提示（原文）。

| # | 改后应该是 |
| --- | --- |
| D4 | 明细文件名 `app.log`，下载成功，`dl\app.log` 存在 |
| D5 | 明细文件名是整段 `a\b.log`，按下载失败报出，`dl\` 里**不多出文件** |

### 4.5 E1–E2：顺手确认

```powershell
$here = (Get-Location).Path     # 每个新开的 PowerShell 窗口都要重设一次

.\SSHFleet.exe -f nodes-dummy.csv -u "$here\scripts\a\b.txt" -p '\opt\app\' --yes
.\SSHFleet.exe -f nodes-dummy.csv -d '/tmp/a\b.log' -p .\dl\ --yes
```

- **E1**：看 `p='...'`。若 `p='/opt/app/'`，说明改前确实把 `\` 换成了 `/`（于是凑巧通过了「必须以 / 开头」）；改后同一条会**被合规检查拦下，日志里没有 `p=`**，只能看终端报错——报错里应当回显 `\opt\app\` 原值。这是工单 01 的判据。
- **E2**：看 `d='...'`。若 `d='/tmp/a/b.log'`，就回答了 4.6 第 3 点，D5 的目标机可以省掉。

### 4.6 记录纪律与三条待证推断

**不要按任何「预期」去凑数。** 我这边没有 Windows 真机，对现状的推断已经错过一处，所以只记**你看到的原文**。日志里出现 `_` 替换掉 `\` 或 `:` 的形态（备注过了一遍 `SanitizeRemark`）也照抄，别自己翻译回原路径。

三条推断，实测时留意但不许为了对上它们而改记录：

1. **D1/D2 改前很可能已经是 `b.sh`，不是整段。** 因为 `normalizePath`（`internal/cli/args.go:441`）先把 `\` 换成 `/`，之后 `path.Base` 才切得对。若果然如此，说明工单 02 的 `Blocked by: 01` 理由写反了——真正会退化成整段的时点是「01 删掉 normalizePath 之后」，02 是来堵这个洞的。
2. **D3 改前的后缀大概率是 `.v2/deploy`（正斜杠）**，工单写的是「不是 `.v2\deploy`」。同一个原因。
3. **D5 改前可能不是「悄悄落成 b.log」，而是「远端文件不存在」**：`-d` 的值同样过 normalizePath，`/tmp/a\b.log` 先变成 `/tmp/a/b.log`。到底哪种，只有实测能定。

---

## 五、结果：改前基线

### 5.1 环境与状态

| 项 | 值 |
| --- | --- |
| 执行日期 | 2026-10-26 |
| 仓库 HEAD | `0da83af`（master）：`docs(命令行路径): 交接文档修四处坑，补一条给 Windows 侧的指令` |
| go version | 仓库根目录：`go1.26.4 windows/amd64`；**模块目录内：`go1.27.0`**（`GOTOOLCHAIN=auto` 自动切到 1.27） |
| go env GOOS | `windows` |
| `func normalizePath` 是否命中 | 命中：`modules\SSHFleet_Go\internal\cli\args.go:507`（本文写的 `:441` 是行号漂移，同一个函数） |
| `filepath.Base(remotePath)` 是否命中 | 命中 3 处：`internal\ssh\sftp.go:258`、`:299`、`:356` |
| 目标机 | 有：WSL2 `Ubuntu 26.04`，经 localhostForwarding 以 `127.0.0.1:22` 访问；登录账号为**自建非 root 账号 `wbverify`**（NOPASSWD sudo）。CentOS-7 未使用 |

### 5.2 测试基线（`go test ./...` 全文）

```text
?   	sshfleet	[no test files]
?   	sshfleet/internal/batch	[no test files]
ok  	sshfleet/internal/cli	8.471s
ok  	sshfleet/internal/common	0.504s
ok  	sshfleet/internal/config	8.504s
ok  	sshfleet/internal/confirm	1.610s
ok  	sshfleet/internal/credential	9.245s
ok  	sshfleet/internal/dangercheck	3.799s
?   	sshfleet/internal/log	[no test files]
ok  	sshfleet/internal/nodelist	12.943s
ok  	sshfleet/internal/output	12.576s
ok  	sshfleet/internal/result	0.343s
ok  	sshfleet/internal/ssh	0.502s
ok  	sshfleet/internal/verdict	4.519s
```

无 `FAIL`、无 `build failed`、无 `no such tool`，也没有 `[警告]` 类非测试行。全量退出码 0。

SKIP 用例（上面这条命令不带 `-v`，看不出 SKIP；另跑一次 `go test ./... -v` 抓的）：

```text
--- SKIP: TestParseDoesNotRewriteWindowsStylePath (0.00s)
--- SKIP: TestConvertSecretEncryptsPlainFile (0.00s)
```

`TestParseDoesNotRewriteWindowsStylePath` 是 SKIP，符合 3.3 的预期。第二条 `TestConvertSecretEncryptsPlainFile` 本文没提，同样算基线。

### 5.3 五条验收

| # | 命令 | 实测（日志原文 / 文件名 / 落盘结果） |
| --- | --- | --- |
| D1 | `-s ...\scripts\a\b.sh` | `r='b.sh'`；`s='D:/Desktop/Code/SSHFleet/build/win-verify/scripts/a/b.sh'`；归档目录 `history\2026-10-26_19-36-06_script_b.sh`；连接 127.0.0.1:22 被拒（预期内，日志行齐备） |
| D2 | `-u ...\scripts\a\b.txt -p /opt/` | `r='b.txt'`；`u='D:/Desktop/Code/SSHFleet/build/win-verify/scripts/a/b.txt'`；`p='/opt/'`；归档目录 `history\2026-10-26_19-36-14_upload_b.txt` |
| D3 | `-s ...\scripts\dir.v2\deploy` | `r='deploy'`；`s='D:/Desktop/Code/SSHFleet/build/win-verify/scripts/dir.v2/deploy'`；Trace：`非交互模式（--yes）跳过确认：脚本后缀 "" 未配解释器，直接回退到 bash`，**后缀已是空串** |
| D4 | `-d /tmp/app.log` | `r='app.log'`；`d='/tmp/app.log'`；终端「成功 1 / 失败 0」；落盘 `dl\127.0.0.1\app.log`（2 字节） |
| D5 | `-d "/tmp/a\b.log"` | `r='b.log'`；`d='/tmp/a/b.log'`（`\` 已被换成 `/`）；终端「失败 1」、`失败分类统计 >>> 远程路径不存在 ×1`；`dl\` 里**没有多出文件**（只有 D4 那个 `127.0.0.1\app.log`） |

> D4/D5 第一次（用 root 账号跑）报「远程路径不存在」是假象：进 `/tmp` 一看，19:40 造的两个文件已被 WSL 清空（发行版重启时 systemd 清 `/tmp`）。换成 `wbverify` 重建文件后重跑，上表记的是重跑结果。

### 5.4 顺手确认

| # | 看哪一格 | 实测 |
| --- | --- | --- |
| E1 | `p=` | `p='/opt/app/'`。改前确实把 `\opt\app\` 换成了 `/opt/app/`，于是凑巧通过「必须用 / 开头」的合规检查——日志里有 `p=`、有「合规性检查通过」 |
| E2 | `d=` | `d='/tmp/a/b.log'`。`\` 同样被换成了 `/` —— 4.6 第 3 点就此有答案，见 5.5 第 8 条 |

### 5.5 异常与意外

1. **工作区不干净（已按第二节上报，未清理）**：开工时 `git status --short` 有一条 `D docs/issues/2026-10-21-1413-命令行路径规范化/指令-Windows环境.txt`（未暂存）。因此构建标识带脏标记：`版本: v12.0.1+0da83af-dirty (2026-10-26 10:46:10)`。本文只提交自己，那条删除留在工作区没动。
2. **`go version` 达不达标取决于在哪个目录敲**：仓库根目录回 `go1.26.4`，`modules\SSHFleet_Go` 内回 `go1.27.0`。`go.mod` 写 `go 1.27`，`GOTOOLCHAIN=auto` 自动下载并切到 1.27，构建与测试都正常。3.1 若在仓库根判「≥ 1.27」会误判为不达标。
3. **B 的判据要排在 4.1 之后才成立**：裸 `.\SSHFleet.exe --help` 直接退出码 1，报 `[ERROR] [function:config] 加载配置文件失败：config/SSHFleet.conf`。程序启动第一步就读配置，`--help` 也不例外。拷完配置后才打出帮助，首行下方确实是版本行。
4. **4.1 漏了三份规则文件**：只拷 `SSHFleet-for-windows.conf` 时，任何实跑都先死在 `[ERROR] [function:cli] 规则文件缺失：./config/keywords_dangerous.conf、./config/keywords_error.conf、./config/keywords_passwd.conf`。从 `modules\SSHFleet_Go\config\` 补拷这三份才能跑。建议把这一句补进 4.1。
5. **归档目录名不是本文猜的「脚本」**：实际是 `2026-10-26_19-36-06_script_b.sh` / `..._upload_b.txt` / `..._script_deploy`，前缀是 `script` / `upload` 这类 ASCII 词，备注 `b.sh` 原样接在后面。路径本身没进目录名，也没看到 `_` 替换 `\` 或 `:` 的形态。
6. **4.6 第 1 点成立**：D1/D2 改前 `r=` 已经是 `b.sh` / `b.txt`，不是整段。原因是 `normalizePath` 先把 `\` 换成 `/`，之后 `path.Base` 才切得对。工单 02 的 `Blocked by: 01` 说法确实反了：整段退化发生在「01 删掉 `normalizePath` 之后」，02 是来堵这个洞的。
7. **4.6 第 2 点不成立，比预判更彻底**：D3 改前的后缀**已经是空串 `""`**，不是 `.v2/deploy`。取后缀用的是 `path.Ext`（`internal\cli\interpreter.go:64`），它只看最后一个 `/` 之后那一段——`deploy` 没有点，于是 `""`。这意味着 D3 在「改前」与「02 落地后」都是空串，判据两栏都满足；真正会冒出 `.v2\deploy` 的窗口是「01 已删 `normalizePath`、02 还没补」。
8. **4.6 第 3 点有答案**：D5 改前不是「悄悄落成 `b.log`」，而是**远端文件不存在**。`-d` 的值先过 `normalizePath` 变成 `/tmp/a/b.log`（E2 的 `d=` 直接印证），目标机上没有这个路径，于是按「远程路径不存在」失败，`dl\` 不多出文件。也就是说改前 D5 与改后 D5 **表现为同一种失败**，差别只在报错与回显里的路径（改前 `/tmp/a/b.log`，改后应回显原值 `/tmp/a\b.log`）——第六节验的时候只看「失败」不够，要连报错原文一起对照。
9. **`--help` 输出经管道会黏行**：`... (2026-10-26 10:46:10)用法:` 挤在同一行，`密钥管理选项:  模式（五选一）` 也黏着。像是用 `\r` 做整行刷新被管道吞了换行。只影响观测，不影响判据，没深查。
10. **测试账号**：D4/D5 一开始用了 root，被作者当场纠正。root 登录会被 `effectiveSudo := useSudo && cfg.User != "root"` 这道门禁挡掉，`sudo` 路径根本没跑。后来在 Ubuntu 里自建 `wbverify`（`sudo` 组 + `/etc/sudoers.d/wbverify` NOPASSWD），配置 `sudo = true` 因此真正走了提权路径。root 密码已锁回（`passwd -l root`）。
11. **节点用 `127.0.0.1:22`**：WSL2 的 localhostForwarding 把 Windows 侧 22 转到发行版内 22，`Test-NetConnection 127.0.0.1:22` 回 True。4.4 写「换成真值」，这里的真值就是 `127.0.0.1`。
12. **沙盒保留了**：`build\win-verify\` 下 exe、config、scripts、dl、history、`test-baseline.txt` 全在，第六节可以直接复用（`/tmp` 的远端文件会被清，重跑前要重建）。

---

## 六、结果：改后验证（判据见 4.3 / 4.4）

> 2026-10-26 落地后重跑。二进制 `build\win-verify\SSHFleet-new.exe`（同日同沙盒，目标机仍是 WSL2 Ubuntu 26.04 + `wbverify`）。

| # | 改后实测 | 与 5.3 有无差别 | 是否满足判据 |
| --- | --- | --- | --- |
| D1 | `r='b.sh'`；`s='D:\Desktop\Code\SSHFleet\build\win-verify\scripts\a\b.sh'`（原样） | 有：`s=` 由 `D:/…/b.sh` 变回 `D:\…\b.sh` | 满足 |
| D2 | `r='b.txt'`；`u='D:\Desktop\Code\SSHFleet\build\win-verify\scripts\a\b.txt'`；`p='/opt/'` | 有：`u=` 同上，分隔符不再被换 | 满足 |
| D3 | `r='deploy'`；Trace `脚本后缀 "" 未配解释器`；`s=` 原样 | 无（改前也是空串，见 5.5 第 7 条） | 满足 |
| D4 | `r='app.log'`；`d='/tmp/app.log'`；成功 1；落盘 `dl\127.0.0.1\app.log` | 无 | 满足 |
| D5 | `r='a_b.log'`；`d='/tmp/a\b.log'`（原样）；失败 1，分类「本机写入失败」，明细原文 `a\b.log: 下载失败 - sshfleet.transfer.local_file_failed: name not usable as a local file name: a\b.log`；`dl\` 没多出文件 | 有：改前是「远程路径不存在」（`d=` 被换成 `/tmp/a/b.log` 后远端没有），改后是「本机写入失败」且文件名是整段 `a\b.log` | 满足 |

> 顺手确认两条也重跑了：E1 的 `-p '\opt\app\'` 现在**被合规检查拦下**，报错回显原值 `\opt\app\`，日志里没有 `p=`（与 4.5 的判据一致）；E2 的 `d=` 是原值 `/tmp/a\b.log`。

---

## 七、交付方式（靠 git 回来）

1. 填完第五节（改后那一轮填第六节），把顶部的 `Status:` 改成 `ready-for-human`——**只写这一个标签字符串**，「改前基线已填、待作者复核」这类话记到 `## Comments` 里另起一行，别塞进 `Status:` 行（状态行只放标签，`docs/agents/triage-labels.md`）。
2. **只 commit 本文件**。路径里有中文，PowerShell 5.1 下手打容易乱码，用通配更稳：
   ```powershell
   git add "docs/issues/2026-10-21-1413-*/audit.md"
   git commit -m "docs(命令行路径): Windows 侧改前基线实测" -m "工单 02 的验收在 Linux 上验不出来（path/filepath 按 GOOS 在编译期定），这部分交给 Windows 环境跑，结果贴在本文件第五节。"
   ```
3. `git push`。沙盒里的 txt、exe、history 都不提交（`build/` 已在 `.gitignore` 里）。提交前 `git status --short` 确认**只有 audit.md 一个文件**。

---

## Comments

- 2026-10-26：建立。工单 02 的五条验收依赖 Windows，Ubuntu 侧只能验到「Linux 上两种切法结果一样」，故把验证环节外派到 Windows 环境，结果落本文件。同日实跑过一次 Linux 侧全量测试：11 包 ok，唯一红是 `TestParseDoesNotRewriteWindowsStylePath`（`source_test.go:217`），该条归工单 06 清理，01–04 只承诺「不新增红」。
- 2026-10-26（Windows 侧）：改前基线已填，见第五节。全量测试 12 包 ok、2 条 SKIP、无红；D1–D5 与 E1–E2 全部实跑，目标机为 WSL2 Ubuntu 26.04（`127.0.0.1:22`），登录用自建非 root 账号 `wbverify`。三处与正文预判不符的地方记在 5.5 第 6–8 条，其中第 6 条（`Blocked by: 01` 的方向）建议在工单 02 里更正。第六节按要求留空，等代码改完再跑。
