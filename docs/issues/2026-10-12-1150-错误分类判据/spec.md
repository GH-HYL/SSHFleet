# 错误分类判据补全 · 规格

Status: done

**闭环说明（2026-10-08 复核）**：已解决。两块判据表已落地——`config/keywords_error.conf` 的 `[[categories]]`（第 46 行起）与 `[[exit_code_categories]]`（第 325 / 334 / 344 行）。**后续演进一步**：第二组的**选组依据**已由 `2026-10-12-1150-结果统一判定` 重构（2026-09-29，见 `docs/adr/0004` 的《2026-09-29 取代》节）从「结果退出码非 0」改为「目的命令是否给出退出码（`CommandExitCode != nil`）」；分类名与关键词表不变。真机实测（2026-10-08，非 root 账号）：`拒绝网络连接` 等分类按预期命中。

> 记录「连接与认证都成功、命令却被拒绝执行」这一族报文的分类决定。
> 全部结论来自 2026-09-16 的真机复现，原文与字段见第五节。
> **实施需用户明确授权**（个人开发规范 一、约束·1 红线 1）；本文档只落设计。
>
> **本轮取舍原则**（开发者 2026-09-16 明确）：分类的准确与稳定优先，**不拿"与旧版本兼容""改动面/工作量"当否决理由**——需要重构就重构。因此下面凡涉及取舍，一律选可靠的那一侧，并把代价写出来。

---

## 一、问题清单（全部纳入本轮）

| # | 现象 | 现状分类 | 性质 |
| --- | --- | --- | --- |
| P1 | 目标机密码过期 | 执行失败(退出码1) | **信号在手里被短路**（第二节） |
| P2 | 账号过期（`chage -E`） | **SSH协议错误** ❌ | 误标，且把人引向网络/协议排查 |
| P3 | `/etc/nologin` 存在（维护窗口） | **SSH协议错误** ❌ | 同上 |
| P4 | 登录 shell 为 nologin | 执行失败(退出码1) | 同族，未识别 |
| P5 | `ForceCommand internal-sftp` 下跑命令 | 执行失败(退出码1) | 同族，未识别 |
| P6 | 账号不在 sudoers | 执行失败(退出码1) | 同族，未识别 |
| P7 | 传输模式（上传/下载）遇上述任一 | SFTP失败 / 预处理失败 | 报错原文里没有任何原因线索 |
| P8 | `MaxSessions 0`（并发受限） | 会话创建失败 | 能分类，但文案看不出原因 |
| P9 | 词表自身：`has expired` 宽泛 | 误伤 `certificate has expired` 等 | 判据质量问题 |

**共同根因**：判据只认「退出码」与少数几条库/自撰文案，而**服务端其实一直在把原因告诉我们**——只是有两处没接住（退出码短路、认证 banner 被丢弃）。

## 二、P1 根因：信号在手里，被退出码短路

`internal/result/classify.go:39-47`：退出码非 `nil` 时**直接返回，不再匹配关键词**。

实测一次（`-c "id"`，密码与密钥登录、`direct` 与 `sudo` 身份，表现一致）：

| 字段 | 值 |
| --- | --- |
| `connect_success` | `true` |
| `exit_code` | `1` |
| `error` | 空 |
| `output` | `WARNING: Your password has expired.` + `Password change required but no TTY available.` |
| 分类 | **执行失败(退出码1)** |

词表里本来就有「密码过期」一类，`output` 里也正好有可命中内容 —— **不需要任何协议层新机制，是短路把信号吞了**。顺带查出该类现有 4 个关键词实测只有 `has expired` 能命中（sshd 说的是 `password has expired`；`change your password now` 属于交互变体，非交互场景不出现），能命中的偏偏是最宽泛、最易误伤的一条。

## 三、P2/P3 为什么连原因都拿不到：两处没接住

拒绝点决定客户端能拿到什么：

| | 会话被拒（P1/P4/P5/P6） | 认证阶段被拒（P2/P3） |
| --- | --- | --- |
| 拒绝点 | 认证**通过之后**（PAM session / 登录 shell / sudo） | 认证阶段（PAM account 预认证） |
| 表面上拿到什么 | 会话内 stderr 的拒绝语 + 非 `0` 退出码 | 只有 `ssh: handshake failed: EOF` |
| 实际漏掉什么 | 退出码短路（第二节） | **认证 banner 被丢弃**（见下） |

**突破口：`BannerCallback`。** 服务端在认证阶段把 PAM 的话当作 `SSH_MSG_USERAUTH_BANNER` 发出来，`x/crypto/ssh` 会解析它，但工具的 `ClientConfig`（`internal/ssh/client.go:39-43`）**没配 `BannerCallback`，于是解析完整包丢弃**。用 Go 直接挂上回调实测：

| 场景 | 连接结果 | banner 原文 |
| --- | --- | --- |
| 密码过期（密码登录） | 成功 | `You are required to change your password immediately (administrator enforced).` |
| 密码过期（密钥登录） | 成功 | 同上 |
| 账号过期 | 失败（EOF） | `Your account has expired; please contact your system administrator.` |
| `/etc/nologin` 存在 | 失败（EOF） | **该文件的内容原文**（实测为 `maintenance`） |
| 密码真错 | 失败 | 空（认证没过，服务端无话可说） |

结论：**banner 在失败之前就到了，一直躺在被丢弃的包里**。挂上回调即可拿到账号过期、密码过期（认证阶段变体）、`/etc/nologin` 通知等全部原因，且对「密码真错」这类真认证失败没有副作用。

顺带说明 P2/P3 之所以看起来"无解"：sshd 在预认证阶段刻意不发原因（服务端日志 `fatal: Access denied … [preauth]` + 统一话术 `Failed password`，防账号探测）。**banner 是它唯一留的口子。**

## 四、定稿设计

### 4.1 机制 A：接住服务端提示，追加进 `error`（覆盖 P2 / P3 / P7）

- `ClientConfig` 挂 `BannerCallback`，拿到的原文**追加进结果的 `error` 字段** —— 原有文本处理逻辑一行不改（开发者 2026-09-16 裁定）。
- 于是它天然进入现有分类链路：第一块匹配 `error` 时就能命中「账号过期」等判据；传输模式（连接成功、SFTP 失败）也靠它把原因带上来。
- 追加规则：`error` 为空则直接作为其内容；非空则换行追加到尾部（原有报错原文保留在前）。
- **副作用需知**：服务端提示可能是 MOTD / 法务声明这类无害内容——sshd 配了 `Banner` 的站点，每行 `error` 都会带上那段公告。分类不受影响（退出码为 `0` 时退出码优先），但明细的 `error` 列会有这段文本。

### 4.2 机制 B：退出码非 `0` 时可覆盖分类（覆盖 P1 / P4 / P5 / P6）

错误关键词表**拆为两块，触发条件互斥**：

| 判据块 | 触发条件 | 匹配文本 | 现有行为 |
| --- | --- | --- | --- |
| `[[categories]]`（现有） | 退出码为 `nil` | `error`（服务端提示已并入）→ 未命中再看 `output` | 文本处理逻辑一行不改 |
| `[[exit_code_categories]]`（新增） | 退出码非 `0` | `error` 与 `output` 的合并文本 | 命中即覆盖分类；**退出码原样保留** |

```toml
# 第一块：认证阶段（服务端提示里出现）
[[categories]]
name = '密码过期'
keywords = ['You are required to change your password immediately']

# 第二块：会话被拒（会话内的拒绝语）
[[exit_code_categories]]
name = '密码过期'
keywords = ['Password change required but no TTY available']
```

**同名分类允许两块各写一遍**（开发者 2026-09-16 裁定）：同一个诊断横跨两个阶段——「密码过期」既可能以认证阶段的服务端提示出现（无退出码侧），也可能以会话内拒绝语出现（非 0 退出码侧）。两块各登记该阶段会出现的文案，**谁命中就按谁分类**。

实现上 `Has()` / `Names()` 必须取**两块并集**（`Names()` 去重）：漏掉第二块的名字，`IsFallbackCategory` 会把「密码过期」当成兜底原文（`internal/output/terminal.go:159`）。

块内仍按现有「自上而下取首个命中」，因此具体类要排在「SSH协议错误」这类通用类之前。

**命中只改分类，退出码原样保留**（开发者 2026-09-16 二次裁定）：退出码字段与它的显示逻辑一行不动——命令确实以非 `0` 结束了（只是这个码来自拒绝方），保留它不碍事；**能正常分类即可**，不做额外加工。

### 4.3 判据词表（全部取服务端固定文案）

| 分类 | 判据块 | 判据 | 出处 | 会否本地化 |
| --- | --- | --- | --- | --- |
| 密码过期 | 第一块 | `You are required to change your password immediately` | pam_unix（服务端提示） | 否 |
| 密码过期 | 第二块 | `Password change required but no TTY available` | sshd（会话内） | 否 |
| 账号过期 | 第一块 | `Your account has expired` | pam_unix（服务端提示） | 否 |
| 登录被禁止 | 第二块 | `此帐户目前不可用。` / `This account is currently not available.` | nologin（shadow-utils） | **是**（本机 `zh_CN` 实测中文） |
| 仅允许 SFTP | 第二块 | `This service allows sftp connections only.` | sshd | 否 |
| sudo 被拒 | — | **不立判据**（开发者 2026-09-16 裁定）；维持现有原文回退 | sudo | 否 |

三条口径：

- **判据要长、要取自服务端固定文案** —— 精度全部来自这里，与退出码取值、与块的位置都无关。
- **中英双份只给会漂的那条**：sshd 不翻译自己的消息；`sudo` 在工具包装**内**执行、已被 `export LC_ALL=en_US.UTF-8` 强制成英文；只有包装**外**由 sshd 拉起的 nologin 跟随系统 locale。
- **能整句就别拆**：现有语义是「不含 `*` 即子串包含（大小写不敏感）；含 `*` 则片段按顺序出现」，为"更长"把句子拆成一串 `*` 会变成乱序命中、精度下降。

### 4.4 修正：`SSH协议错误` 收 `eof` 是错的（覆盖 P2 / P3 的错标）

`ssh: handshake failed: eof` 现在挂在「SSH协议错误」下，而 EOF 是**万金油症状**：服务端拒绝、网络中断、中间设备掐断都会是它。它天然是一类**事实**（认证阶段被对端断开），不是**原因**。

处理：把这条从「SSH协议错误」里摘掉。挂上 banner 之后，账号过期与 `/etc/nologin` 都能被 4.3 的词命中；剩下真正无原因的 EOF 落回原文，不再冒充"协议错误"。（是否再单立一类中性的「握手被断开」——待定，见第九节。）

### 4.5 修正：`MaxSessions` 一类的文案（覆盖 P8）

`MaxSessions 0` 现分类是「会话创建失败」，能命中靠的是**工具自己的前缀**，是通用名（任何建会话失败都叫这个），看不出原因，且它走 error 侧、与机制 B 无关。

处理：另开一类，关键词取 `ssh: rejected: connect failed ("open failed")`（x/crypto 特征串 + 服务端原因，升库要跟）。

### 4.6 判据精简（覆盖 P9）

**口径（开发者 2026-09-16 定）**：Go 侧依赖库的报错是**固定文案**，穷举一次就够；靠"经验"攒出来的变体（多是 Python + paramiko 时代留下的）在 Go 侧根本不会出现，一律删。**操作系统/服务端输出的报文是真实经验，保留。**

对账方法：把 `x/crypto/ssh`、`pkg/sftp`、`net`/`os`/`syscall`/`context` 的源码文案，与工具自己实现里的文案，汇成一份"真相集"，再拿原有 91 条关键词逐条撞——**每条只允许落在三种来源之一**，落不到就删。

| 处置 | 条目 |
| --- | --- |
| **删 · 库文案但 Go 侧不会出现** | `authentication timeout`、`connect timeout`、`connection timeout`、`password not accepted`、`name or service not known`、`cannot resolve hostname`、`hostname not found`、`dns resolution failed`、`unknown host`、`connect refused`、`conn refused`、`auth failed`、`socket closed`、`disconnected`、`connection closed`、`network unreachable`（是 `network is unreachable` 的子串，冗余）、`reset by peer`（同上）、`refused`（过短） |
| **删 · 实现里已无对应报错（旧 Py 文案）** | `远程目标路径不存在`（真实是 `远程路径不存在`）、`不存在或不是目录`、`远程目录为空`、`没有下载到任何文件`、`本地文件不存在或不可读`、`读取本地文件权限失败`、`sudo权限`、`上传超时被取消`、`认证失败`、`密码错误`、`password expired`、`change your password now`、`has expired`（过短，误伤）、中文连接类词（`连接中断`/`连接已断开`/`管道破裂`/`拒绝连接`/`连接被拒绝`/`拒绝网络连接`）、中文网络类译文（`目标主机不可达`/`无法访问目标主机`/`无路由到主机`）、`ssh protocol error`、`incompatible ssh version`、**`ssh handshake failed`（真实文案是 `ssh: handshake failed:`，少一个冒号，从未命中过）** |
| **删 · 整类因此空掉** | 「上传超时」——唯一关键词是死词；Go 侧该场景回落到原文或「任务已取消」 |
| **改成长句（换成库/系统真实文案）** | `disconnected` → `ssh: disconnect, reason`；`unknown host` → `no such host`；`already exists` → `file already exists`；SSH协议错误 → `unsupported protocol` / `junk character in version line` / `protocol mismatch` |
| **补上实现侧真实文案（此前漏收）** | 本地文件错误 + `文件不可读`；远程无文件 + `没有可下载的文件`；SFTP失败 + `创建远程目录失败`；会话创建失败 + `创建输入通道失败` |
| **删（误标根源）** | `ssh: handshake failed: eof`（EOF 不是协议错误，见 4.4） |
| **保留** | 库/标准库真实文案（`no route to host`、`network is unreachable`、`host is unreachable`、`too many open files`、`i/o timeout`、`context canceled`、`connection lost`、`use of closed network connection`、`broken pipe`、`connection refused`、`connection reset`、`unable to authenticate`、`permission denied (publickey|password|keyboard`、`[none publickey]`、`ssh: command * failed`、`ssh: could not start shell`、`remote command exited without exit status`、`permission denied`）；工具自撰文案；服务端输出（sshd / PAM / nologin / sudo） |

**结果**：关键词 **91 → 55 条**（-40%），分类 **25 → 28 个**（删掉空掉的 1 个，新增「账号过期」与第二块的 3 个）；**没有一条判据失去出处**。判据文件的头部已写明"只允许三种来源"的口径，供后续维护照此增删。

**一处例外保留**：`access denied` —— 它是服务端会原样回的标准短语（不是单个通用词），删掉会丢掉一部分服务端的真实回话。

### 4.7 机制 B 的匹配范围（边界声明）

凡退出码非 `0` 的结果都会进机制 B，**包括「预处理失败」**（上传/下载建连之后、逐文件传输之前的命令失败，以真实非 `0` 退出码表达，见 `GLOSSARY.md` 同名词条）。实测该路径的文案不命中任何判据，行为不变——此条只是把边界写清。

### 4.8 明确不做的两件事

- **不为"用户命令把 stderr 吞掉"（如 `id 2>/dev/null`）设计补偿**（开发者 2026-09-16 裁定：这种用法极端，不考虑）。原先设想的"失败后补一次探测会话"**不纳入**——它会让目标机多一次会话、多一条命令（可能进对方审计日志），收益对不上代价。
- **不动退出码字段与它的显示逻辑**（见 4.2）：命中只改分类。

### 4.9 分类提示块（2026-09-16 追加）

**要解决的问题**：有些分类底下不止一种情况（最典型的是「握手被断开」——认证阶段被掐断、原因不下发），光看分类名仍然不知道往哪查。

**做法**：在**统计块末尾**（横排不动、不再另起竖版）追加一个提示块：

```
  失败分类统计 >>>  密码过期：1  身份验证失败：1
  提示：
    身份验证失败：密码错、私钥不被认、认证方式不被接受
```

| 环节 | 归属 |
| --- | --- |
| 提示内容 | 配置文件 `error_keywords.toml` 里每个分类的 `tip` 字段（**可选**，不写就不显示） |
| 是否显示 | 配置 `enable.show_category_tips`（**必填**）；关闭时退回旧行为——只显示一行「常见退出码」 |
| 「执行失败(退出码N)」 | 由**工具按退出码自动给**（分类名带数字，配置文件里写不了）：`命令自身失败，或命令没跑起来被拒（未识别出具体原因）。退出码 N = <含义>` |
| 输出位置 | 终端统计块 **与** `report.txt` 的【结果统计】段（两处口径一致） |

**内容口径**（开发者 2026-09-16 定）：只写"这个分类可能包括哪些情况"，**不写怎么修**——像密码过期、账号过期这种原因唯一的分类，本来就不需要提示，所以不写。

**新分类「握手被断开」**：判据 `ssh: handshake failed: EOF`，中性命名（与已有「握手超时」成对），提示词列出可能情况。**位置有讲究**：必须排在任何"由服务端提示命中的分类"之后（账号过期、密码过期）——实测放在「握手超时」后面时，`建立连接失败 - ssh: handshake failed: EOF\nYour account has expired…` 这类会被它抢先命中，把账号过期吃掉。这正是本文件头部第 2 条规则（具体类往前、但要让带更具体信息的先说话）。

## 五、实测证据（2026-09-16，Ubuntu 26.04 / OpenSSH 10.2，`UsePAM yes`）

| 场景 | connect | exit_code | error | output / banner | 现分类 |
| --- | --- | --- | --- | --- | --- |
| 密码过期（密码登录） | true | 1 | 空 | `WARNING: Your password has expired.`<br>`Password change required but no TTY available.` | 执行失败(退出码1) |
| 密码过期（密钥登录） | true | 1 | 空 | 同上 | 执行失败(退出码1) |
| 密码过期（`-m sudo`） | true | 1 | 空 | 同上 | 执行失败(退出码1) |
| 登录 shell 为 nologin | true | 1 | 空 | `此帐户目前不可用。` | 执行失败(退出码1) |
| `ForceCommand internal-sftp` | true | 1 | 空 | `This service allows sftp connections only.` | 执行失败(退出码1) |
| 账号不在 sudoers | true | 1 | 空 | `sudo: I'm sorry sftest2. I'm afraid I can't do that` | 执行失败(退出码1) |
| `MaxSessions 0` | true | **nil** | `创建会话失败 - ssh: rejected: connect failed ("open failed")` | 空 | 会话创建失败 |
| 账号过期 | **false** | nil | `建立连接失败 - ssh: handshake failed: EOF` | banner：`Your account has expired…` | **SSH协议错误** ❌ |
| `/etc/nologin` 存在 | **false** | nil | 同上 | banner：文件内容原文 | **SSH协议错误** ❌ |
| 密码被锁 / shell 不存在 | false | nil | `ssh: unable to authenticate…` | 空 | 身份验证失败 |
| 对照：正常账号 | true | 0 | 空 | 无 banner | 执行成功 |

已排除（不产生特殊报文）：密码修改日期设在未来（命令照常成功）、`pam_limits` 的 maxlogins（未拦截）。

> 判据出处核对：`WARNING: Your password has expired.`、`Password change required but no TTY available.`、`This service allows sftp connections only.` 均在目标机 `/usr/sbin/sshd` 的字符串表里逐字命中 —— 是 sshd 自己打的，**不是用户命令的输出**，这是它可以安全充当判据的前提。

## 六、与既有决策的关系

- **D11**（`rewrite-to-go/spec.md`）约束的是**迁移时** YAML → TOML 逐字节平移；本轮是迁移完成后的语义修正，不构成违反。文档上明示：**关键词表的语义自此不再冻结**。
- **`classify.go` 文件头把退出码语义引作「ADR-0003 退出码语义」是误引** —— ADR-0003 的主题是「执行模式判定不做集中化」。退出码语义此前只存在于 `GLOSSARY.md` 词表与该注释里，已由 ADR-0004 正式承接，实施时一并修掉那句注释。
- **`AuthFailure`（D44）**仍是"由 SSH 层直接给短分类文案"的既有先例；本轮的 banner 走"原文入结果 + 关键词外置"的路子，两者不冲突：前者判据是库文案、结论固定；后者判据是服务端自由文本、分类靠词表。

## 七、改动点清单（已实施 · 2026-09-16）

| 文件 | 改动 |
| --- | --- |
| `config/error_keywords.toml` | 重写：新增 `[[exit_code_categories]]` 块（密码过期 / 登录被禁止 / 仅允许 SFTP）；「密码过期」两块各写一遍；新增「账号过期」；按 4.6 精简（91 → 55 条关键词）；文件头写明"判据只允许三种来源"的口径 |
| `internal/result/keywords.go` | 双表加载与校验；`Names` / `Has` / `KeywordsOf` 取两块并集（`Names` 去重、`KeywordsOf` 合并且同名合并）；匹配入口分两个（`match` / `matchExitCode`）；保留 `match` 的 nil 保护 |
| `internal/result/classify.go` | 退出码非 `0` 时先跑第二块匹配（匹配文本 = `error` + `output`），命中即覆盖分类；退出码字段照旧；修正文件头注释的 ADR 误引；新增 `joinText` |
| `internal/ssh/client.go` | `ClientConfig` 挂 `BannerCallback` 收服务端提示；`applyBanner` 把提示并入 `Result.Error`（干净成功的行不并，避免把公告写进正常行）；新增 `isCleanSuccess` |
| `internal/ssh/exec.go` / `internal/ssh/sftp.go` | 三条执行路径（命令 / 上传 / 下载）各加 `defer c.applyBanner(result)` |
| `testdata/error_classification_cases.toml` | 修 4 条因精简而失效的旧用例（改用实现侧真实文案），新增 5 条（会话被拒三类 + 认证阶段提示两条 + "判据未命中仍按退出码"的回归），共 **29 条** |
| `internal/output/terminal.go` | `PrintStatistics` 加 `showTips` 开关：开启时输出「提示：」块（`categoryTipLines`），关闭时保留旧的「常见退出码」行；新增 `exitCodeFailTip` |
| `internal/output/report.go` | `WriteReport` 加 `kw` 入参；【结果统计】段同样输出「提示：」块（与终端同口径） |
| `internal/config/config.go` | 新增 `enable.show_category_tips`（**必填**，缺字段启动即报错——与既有"全字段必填"口径一致） |
| `main.go` / `internal/output/output.go` | 把开关与 `kw` 传到两个输出端 |
| `internal/output/terminal_test.go` | 新增两个单测：`TestCategoryTipLines`（含"没写 tip 不显示"与退出码含义拼接）、`TestPrintStatisticsTipSwitch`（开关开/关的行为） |
| `config/error_keywords.toml` | 8 个分类加 `tip`；新增分类「握手被断开」（判据 `ssh: handshake failed: EOF`，位置排在带服务端提示的分类之后） |
| `CHANGELOG.md` + 程序入口版本常量 | 实施当天开段（规范四、五） |

配置与发布包：`config/error_keywords.toml` 在 `build/`、`release/` 下各有一份副本，改源后需重新打包才对已发出去的包生效（本轮已同步 `build/`，`release/` 待下次打包）。

## 八、验证结果（2026-09-16 真机 · 测试机 172.28.118.49）

单测：`go test ./...` 8 个包全绿；错误分类语料 **29/29 通过**（其中 4 条旧用例按实现侧文案修正）。

真机 11 个场景，全部符合预期：

| 场景 | 分类 | 备注 |
| --- | --- | --- |
| 正常账号（对照） | 执行成功 | 无回归 |
| 密码过期（密码登录 / 密钥登录 / `--nobash`） | 密码过期 | 三条路径一致 |
| 账号过期 | 账号过期 | 原为「SSH协议错误」❌ |
| 登录 shell 为 nologin | 登录被禁止 | |
| `ForceCommand internal-sftp` | 仅允许 SFTP | |
| 密码真错 | 身份验证失败 | 服务端提示为空，无副作用 |
| 命令真的失败（`exit 3`） | 执行失败(退出码3) | 退出码权威仍生效 |
| 下载模式遇密码过期 | 密码过期 | 原为「SFTP失败」（P7 已解决） |
| `-m sudo` 遇密码过期 | 密码过期 | |

复现配方（测试机）：

```bash
useradd -m sftest1 && echo 'sftest1:Test@12345' | chpasswd && chage -d 0 sftest1   # 密码过期
chage -E 2020-01-01 sftest2                                                        # 账号过期
usermod -s /usr/sbin/nologin sftest2; usermod -s /bin/bash sftest2                 # nologin shell（用完改回）
printf 'Match User sftest2\n    ForceCommand internal-sftp\n' > /etc/ssh/sshd_config.d/99.conf  # 用完删除
```

## 九、决策记录

### 已裁定（2026-09-16）

| 事项 | 结论 |
| --- | --- |
| 机制 B 的形态 | 两块表 + 同名分类两块各写一遍，谁命中就按谁分类（4.2） |
| 服务端提示落到哪 | **追加进 `error` 字段**，原有文本处理逻辑不变（4.1） |
| 认证阶段被拒、判据未命中 | 落回原文，不另立中性分类；关键词文件是给人调整的配置，评估后自己改 |
| `P6` sudo 被拒 | 不立判据 |
| 退出码怎么处理 | **只改分类，退出码原样保留**，现有显示逻辑一行不动（4.2、4.8。此为二次裁定，前一次"命中后清除退出码"已作废） |
| 判据宽泛词 | **精简**（4.6）：Go 库的报错是固定文案，穷举一次即可，靠经验攒的变体一律删；OS/服务端报文属经验，保留 |
| 「用户命令吞掉 stderr」的补偿 | **不做**（4.8）：按裁定属极端用法；原设想的"失败后补探测"不纳入 |
| git 追踪范围 | `.gitignore` 只放行 `docs/adr/`；`docs/issues/`（33 个）与 `docs/agents/`（3 个）已 `git rm --cached` 取消追踪，磁盘文件保留 |
| 提示块的排版 | **不做竖版**（开发者 2026-09-16 否决）；保持横排，末尾追加「提示：」块（4.9） |
| 提示词的内容口径 | 只列"这个分类可能包括哪些情况"，**不写怎么修**；原因唯一的分类不写（4.9） |
| 提示词放哪 | 配置文件的 `tip` 字段（用户维护）；`执行失败(退出码N)` 由工具按退出码自动给 |
| 新分类名 | 「**握手被断开**」（与「握手超时」成对）；位置须排在带服务端提示的分类之后 |

### 未决

（无。）

## 十、关联

- `docs/adr/0004-非零退出码下的判据覆盖.md` —— 机制 B 的 ADR（含备选方案与拒绝理由）
- `docs/adr/0005-用banner作认证故障原因.md` —— 机制 A 的 ADR
- `GLOSSARY.md` —— 词条「会话被拒」「退出码」「服务端提示」
- `docs/issues/2026-09-11-1336-Go重写/spec.md` D11 / D44 —— 关键词表迁移约束与认证失败分类的既有决策
