# 实测报告：该复用却自造——逐条三问

**跑于** 2026-11-13 15:20，`HEAD = 7549a62`（顺序议题收尾后的第一笔）。只读：`git status -- modules` 为空。
**手法**：三份料交叉——老工程 `internal/common/` 与 `internal/ssh/` 的符号清单、`docs/issues/2026-11-10-1720-老新交集收全/audit.md`（交集 48、逐字同体 20）、新工程里同名或近义的自造判据；每一条只问三件：老的哪把尺子（紧还是松）、新的现在是什么、为什么不是直接搬。第三问答不出的记为「无理由自造」。
**实跑**：两处口径差用 `build/该复用_2026-11-13/` 的对照小程序跑出来（只读，不进版本控制）。

## 一、总数

| 项 | 数 |
| --- | --- |
| 老 `common` 顶层符号 | 31 |
| 老 `ssh`／`cli` 里的判据（归一、解释器表、身份、转义） | 5 |
| 这一轮过筛的判据 | 11 |
| 已合（照搬，一处定义） | 4 |
| **仍两处以上各写一套** | **6** |
| 登记为「留两份」并给理由 | 1 |

## 二、逐条

| # | 判据 | 老（哪把尺子） | 新（现在是什么） | 为什么不是直接搬 | 处置 |
| --- | --- | --- | --- | --- | --- |
| 1 | 合法 IPv4 | `common/ip.go` `netip.ParseAddr` ＋ `Is4`，拒前导零 | `criterion/ipv4.go` 同一句 | 已搬 | 已结（`1dbac65`、4.3） |
| 2 | 剥系统前缀 | `common/systemreason.go` 两支（`*fs.PathError`、`*os.LinkError`） | `criterion/systemreason.go` 逐字同 | 已搬 | 已结（4.3） |
| 3 | 终端显示宽度 | `common/width.go`，`x/text` 的 EAW＝W/F 占 2 列、Mn/Me/Cf 占 0 | `message/width.go`，同一判据，EAW 表由生成器逐字符扫出 | 已搬（零第三方依赖，表自己带） | 已结（4.3） |
| 4 | 单引号转义 | `ssh/command.go`：一处三形状——`escapeSingleQuotes`（只转义）、`shellQuote`（包裹）、`QuoteForShell`（按需包裹），转义规则只写一遍 | **三份各写一遍**：`parse/downlink.go:197`、`execute/remote.go:35`、`message/stats.go:245`，都是 `strings.ReplaceAll(s, "'", "'\\''")`，互不共享 | 答不出。`execute/remote.go:33` 的注释写「与解析站那一处同源……不再各写一遍」——它自己就是另一份 | **搬回**：`criterion` 三形状 |
| 5 | 解释器名归一 | `ssh/command.go:445` 一份：取第一个词、`path.Base`、小写、去尾部版本号 | `parse/downlink.go:287` 照搬（好）；`message/confirm.go:148` `isBashInterpreter` **手搓第二份**：自己按 `/` 与 `\` 都切 | 答不出。注释称「与解析站那张解释器表同源」，实测两处结论不同（见 §三·2） | **搬回**：`criterion.InterpreterBaseName`，两处都调。只认 `/`（照老版）：解释器跑在目标机上，`\` 不是它的分隔符 |
| 6 | 解释器名表 | `ssh/command.go` 一张表带属性（程序文本参数、是否 shell 家族、`source` 前关什么） | `parse/downlink.go:228` 属性表 15 项；`message/modename.go:131` 显示名单 15 项（顺序即显示顺序） | 答得出：属性是机器要的，顺序与句子是显示要的，`message` 不得依赖 `parse`；漂了两边名字，`parse` 有一条按集合比对的用例会红 | **留两处**，理由记在此行 |
| 7 | CSV 读入口径 | `common/csvtext.go` `ReadCSVRows` 一份：剥 BOM、`#` 注释跳过、空行跳过、列数不限、引号成对可含逗号换行；清单与代填共用（ADR-0023 决定 5） | **两份**：`parse/nodelist.go:120`（严格）与 `mask/csv.go:13`（`LazyQuotes = true`） | 答不出。宽松那侧没有任何注释说为什么宽 | **搬回**：一份，恒认注释、一律严格；取值侧与遮罩侧都调它 |
| 8 | 算不算一条内联记录 | `common/mask.go` 的 `LooksLikeInlineNodeRecord` 一份：恰好一条、首列是 IPv4；取值判定与脱敏两处调（ADR-0023 决定 5 点名） | **两份**：`parse/nodelist.go:173`、`mask/inline.go:44`，各调各的 CSV 读法 | 答不出 | **搬回**：`criterion.LooksLikeInlineNodeRecord` |
| 9 | 算不算二进制 | `common/csvtext.go` `IsBinaryContent` 一份：出现 NUL 就算，**只看前 1KB**；脚本、清单、代填三处用 | **两份**：`parse/answer.go:180`（逐字节）、`parse/material.go:72`（`bytes.IndexByte`），**都查全文** | 答不出。且与老版口径不同：老版前 1KB 是性能取舍，不是判据 | **搬回**：同包收成一处（`parse/binary.go`）。不进 `criterion`——准入要两个以上包，这条只有解析站在用 |
| 10 | 执行身份标签 | 老版 `Args.Identity()`（`sudo && !noShell` → root） | **两份逐字同体**：`execute/log_session.go:122`、`message/confirm.go:137` | 答不出。`execute` 已依赖 `message`，搬得动 | **搬回**：`message.IdentityLabel`，`execute` 调它 |
| 11 | 速度格式化 | `output/terminal.go` `FormatSpeed` | `message/stats.go` 同体 | 已裁：登记成照搬的格式化规则，代码不动（4.7 §五） | 不动 |

## 三、实跑证到的两处差

**1. CSV 宽松那侧比取值侧宽。** 同一组输入，两处结论相反：

| 输入 | 取值侧（严格） | 遮罩侧（`LazyQuotes`） |
| --- | --- | --- |
| `10.0.0.1,22,root,p"w` | 读不出（`bare " in non-quoted-field`） | 读得出，密码段被遮 |
| `10.0.0.1,22,root,"p"w` | 读不出（`extraneous or missing "`） | 读得出 |
| `10.0.0.1,22,root,pw"` | 读不出 | 读得出 |
| `10.0.0.1,22,ro"ot,pw` | 读不出 | 读得出 |

方向是「遮罩比取值宽」，所以**没有泄密面**（取值认的，遮罩一定也认）。失守的是 ADR-0023 决定 5 那条：同一把尺子变成两把，宽的那把没人说得清为什么宽。

**2. 解释器归一在 Windows 路径写法下两处两个结论。**

| 配置里的值 | 解析站归一成 | 显示侧归一成 |
| --- | --- | --- |
| `C:\Python\bash` | `c:\python\bash`（表里没有，不认） | `bash`（认） |
| `C:\bin\zsh5` | `c:\bin\zsh5`（不认） | `zsh`（认） |
| `/usr/bin/python3` | `python` | `python` |

后果是同一份配置两个判断：确认屏当它是默认 `bash`，把「解释器」那一行省掉；`-a` 那条判据按解析站的结论说「不支持解释器」。老版只有一份（`path.Base`，只切 `/`），所以老版不会有这条不一致——这是新版抄第二份时多出来的。

处置时选的方向：**两处都按解析站那条判**（只认 `/`），不是放宽到两处都认。理由：解释器是喂给目标机的，`C:'+bs+'Python'+bs+'bash` 在那儿不存在；归成 `bash` 会把它放行到执行期才失败。现在是屏上照实印出这一格、`-a` 照实说「不支持」，同一个结论，且错在解析期。

## 四、不搬的两条，理由

- **解释器名表**（第 6 行）：两个落点各有职责，且依赖方向不许反过来。漂了会红（`parse/downlink_test.go` 按集合比对）。代价是新增解释器要改两处——用例当场指出，可接受。
- **速度格式化**（第 11 行）：4.7 已裁，属「照搬的格式化规则」，理由是输入受保护、瑕疵只在边界。

## 五、已知开口（本议题不修，另记）

- **被拒绝的畸形值会不会带明文进日志**：遮罩侧读不出就原样返回（与老版同）。**收口后核过，到不了日志**：argv 只在 `mask.CommandLine` 一处脱敏，而它唯一的调用点是 `report.Run`，报告只在真跑完之后写；拒绝与起不来那两条路不写报告，工具日志里只有话、没有 argv。所以去掉 `LazyQuotes` 不会把明文带进任何地方，这条开口只剩「两处口径不一致」本身——工单 03 把它消掉。
- **`bash.exe` 这类带扩展名的写法**：去尾部版本号不剥 `.exe`，老版也不剥。两版都认不出，属既有口径，不动。

## 六、未覆盖

- **方法（带接收者）不在交集表内**：4.7 只收导出顶层函数。本轮靠人工读源码补看，没做全量扫描。
- **Windows／unix 同名符号只留一份**：4.7 §六 已记，本轮沿用同一局限。
- **没跑 A/B**：本轮只读。搬动的四处按各自包既有测试＋收尾那轮 37 场复跑兜。
