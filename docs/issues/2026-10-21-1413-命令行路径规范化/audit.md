# Windows 侧基线：路径取名与后缀规则（2026-10-26）

Status: ready-for-agent

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

命中不了 = 代码已被改过（或你在别的分支上）。**立刻停止**，把 `git log -1 --format="%h %ad %s"` 与这两条的输出写进第七节，不要往下跑——在错误的代码状态上测出来的数没有用。

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
"echo hello" | Set-Content -Encoding utf8 scripts\a\b.sh
"hi"         | Set-Content -Encoding utf8 scripts\a\b.txt
"deploy"     | Set-Content -Encoding utf8 scripts\dir.v2\deploy
"127.0.0.1,22,root,x" | Set-Content -Encoding utf8 nodes-dummy.csv
```

再改 `config\SSHFleet.conf` **两行**：`[account]` 的 `password` 填明文密码；`[credential]` 的 `encrypt = true` 改 `encrypt = false`。

> 程序固定从 `./config/SSHFleet.conf` 读配置（相对当前目录），所以**所有命令都在 `build\win-verify` 这一层执行**。

### 4.2 为什么看日志而不是看屏

程序会把解析结果写进 `history\SSHFleetTools.log`，其中 `r='...'` 是自动备注、`s=` 脚本、`u=` 上传、`d=` 下载、`p=` 目录。`--yes` 会跳过确认屏直接去连机器——D1–D3 用连不上的 `127.0.0.1` 正好：连接失败不影响那几行日志，而那才是要看的。

### 4.3 D1–D3：不需要目标机

```powershell
$here = (Get-Location).Path

.\SSHFleet.exe -f nodes-dummy.csv -s "$here\scripts\a\b.sh" --yes
.\SSHFleet.exe -f nodes-dummy.csv -u "$here\scripts\a\b.txt" -p /opt/ --yes
.\SSHFleet.exe -f nodes-dummy.csv -s "$here\scripts\dir.v2\deploy" --yes
```

每条跑完，抄两样：

```powershell
Get-ChildItem history | Sort-Object LastWriteTime | Select-Object -Last 1 -ExpandProperty Name
Get-Content history\SSHFleetTools.log -Tail 40
```

| # | 看哪一格 | 改后应该是（判据，不是现在的预期） |
| --- | --- | --- |
| D1 | `r='...'` 或最新 `history\` 归档目录名 | `b.sh` |
| D2 | `r='...'` 或最新归档目录名 | `b.txt` |
| D3 | 含「脚本后缀」的那行 Trace：`非交互模式（--yes）跳过确认：脚本后缀 "…" 未配解释器…` | 后缀是空串，即 `""` |

> D3 若日志里没有那行 Trace，照实报告，不要自己下结论（可能是后缀被当成已配置，或这次没走回退分支）。

### 4.4 D4–D5：要目标机

```powershell
"IP,22,root,PASSWORD" | Set-Content -Encoding utf8 nodes.csv   # 换成真值

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
.\SSHFleet.exe -f nodes-dummy.csv -u "$here\scripts\a\b.txt" -p '\opt\app\' --yes
.\SSHFleet.exe -f nodes-dummy.csv -d '/tmp/a\b.log' -p .\dl\ --yes
```

- **E1**：看 `p='...'`。若 `p='/opt/app/'`，说明改前确实把 `\` 换成了 `/`（于是凑巧通过了「必须以 / 开头」）；改后同一条应当直接报错并回显 `\opt\app\` 原值。这是工单 01 的判据。
- **E2**：看 `d='...'`。若 `d='/tmp/a/b.log'`，就回答了 4.6 第 3 点，D5 的目标机可以省掉。

### 4.6 记录纪律与三条待证推断

**不要按任何「预期」去凑数。** 我这边没有 Windows 真机，对现状的推断已经错过一处，所以只记**你看到的原文**。日志里出现 `_` 替换掉 `\` 或 `:` 的形态（备注过了一遍 `SanitizeRemark`）也照抄，别自己翻译回原路径。

三条推断，实测时留意但不许为了对上它们而改记录：

1. **D1/D2 改前很可能已经是 `b.sh`，不是整段。** 因为 `normalizePath`（`internal/cli/args.go:441`）先把 `\` 换成 `/`，之后 `path.Base` 才切得对。若果然如此，说明工单 02 的 `Blocked by: 01` 理由写反了——真正会退化成整段的时点是「01 删掉 normalizePath 之后」，02 是来堵这个洞的。
2. **D3 改前的后缀大概率是 `.v2/deploy`（正斜杠）**，工单写的是「不是 `.v2\deploy`」。同一个原因。
3. **D5 改前可能不是「悄悄落成 b.log」，而是「远端文件不存在」**：`-d` 的值同样过 normalizePath，`/tmp/a\b.log` 先变成 `/tmp/a/b.log`。到底哪种，只有实测能定。

---

## 五、结果：改前基线（待填）

### 5.1 环境与状态

| 项 | 值 |
| --- | --- |
| 执行日期 | |
| 仓库 HEAD | |
| go version | |
| go env GOOS | |
| `func normalizePath` 是否命中 | |
| `filepath.Base(remotePath)` 是否命中 | |
| 目标机 | （有无、IP） |

### 5.2 测试基线（`go test ./...` 全文）

```text
（待填）
```

### 5.3 五条验收

| # | 命令 | 实测（日志原文 / 文件名 / 落盘结果） |
| --- | --- | --- |
| D1 | `-s ...\scripts\a\b.sh` | |
| D2 | `-u ...\scripts\a\b.txt -p /opt/` | |
| D3 | `-s ...\scripts\dir.v2\deploy` | |
| D4 | `-d /tmp/app.log` | |
| D5 | `-d "/tmp/a\b.log"` | |

### 5.4 顺手确认

| # | 看哪一格 | 实测 |
| --- | --- | --- |
| E1 | `p=` | |
| E2 | `d=` | |

### 5.5 异常与意外

（任何不对劲的地方：输出与描述对不上、报了你没预期的错、日志里出现本文没提过的行。**你觉得奇怪的地方就是我要的信息**，不要因为「不影响任务」就不写。）

---

## 六、结果：改后验证（待填，判据已给）

> 这一节等代码改完、作者重新提交后再跑。判据写在 4.3 / 4.4 那两张表的右列，跑法完全一样。**此刻不要填。**

| # | 改后实测 | 与 5.3 有无差别 | 是否满足判据 |
| --- | --- | --- | --- |
| D1 | | | |
| D2 | | | |
| D3 | | | |
| D4 | | | |
| D5 | | | |

---

## 七、交付方式（靠 git 回来）

1. 填完第五节（改后那一轮填第六节），把 `Status:` 改成 `ready-for-human` 并注明「改前基线已填，待作者复核」。
2. **只 commit 本文件**：`git add docs/issues/2026-10-21-1413-命令行路径规范化/audit.md`，提交信息：
   ```text
   docs(命令行路径): Windows 侧改前基线实测

   工单 02 的验收在 Linux 上验不出来（path/filepath 按 GOOS 在编译期定），
   这部分交给 Windows 环境跑，结果贴在本文件第五节。
   ```
3. `git push`。沙盒里的 txt 与 exe 都不提交（`build/` 已在 `.gitignore` 里）。

---

## Comments

- 2026-10-26：建立。工单 02 的五条验收依赖 Windows，Ubuntu 侧只能验到「Linux 上两种切法结果一样」，故把验证环节外派到 Windows 环境，结果落本文件。同日实跑过一次 Linux 侧全量测试：11 包 ok，唯一红是 `TestParseDoesNotRewriteWindowsStylePath`（`source_test.go:217`），该条归工单 06 清理，01–04 只承诺「不新增红」。
