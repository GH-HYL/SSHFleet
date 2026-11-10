# 06 包内 `parse`（回执）

**判据**：`../spec.md` 的 A1–A3（层 1）、`ADR-0032` 的 A4–A9（层 2，按 13 条更新记录读）、A10–A20（层 3，自审）。本轴只认分给它的那几条：A1（屏幕／退出码）、A7（拼句只出 `message`，只作事实）、A13（站内按模式分文件，不判重复）、A14（出口白名单，只作事实）。`parse.Run` 四出口的**交接件形状**归 01（已收账），本轴只判「四个出口是不是各写了一遍」。

**只读**：未改 `modules/` 下任何文件。临时产物全在 `build/审计06_parse_2026-11-09/`（两版二进制、配置副本、`GOCACHE`／`GOTMPDIR`）。上机面：**没有连任何主机**——所有运行都在「加载配置／解析参数／读材料」这一段停住，未建立 SSH 会话。

---

## 〇 这一轴怎么审的

1. 读全 `internal/parse/` 的 17 个生产文件（3,594 行），`testdata/` 只作旁证。
2. 老版只读 `internal/cli/args.go`、`internal/cli/source.go`、`internal/cli/check.go`（150–240）、`internal/config/config.go`（1–120）、`internal/common/ip.go`、`internal/common/csvtext.go`（grep），`internal/nodelist/`（grep）。
3. 计数用 grep 复算（命令写在 §五），**没有为计数去构建**。
4. 三条行为差异是**实跑**的：用同一份模块缓存把两版都编到 `build/…/bin/{new,old}`，各带一份 `config/` 副本跑 `-h`／`-f`，取屏幕与退出码。两版都 `go build` 通过（新工程 `-mod=mod GOPROXY=off`，模块缓存取 `/home/ubuntu/go/pkg/mod`）。

---

## 一 三条解析站独有的口径（先答）

| 口径 | 判在哪 | 结论 |
| --- | --- | --- |
| **版本拼装归解析站**（记录 3） | `parse/version.go:40` `versionString` 是唯一的拼装点，全工程被调 1 次（`parse/run.go:41`）；`message/help.go:80` 只加「版本: 」这个标签，不拼值 | **单点，成立**。塔尖只给 `appVersion`（`main.go:29`），`debug.ReadBuildInfo` 取构建标识（`version.go:14-30`）。第二处拼装：0 处 |
| **判二进制归解析站**（记录 4） | 判断确实住在 `parse`：`parse/answer.go:152`（helper）、`parse/material.go:45`、`parse/material.go:94`（内联） | **住对了，但站内没做成一处**：3 处 2 种写法；包外还有第 4 处（`credential/convert.go:34`）。见 J2 |
| **`argv` 递交**（记录 5） | 塔尖取一次（`main.go:75` `parse.Run(os.Args, appVersion)`）；`parse` 内 `os.Args` **0 处**（`argv.go:91` 只是注释）、`Getenv`／`LookupEnv`／`Environ` **0 处** | **argv 面守住**。另读到进程面 3 处（`os.Getwd` `config.go:190`、`runtime.GOOS` `config.go:568`、`debug.ReadBuildInfo` `version.go:15`）与终端宽度 1 处（`console.Width()` `run.go:41`）——都不是 argv，也不是环境变量 |

---

## 二 四形态的分布与分母

| 形态 | 分母（查了什么） | 处数 | 报 |
| --- | --- | --- | --- |
| 另造一套 | 6 类可复用能力逐类查（宽度、遮蔽、系统原因、备注清洗、屏幕、帮助排版）＋4 项站内自造基础设施（TOML 词法器、argv 扫描、IPv4 判据、排序／纯数字） | 正面 6/6：`message.DisplayWidth`（`tables.go:158,161`）、`mask.Secret`（`nodes.go:210`）、`message.SystemReason`（10 处）、`archive.SanitizeRemark`（`argv.go:418-425`）、屏幕只经 `message.Line`（`parse` 内 `console.Print`／`fmt.Print`／`os.Stdout` **0 处**）、帮助排版在 `message.HelpScreen` | 1 硬伤（H1）＋1 判断（J4） |
| 回读源头 | 8 类外部读取，30 个 `os.*`／`runtime.*`／`debug.*` 点：`ReadFile` 13、`Stat` 6、`Lstat` 1、`Open` 1、`ReadDir` 2、`Readlink` 1、`Getwd` 1、`GOOS` 1、`ReadBuildInfo` 1、`os.ErrNotExist` 1 | 文件系统 24 点全是解析站本职（配置文件、三份判据表、脚本、清单、代填、上传源、下载目录）；**绕过上游纸的 0 处**（解析站是第一站，无上游纸） | 0 |
| 站里拼话 | 中文字面量 **206 条**（17 个文件）；自己拼成整句的 **93 行**（格式化 72 行 ＋ `+` 拼接 21 行），分布在 10 个文件 | `tables.go` 27／`config.go` 24／`material.go` 10／`nodes.go` 9／`collect.go` 7／`nodelist.go` 6／`toml.go` 4／`runmeta.go` 3／`options.go` 2／`run.go` 1 | 1 判断（J1） |
| 按老版特判 | 15 个候选分支（§四逐条） | 判为「绕路」0 个；1 个是明示的行为变化（有据） | 0 |

**四出口**（`parse.Run`）：4 条 `return`（`run.go:26/42/47/51/57/60`，实际 6 条出口语句、4 种结局），拼「起不来」屏的入口 3 个（`cannotStart`／`cannotStartBeforeLog`／`cannotStartScreen`），**但只有 1 份实现**（`run.go:78`），另两个是带名字的壳。不是四个出口各写一遍。屏幕那一句统一由 `message` 出（`message.HelpScreen`／`message.Shell`），`parse` 不给 `Problem` 写拒绝句——7 类 `Problem.Kind` 的句子都在 `gate`→`message` 那一侧成形（`gate/run.go:116-130,227,261,287`），`parse` 只记 `Kind`＋`Option`＋`Raw`＋`Detail`。四出口：**没发现各写一遍**。

---

## 三 一条一条

### H1【硬伤】IPv4 判据被放宽：前导零八位组从「拒」变成「收」

- **位置与证据**（行为类，命令＋输出）：
  - 新版自己手写一份：`parse/nodelist.go:180-204`，判据是「四段、每段 ≤3 位纯数字、≤255」。**不拒前导零**。
  - 老版用标准库：`modules/SSHFleet_Go/internal/common/ip.go:13-16` `netip.ParseAddr` ＋ `Is4()`。
  - 两版二进制实跑同一句（`-f` 只给一个值，它同时测「像不像一条内联节点记录」，两个调用点用的是同一把尺子）：
    ```text
    $ bin/old -f "010.0.0.1" -c ls        # 老版
    [ERROR] [function:cli] 选项/参数合规性检查未通过：-f 参数指定的文件读不到：010.0.0.1
    原因：no such file or directory
    ...
    $ bin/new -f "010.0.0.1" -c ls        # 新版
    [ERROR] [function:nodelist] CSV凭据预检查失败，共 1 处错误：
      配置 account.password：读不到凭据文件 /home/hyl/.MyPW/SSHFleet_pw: ...
    ```
    老版把这个值判成「不是一条内联记录」；新版判成「是一条内联记录」，直接进了凭据预检。两版退出码都是 1，但停在不同的门。
  - 判据本身的差异（同一台机器、同一段逐字抄来的函数）：
    ```text
    输入           netip.ParseAddr(err)                 老版   新版
    "10.0.0.1"   <nil>                                  true   true
    "010.0.0.1"  IPv4 field has octet with leading zero  false  true
    "1.2.3.04"   IPv4 field has octet with leading zero  false  true
    "999.1.1.1"  IPv4 field has value >255               false  false
    ```
    （探针源码 `build/审计06_parse_2026-11-09/vprobe/main.go`，逐字抄 `parse/nodelist.go:180` 与 `common/ip.go:13`。）
- **附带的注释与事实不符**：`nodelist.go:177-179` 写「严格认 IPv4 字面量……**不认前导零以外的花样**」并且「与老版同一条判据」——两句都不成立：它恰恰认了前导零，而老版不认。
- **影响面**：`010.0.0.1`（Excel 导出的零填充 IP 是常见来源）会作为合法节点往下走，到执行站才以「连不上」收场；老版在解析阶段就报「IP格式不正确」。`mask` 用的是另一份同形实现（`mask/inline.go:70-90`，未受本行影响），两份在这一点上一致，所以脱敏侧不跟着变。
- **伤**：行为变了。
- **档**：硬伤（判据被放宽，且注释把放宽说成「与老版同一条」）。

### H2【硬伤】`-f` 指一个目录：报错换了一句，原因句与事实相反

- **位置与证据**（行为类，命令＋输出）：
  ```text
  $ bin/new -f ../dirprobe -c "ls"     # 新版，dirprobe 是一个存在的空目录
  [ERROR] [function:cli] 选项/参数合规性检查未通过：-f 参数指定的文件读不到：../dirprobe
  原因：找不到这个文件
  提示：要临时传一台机器，可以直接写一行节点信息，第一个字段是 IP，例如 192.168.1.1,22,root,密码
  日志：…/run_new/history/SSHFleetTools.log
  退出码=1

  $ bin/old -f ../dirprobe -c "ls"     # 老版，同一目录
  [ERROR] [function:cli] 选项/参数合规性检查未通过：-f 参数指定的 CSV 文件读不到：../dirprobe
  原因：is a directory
  提示：请检查文件是否存在、当前用户有没有读权限
  日志：…/run_old/history/SSHFleetTools.log
  退出码=1
  ```
  三处不同：抬头少「CSV」两字；原因从系统的 `is a directory` 变成自造的「找不到这个文件」（目录**是存在的**，这句是假话）；提示从「检查文件是否存在、有没有读权限」变成内联记录的去路。
- **代码路径**：老版一处判来源（`cli/source.go:32-38` `resolveSource`：`os.Stat` 成功即 `IsFile=true`，目录也是），随后 `check.go:185-190` 去读它，目录在**读**这一步报 `EISDIR`。新版把这处判来源拆成了两份：`nodelist.go:65-107` `manifestText` 与 `answer.go:91-105` `answerSource`。
  - `answerSource` 把「目录也算命中路径」抄下来了（`answer.go:94-96` 注释自陈「后面 `ReadFile` 会报『是个目录』，那正是老版的口径」）；
  - `manifestText` 没抄：它的文件分支条件是 `statErr == nil && !info.IsDir()`（`nodelist.go:74`），目录从这个条件里漏下去，落到内联分支；内联分支用 `looksLikeInlineNode(目录名)` 判否，再在 `nodelist.go:97-100` 把 `statErr == nil` 当成「没 err」，于是原因停在默认的「找不到这个文件」。
  - 新版前面还多一道：`material.go:81` 的 `err == nil && !info.IsDir()` 同样跳过目录，所以这道检查对这个值什么都没说。
- **旁证**：`answer.go:5-8` 的包注释写「`-f` 那一路在 `nodelist.go` 的 `manifestText`，`-a` 这一路在这里，**判定口径相同**」——口径相同的两份，在「目录」这一格已经分叉。老版这一处是**一份**（`cli/source.go:44-105` 的 `valueForm` 表 ＋ `checkInlineValue`，`-f` 与 `-a` 只各给一条 `isRecord` 判据）。
- **影响面**：`-f <目录>` 是「路径打错／把目录当文件」的常见手误。使用者现在拿到的是一句自相矛盾的话（「不存在」指着一个存在的目录），并且被引向内联写法那条错路。
- **伤**：行为变了。
- **档**：硬伤。

### H3【硬伤】配置里的重复键：老版报错，新版静默取后值

- **位置与证据**（行为类，命令＋输出）——在 `[account]` 里给 `port` 写两遍（第二行 `port = 2200`）：
  ```text
  $ bin/old -h        # 老版
  [ERROR] [function:config] 加载配置文件失败（config/SSHFleet.conf）：解析 TOML 失败：
  toml: line 10 (last key "account.port"): Key 'account.port' has already been defined.
  提示：多数是引号或方括号没配对，照随包的配置文件核一遍
  退出码=1

  $ bin/new -h        # 新版
  SSHFleet - 批量 SSH 运维工具（命令/脚本执行、文件上传下载）
  版本: v1.0.0+620b884-dirty (2026-11-09 09:34:17)
  …
  退出码=0
  ```
- **代码**：`toml.go:179-195` `rawView` 对同一节的同一个键做 `raw[section][key] = value`（后值盖前值），没有查重。而同一个文件第 15 行写「引号外转义、**重复键**这两样出厂配置里没有，**遇到就报错，不猜**」——代码没这么做。全工程的配置规则是「未知字段零容忍、缺字段零容忍」（`config.go:164-173`），重复键既不是未知也不是缺失，于是从这两道网里漏过去。
- **影响面**：一份被手工改出重复键的配置（复制粘贴一行忘了删）会静默按**后一个**值跑，屏幕上没有任何提示；老版停在配置这一步。值差得远时（端口 10022 → 2200）使用者只会看到「连不上」。
- **伤**：行为变了。
- **档**：硬伤（判据被放宽：同一类「配置不成器」从拒变成收）。

### J1【判断题】中文句子：93 行自拼 ＋ 29 条裸帮助文案；帮助那一块字在 `parse` 成形

- **位置与证据**（腐蚀类，给可复算计数）：
  - 全包中文字面量 **206 条**；自己拼成整句 **93 行**（`fmt.Sprintf`／`Errorf`／`Fprintf`／`WriteString` 72 行 ＋ `+` 拼接 21 行），按文件：`tables.go` 27、`config.go` 24、`material.go` 10、`nodes.go` 9、`collect.go` 7、`nodelist.go` 6、`toml.go` 4、`runmeta.go` 3、`options.go` 2、`run.go` 1。复算命令见 §五。
  - **其中帮助文案是一整块裸字面量**，不是拼装所以不在上面 93 里：`options.go:56-100` 的 `helpRowsOf` 有 20 条选项说明 ＋ 5 个组标题 ＋ 4 条「当前配置」标记（31 条，含 2 行 `fmt.Sprintf` 的默认值标记 `options.go:85-86`）。这 31 条经 `message.HelpEntry.Desc/Tag`（`options.go:117-119`）交给 `message`，`message.HelpScreen` 只做缩进、列对齐、折行（`message/help.go:59-116`）。
  - 两处注释对这件事的写法互斥：`parse/doc.go:4`「话由 message 拼」；`message/help.go:8`「字只在这一处成形（ADR-0032 第四条）」。实际是「字在 `parse` 成形、`message` 排版」。
- **代价**：改一个选项的措辞要去 `parse` 找，不在唯一改字处；新增一个选项要在 `options.go` 同时补「名字（选项表）」与「说明（helpRowsOf）」，两处都是人写的字。与 `ADR-0032` 记录 3「版本与帮助同源」不冲突——那条讲的是**事实**（有哪些选项、版本号）同源，没讲说明文字住哪。
- **口径**：按 §1.8，**契约格里的句子（`CannotStart.Body`／`Problem.Detail` 那一路）只登记事实与计数，落点写「待开发者裁」**，本条不替它下结论。帮助文案走的是 `message.HelpEntry`，不是契约格，可以判：落点＝腐蚀架构（字有第二个改字处），代价如上。
- **伤**：腐蚀架构。
- **档**：判断题（其中契约那半的落点：**待开发者裁**）。
- **附**：`parse` 对 `message` 的使用是密集的——`message.` 下 **46 个不同的名字、共 123 次调用**（`ProblemHint` 25、`SystemReason` 10、`Problem` 10、`ValueInvalid` 8、`CurrentValue` 8，含 `Line`／`HelpEntry` 两个类型名）。所以这不是「不用 message」，是「message 已给的那一层用满了，句子还是在自己手里拼」。

### J2【判断题】「判二进制」在包内 3 处、2 种写法，包外还有 1 处

- **位置与证据**：
  - `parse/answer.go:151-159` `isBinaryContent`（判 NUL），`answer.go:43` 调用；
  - `parse/material.go:45` `bytes.IndexByte(data, 0) >= 0`（`-s`）；
  - `parse/material.go:94` 同一行判据再抄一遍（`-f`）；
  - 包外：`credential/convert.go:34` `strings.ContainsRune(text, 0)`（`--convert-secret` 读的那份）。
- **代价**：记录 4 把这条判断判给解析站，但解析站内部没做成一处：同一个「含 NUL 即二进制」有 1 个 helper ＋ 2 处内联，包外还有第 4 处换了写法。改判据（例如以后要改成「非 UTF-8 也算二进制」）要改 4 处，而 `-s` 那条**已经在旁边多做了一道 UTF-8 判**（`material.go:50-54`），另外三处没有——三处的「二进制」含义现在已经不一样。
- **可权衡处**：`credential/convert.go` 那一处问的是「这份凭据能不能转」，与「这份料能不能当文本读」不是同一问，且凭据包不许 import `message`／`parse`（方向反了），分开有理由。所以本条只判**包内那 3 处**该不该合，包外那一处只登记。
- **伤**：腐蚀架构（同一判断多个出口，已开始分岔）。
- **档**：判断题。

### J3【判断题】下载模式的 `-p`（本机目录）被 `TrimSpace` 过——老版一个字节都不动

- **位置与证据**：
  - 新版 `argv.go:222-224`：`p.Task.Path = strings.TrimSpace(vals["--path"])`，**不分模式**。注释的理由是「`-p` 与 `-d` 是远端路径：两端空白在这里就去掉（老版也在 Parse 里先 filter 过一次）」。
  - 但 `-p` 在下载模式下是**本机目录**——同一份代码自己的帮助文案写着「目标路径：上传写服务器目录 / **下载写本机目录**」（`options.go:73`），`material.go:63-76` 也按本机目录 `os.Stat` 它。
  - 老版只在上传模式过滤：`modules/SSHFleet_Go/internal/cli/args.go:446-451` `if a.Upload != "" { a.Path = strings.TrimSpace(a.Path) }`，同处注释 `:445`「本地的 `-p`（下载模式）不在此列——本地路径一个字节都不动」。
- **同一个值的两个形态**：`material.go:64` 检查的是 `vals["--path"]`（原值），纸上装的是 `p.Task.Path`（去过空白）——检查放行的那条串与执行站真正用的那条串，不是同一个。
- **代价**：`-d /remote -p "/tmp/my dir "` 这种写法，老版按原样用（带尾空格 → 目录不存在 → 报错），新版去掉空格后可能命中一个**不同的**目录。这一格是使用者给的本地路径，去掉空白等于替他改了一个字节。
- **证据缺口**：这是静态对照到的；没跑到确认屏（要跑到那一屏得有一份能解开的凭据，配置里 `encrypt = true` 且 `account.password` 指着一个不存在文件，两版都在凭据预检停下）。够条件时对一下 `-d /x -p " dir "` 的确认屏即可。
- **伤**：行为变了（潜在）。
- **档**：判断题。

### J4【判断题】站内自造的基础设施：TOML 词法器 226 行 ＋ argv 扫描 547 行，替代了两个库；无施工记录

- **位置与证据**：
  - `toml.go:27-68` `tokenize`（自家 TOML 子集：只认 `[节]`／`[[数组表]]`／`键 = 值`／跨行数组／`#` 注释；不支持行内表、多行字符串、引号外转义）。老版用 `github.com/BurntSushi/toml`（`modules/SSHFleet_Go/go.mod`）。
  - `argv.go:42-113` 自家命令行扫描（20 条选项、`--x=y` 内联、`--` 终止、位置参数、`-h` 早停）。老版用 `github.com/spf13/pflag`。
  - 连带自造的四件小事：`config.go:573-579` 手写插入排序（同一包的 `downlink.go:150` 已经在用 `sort.Strings`）；`tables.go:368-380` 手写 `atoi`、`nodes.go:282-292` 手写 `allDigits`（`config.go:337` 用的是 `strconv.Atoi`）；`nodelist.go:180` 手写 IPv4（见 H1）。
  - `docs/adr/` 里查不到这项决定（`grep -rn 'pflag\|BurntSushi\|TOML' docs/adr/0032*.md docs/adr/003[3-9]*.md docs/adr/004*.md` 无命中）；两处包注释只写了「为什么口径必须这样」，没写「为什么不用库」。
- **代价**：`go.mod` 少两个直接依赖（这是收益）；代价是**解析器的正确性面要自己守**：TOML 的错误面已经不完整（H3 的重复键），argv 的错误面与 pflag 的方言有出入（`-abc` 合并短选项、`--flag` 的缩写匹配都不认）。这两面没有 ADR 兜底，也没有「只认子集」的边界写在文档里（只有 `toml.go:8-15` 的注释）。
- **旁证**：`toml.go:15` 那句「重复键…遇到就报错，不猜」说明作者自己也把「重复键要报」当成这个子集的一部分——是**漏实现**，不是有意取舍。这条漏实现已经单独记成 H3。
- **伤**：只是架构取舍（库换自写）；漏实现那一半记 H3。
- **档**：判断题。

---

## 四 按老版特判：15 个候选，0 个绕路

逐个读代码分支，问「这个分支是不是只有老版才需要」：

| # | 位置 | 分支 | 判 |
| --- | --- | --- | --- |
| 1 | `config.go:153-162` | 见到 `[paths]`／`[execution.env]` 给专用提示 | 老版同处同字（`modules/SSHFleet_Go/internal/config/config.go:161-171`）；随包配置永不含这两段，是新旧都留的历史分支，属 A1 屏幕等价 |
| 2 | `nodelist.go:66-71` | `-f` 空串当路径交给系统读（借系统那句报错） | 老版同口径（`cli/source.go:33`），照办不另写 |
| 3 | `argv.go:462-466` | 密钥命令上「多余参数」先于「两条密钥命令互斥」 | 老版次序（`cli/args.go:369-375` 在读命令之前报多余段） |
| 4 | `argv.go:524-547` | 没给 `-r` 时把自动备注算成「给了 `-r`」，用于「密钥命令不能与批量参数一起给」 | 老版把默认备注填在解析期（`cli/args.go:453`），同一笔账 |
| 5 | `argv.go:105` | `splitOwner` 的回溯门槛 `pos < 3`（老版 `pos < 2`） | 只是索引适配：老版收到的是 `os.Args[1:]`，新版收到整条 argv。口径未变 |
| 6 | `tables.go:57-65` | 改密模式下「密码过期」分类无关键词即起不来 | **明示的行为变化**（`ADR-0034` 决定七），有据 |
| 7 | `script.go:19-34` | 后缀用 `filepath.Ext`（本机规则），比较前转小写 | 有据：`ADR-0025` 决定 2（本地路径按本机规则切）；配置加载时键已归一成小写 |
| 8 | `nodelist.go:45-57` | 首行首列不是 IPv4 就当表头移除并报一句提示 | 老版 D13 的落法（`nodelist/csvread.go:39-46`），同一判据 |
| 9 | `tables.go:182-190` | 危险规则正则统一加 `(?i)` | 对位老版 `re.IGNORECASE`，有据 |
| 10 | `nodes.go:203-218` | 凭据失败回显：值里含 `/`／`\` 就原样给，否则遮 | 老版口径（`common/mask.go` 同一判），且遮走 `mask.Secret` 不自己拼星号 |
| 11 | `passwd.go:33-38` | 密文串被贴进 `--change-password` 时，按加密开关给两句不同的下一步 | 老版 `ADR-0027` 口径 |
| 12 | `run.go:23-28` | 配置这一步的「起不来」不带「日志：」行 | 老版步骤 1 在步骤 2 之前（日志还没建），两个具名入口各表一件事 |
| 13 | `argv.go:44-47` | 一个参数都不给 → 帮助 | 老版 argparse（`cli/args.go:356-359`） |
| 14 | `config.go:531-533,568-570` | 路径措辞与合法形态按 `runtime.GOOS` 分两套 | 老版同（`config.go` 的 `runtime.GOOS`） |
| 15 | `tables.go:203-208` | 三份判据表的数组标题用单数 `[[category]]` | `A18` 有意写的，提示词点名别当特判报 |

**结论**：15 个里 0 个是「新版多余、只有老版需要」的绕路。#1 是离「可有可无」最近的一个（随包配置永远不触发），但它保的是**拿老配置升级的那一步**，且老版同字——判有据。真正需要盯的是 H1／H2／H3：那三处不是特判，是**判据本身变了**。

---

## 五 分母

| 项 | 分母 | 报了几条 |
| --- | --- | --- |
| 生产文件 | **17／17** 读全，3,594 行（`find internal/parse -name '*.go' -not -name '*_test.go' \| xargs wc -l` 合计 3594） | — |
| 另造一套 | 6 类横切能力 ＋ 4 项自造基础设施 ＝ **10 项** | 1 硬伤（H1）＋1 判断（J4） |
| 回读源头 | **8 类**读取／**30** 个 `os.`／`runtime.`／`debug.` 点 | 0 处越界；4 处进程面读取只记事实 |
| 站里拼话 | 中文字面量 **206** 条；自拼整句 **93** 行（10 个文件）；裸帮助文案 **29** 条 | 1 判断（J1） |
| 按老版特判 | **15** 个候选分支 | 0 绕路；1 个明示变化（有据） |
| 版本拼装 | 全工程 **2** 个可能点（`parse/version.go:40` 拼值、`message/help.go:80` 加标签） | 0（单点） |
| 判二进制 | **4** 处（parse 3 ＋ credential 1），**2** 种写法 | 1 判断（J2） |
| `argv` 递交 | 塔尖取 1 处；parse 自读 `os.Args` **0**、环境变量 **0**、进程面 **4** | 0 |
| 配置认键 | **4 格实跑**（未知键／缺必填／重复键／裸值字符串）＋ 2 段历史分支 | 1 硬伤（H3）；1 处文案差异（见下） |
| `parse.Run` 四出口 | **4** 种结局、6 条 return、3 个拼屏入口（1 份实现） | 0 |
| 帮助面 | 3 个产出点（`helpRowsOf`／`helpEntries`／`HelpScreen`）＋ 31 条文案 | 计入 J1 |

**合计：报 7 条 —— 硬伤 3（H1、H2、H3）、判断题 4（J1–J4）。**

**可复算命令**（都在 `modules/SSHFleet2_Go/internal/parse` 下，`_test.go` 全部排除）：

```bash
# 中文字面量总数
cat $(ls *.go | grep -v _test) | grep -o '"[^"]*"' | grep -c '[一-龥]'          # → 206
# 自拼整句：格式化 72 行
grep -n 'fmt\.Sprintf(\|fmt\.Errorf(\|fmt\.Fprintf\|b\.WriteString' *.go | grep -v _test | grep '[一-龥]' | wc -l   # → 72
# 自拼整句：+ 拼接 21 行
grep -nE '"[^"]*[一-龥][^"]*"[[:space:]]*\+|\+[[:space:]]*"[^"]*[一-龥]' *.go | grep -v _test | wc -l            # → 21
# os 面
grep -ho 'os\.[A-Za-z]*' $(ls *.go | grep -v _test) | wc -l                     # → 30
```

两版二进制的复现路径（本轴临时产物，未动 `modules/`）：

```text
build/审计06_parse_2026-11-09/
├─ bin/{new,old}              两版二进制
├─ run_{new,old}/config/      两份出厂配置副本（含 history/）
├─ cfg_{new,old}/             重复键那格（[account] 里 port 写两遍）
├─ unk_{new,old}/             未知键那格（account.portt）
├─ mis_{new,old}/             缺必填那格（删掉 account.user）
├─ bare_{new,old}/            裸值那格（user = jx_zyc）
└─ {new,old}_{dir,ip}.out      H2／H1 的屏幕
```

---

## 六 未覆盖面

1. **老版只抽读**：`internal/cli/answer.go`、`cli/passwd.go`、`cli/interpreter.go`、`nodelist/fields.go`、`nodelist/precheck.go` 全文**没读**，只在 grep 结果里看到调用点；`config/passwd.go` 没读。凡是「老版那一句话在哪拼」的比对，除 `args.go`／`source.go`／`check.go`／`config.go`／`common/ip.go`／`common/csvtext.go` 外都是按 grep 判的。
2. **配置报错面只实跑 4 格**：正常、未知键、缺必填、重复键、裸值字符串各一格；`[paths]`／`[execution.env]` 两段历史提示、三份判据表（危险／错误分类／改密）的每一句报错**没有逐格跑**。那三张表的报错句共 17 行格式化拼装（`tables.go`），只做了静态核对。
3. **等价性不归本轴**：37 场覆盖到哪、四条已知差异授权没授权，归 03（已收账）。本轴只报「这 3 处是新出现的差异」，不断言「37 场之外还有多少」。
4. **屏幕／报告／退出码的整屏**：`-h` 之外的屏幕（确认屏、参数屏）**没跑到**——要过凭据预检得有一份能解开的凭据（`encrypt = true`），那是 `credential` 与 08 的地盘。J3 因此只有静态证据。
5. **`testdata/` 只当旁证**：`internal/parse/testdata/` 下的样本文件没有逐份读，只看了 `_test.go` 引用到的那几份。
6. **`reconcile_test.go` 的折名表**（A18）按提示词口径只作事实，未判；本轴没有把测试当分母。
7. **`mask/inline.go` 那份 `isIPv4`** 只读了函数体（判它与 parse 那份逐字同形），没审 `mask` 包本身（归 08）。
8. **构建环境**：两版二进制都编在 WSL2 Box 上（Go 1.26.10，`GOPROXY=off` + `/home/ubuntu/go/pkg/mod` 缓存）。若换台机器、模块缓存不同，`-h` 的版本行会变（`v1.0.0+620b884-dirty` 是当时工作区状态），H1–H3 的三句报错不受影响。
9. **`-a` 侧的目录口径**没实跑（只读代码，见 H2 的根因）；若要与 `-f` 那格配对验证，需另跑 `-a <目录>`。

---

## 顺带（文档／注释问题，不占发现位置）

- **D1**（与 H1 同源）：`parse/nodelist.go:177-179` 的注释「严格认 IPv4 字面量……不认前导零以外的花样」「与老版同一条判据」两句与实现、与老版都不符。
- **D2**：`parse/answer.go:5-8` 写「`-f` 与 `-a` 走同一处判定」，实际是两份实现（`nodelist.go:65` 与 `answer.go:91`），只是口径曾经相同；老版那一处是共享的（`cli/source.go:44-105`）。
- **D3**：`parse/toml.go:15` 写「重复键…遇到就报错」，与 `rawView`（`toml.go:179-195`）不符；同一文件两处注释互相矛盾。
- **D4**：`parse/argv.go:222` 写「`-p` 与 `-d` 是远端路径」，与同一份代码的 `options.go:73`（「下载写本机目录」）及 `material.go:63-76`（按本机目录查）矛盾。
- **D5**：`parse/doc.go:4`「话由 message 拼」与 `options.go:61-99` 里 31 条帮助文案的实际位置不一致（J1 的事实基础）。
- **D6**：`go.mod` 相对老版去掉了 `BurntSushi/toml` 与 `spf13/pflag` 两个直接依赖，`docs/adr/` 里没有对应决策记录；`parse/toml.go:8-15` 是唯一写「只认子集」的地方。
