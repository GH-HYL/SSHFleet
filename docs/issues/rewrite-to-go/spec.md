# SSHFleet 全 Go 重写 · 方向稿

Status: needs-info

> 本文只约束**大方向**，不含具体代码设计。
> 「已定」条目均为用户裁定结果；「待定」条目不得据此动手。
>
> 配套文件：`plan-coverage.md`（规划覆盖面与每期推进规程）。动工前先过面。

---

## 一、背景

SSHFleet 是 SSH 批量运维工具：对清单内多台服务器批量执行命令 / 脚本、上传 / 下载文件，并归档结果。

现役实现（已移入 `modules/SSHFleet_bak/`，只读参考）为 Python + Go 混合架构：

| 层 | 规模 | 职责 |
| --- | --- | --- |
| Python 编排层 | 5505 行 / 38 文件 | 参数解析、清单读取、凭据管理、危险检测、结果统计、呈现与归档 |
| Go 执行引擎 | 2618 行 / 21 文件 | 高并发 SSH 连接与命令执行、SFTP 上传下载 |

两进程通过环境变量（端口 / 认证 key / 日志路径）+ 本地 HTTP + SSE 通信，属"一次性引擎"模型：Go 端起服务后只处理一个请求，随后等待关闭信号退出。

---

## 二、目标

以 Go 重写整个工具，产出**单一可执行文件**。原 Python 编排逻辑与原 Go 引擎逻辑合并为同一进程内的一条主干流程。

**这是重构，不是从头开发** —— 目标是一个经得起后续演进的好架构，而不是尽快跑通功能。

---

## 三、已定决策

### 3.1 形态与架构

| # | 决策 | 结论 |
| --- | --- | --- |
| D1 | 产物形态 | 单个可执行文件，一次调用跑完全流程 |
| D2 | 事件流模型 | 不保留。`init` / `progress` / `result` / `done` 四类消息随跨进程管道一起消亡；进度与统计改由进程内的**聚合器**组件承载 |
| D3 | 双进程设施 | 全部下葬——子进程管理、端口探测、健康检查线程、关闭信号、`X-SSH-Fleet-Key` 认证、`ALREADY_USED` 单次使用限制、无请求自动退出、`/api/v1/*` 全部端点、`SSH_FLEET_KEY` / `SSH_FLEET_PORT` / `SSH_FLEET_LOG_PATH` 环境变量 |
| D4 | 术语 | "编排层 / 执行引擎 / SSE 消息 / SSE 会话"等旧术语作废，词表重写 |
| D12 | 工程骨架 | `main.go` 承载主干十步 + `internal/` 下 11 个目录（见第五节） |
| D21 | 框架冻结 | 骨架一经定稿即为**硬边界**。此后所有修改、重构、新增功能都在这套框架内调整，不因局部便利而改动框架形状 |
| D22 | 内聚优先 | 逻辑上属一体的东西保持整体，不为拆而拆；允许函数调用链加深（不追求扁平） |

### 3.2 工作方式

| # | 决策 | 结论 |
| --- | --- | --- |
| D5 | 依赖基调 | **功能优先**：有成熟好用的库就用，不为"极简依赖"自造轮子 |
| D8 | 取舍原则 | 全 Go 重写即全面择优，**不做**一比一复刻 |
| D20 | 功能对照方式 | **不做功能台账**。重构期以「功能」为思考范围，逐个打开旧代码对应实现，三问：① 能不能优化 ② 优化后是否**保持原功能** ③ 保持不了、或确有更优方案 → **必须 ask 用户裁定**。旧代码是一手事实来源，不维护二手摘要 |
| D23 | 测试 | 本次规划**不涉及测试策略**（属 TDD 实施范畴） |
| D9 | 文档基线 | 旧 `CONTEXT.md` 与 ADR-0001–0013 全部作废，新仓库重写；`CHANGELOG.md` 版本号从 **5.0.0** 起（不兼容重写） |

### 3.3 技术选型

| # | 决策 | 结论 |
| --- | --- | --- |
| D24 | 命令行形态 | **保持平级选项**（`sshfleet -f nodes.csv -c "df -h"`）。**不引入子命令**。模式互斥手写校验，提示文案自撰（可读性优于框架原生报错） |
| D6 | 配置格式 | 改为 **TOML**；字段按新结构增删，不做旧 YAML 兼容 |
| D13 | 节点标识 | **只支持 IPv4 字面量**。严格校验（`netip.ParseAddr`），每段范围非法即报错；域名明确报错"不支持域名"，**不得静默丢弃**（旧版第一行域名会被当表头删掉，属缺陷） |
| D14 | 凭据加密 | 新写入 **AES-256-GCM**（`crypto/aes` + `crypto/cipher`，密钥派生 `crypto/hkdf`），文件版本 `0x02`；旧 `0x01`（SHA256 计数器密钥流 + HMAC）**保留只读解密**，`--convert-password` 顺带升级。全部标准库，零外部依赖 |
| D15 | 主密钥环境变量 | 统一为 `SSHFLEET_KEY`（旧架构存在两个名字，现归一） |
| D11 | 规则资产 | `dangerous_keywords` / `error_keywords` 两份规则文件的**语义与正则写法原样沿用，格式一并转为 TOML**（`[[rule]]` / `[keywords.<分类>]`）。TOML 字面量字符串 `'...'` 与 YAML 单引号同为不做转义的写法，正则可逐字节平移；转换后需逐条比对条目数与每条正则字符串 |
| D16 | 日志 | `go.uber.org/zap`（含自定义 SUCCESS 级别） |
| D17 | 终端 UI | 只引 `lipgloss` 做样式；进度条渲染自写，**不引 bubbletea**（TUI 框架会接管终端事件循环，与"禁止子模块反向驱动主流程"冲突） |
| D18 | SSH / SFTP | `golang.org/x/crypto/ssh` + `github.com/pkg/sftp` |
| D19 | xlsx | `github.com/xuri/excelize/v2` |
| D25 | 危险检测解析层 | 引 `mvdan.cc/sh/v3/syntax` 产出语法树；「包装命令剥除 / 旗标归一化 / 内部命令递归」等**语义**自行实现；解析失败降级为整行直接正则匹配 |
| D26 | CLI 框架 | `spf13/pflag` 单独使用（**不引 cobra**）：原生支持长短名并存与"选项可选值"，正好落 `-k` 三态；模式互斥校验与错误提示 100% 自写 |
| D27 | 忽略规则 | 工作区根 `.gitignore` 采用**黑名单**写法（只忽略 `modules/SSHFleet_bak/`、`build/`、`release/`、Go 编译产物）。**不采用**"忽略所有 + 白名单"——旧仓库正是后者，导致 `test/`、`tools/` 从未进入版本控制 |

### 3.4 迁移与工作区

| # | 决策 | 结论 |
| --- | --- | --- |
| D7 | 工作区 | 工作区 `D:\Desktop\Code\SSHFleet`；新工程 `modules/SSHFleet_Go/`；旧工程位于 `modules/SSHFleet_bak/`（只读、无 `.git`） |

---

## 四、待定

- **配置 schema 定稿**（草案见附录 A，待用户过目）
- 里程碑内更细的工单拆分（M1 开工时再定）

---

## 五、硬性约束与主干骨架

依据 `个人开发规范.md`（原文为硬性约束，按字面执行）：

- 入口文件 `main.go` 置于工程根目录，承载「初始化 → 运行 → 退出」主干全流程
- 其余文件均为主干某一环节的分支实现，由入口统一调用并回收结果
- **禁止**子模块反向驱动主流程（回调控制生命周期、全局状态跨模块流转）
- 工程根必须建 `internal/`：其内只建功能目录、不放散文件，且必须含 `log` 模块
- 按功能分目录存放；单文件过大时按**功能边界**拆分，不按行数机械切割
- Git 提交信息中文，格式 `类型(作用域): 简短描述`
- `CHANGELOG.md` 只记用户能直接体验到的功能新增、体验优化、问题修复；「待定 → 转天定版」

### 主干十步与承载目录

| # | 主干步骤 | 承载目录 |
| --- | --- | --- |
| 1 | 加载配置（TOML） | `internal/config` |
| 2 | 初始化日志 | `internal/log`（规范硬性） |
| 3 | 解析命令行 | `internal/cli` |
| 4 | 工具模式分流：keygen / convert-password | `internal/credential` |
| 5 | 参数合规检查 + 危险命令检测 | `internal/dangercheck` |
| 6 | 读取清单 + 字段补全 + 输入记忆 | `internal/nodelist` |
| 7 | 参数确认（交互） | `internal/confirm` |
| 8 | 并发执行 SSH / SFTP | `internal/ssh` + `internal/batch` |
| 9 | 结果聚合 + 错误分类 | `internal/result` |
| 10 | 呈现 / 报告 / 归档 | `internal/output` |
| — | 跨功能共用小工具 | `internal/common` |

**`internal/common` 定位**：集中放置被多处调用的**无状态纯函数**（路径规范化与展开、大小格式化、文本清洗、宽度计算、常量等），一处实现、处处调用，不重复造。
**红线**：`common` 内**不得**持有状态、不得承载生命周期、不得成为跨模块全局状态的中转站——否则即触犯"禁止全局状态跨模块流转"。

> 反面教材：旧 Go 的 `httpserver.Start()` 是典型的子模块接管主流程（内部起服务、等请求、等关闭信号，`main` 调完即止）。新设计必须避免这一形状。

---

## 六、里程碑（按骨架推进）

| 里程碑 | 内容 | 出口条件 |
| --- | --- | --- |
| M1 框架层 | `main.go` 主干十步就位 + `common` + `config`(TOML) + `log` + `cli`（平级选项 / 手写互斥 / 自撰提示） | 十步顺序成型，`--help` 与工具模式分流可用 |
| M2 数据层 | `credential`（三等级 + 主密钥 + 转换 + 新 AEAD）+ `nodelist`（清单 + 字段补全 + 输入记忆）+ `confirm` | 能产出完整的节点信息与凭据 |
| M3 执行层 | `ssh`（连接 / 命令 / SFTP）+ `batch`（并发池 + 进度聚合） | 可对真机批量执行 |
| M4 判定层 | `dangercheck`（解析层 + 判定层）+ `result`（聚合 + 错误分类） | 危险检测对旧语料结果一致 |
| M5 输出层 | `output`（终端 + 报告 + xlsx + 归档） | 归档结构与旧版对齐 |
| M6 收尾 | 交叉编译打包 + 文档重写 + 配置迁移说明 | 5.0.0 可发布 |

> 按骨架的分层依赖顺序推进，不做"先跑通再说"的临时形态——每一层直接按最终架构的形态实现。

---

## 七、迁移影响（对使用者）

| 项 | 影响 |
| --- | --- |
| 配置文件 | YAML → TOML，需按迁移说明重写一份 |
| 危险规则 / 错误关键词 | 语义与正则不变 |
| 凭据文件 | 旧 `0x01` 密文照常可读，无需迁移；`--convert-password` 可选择性升级到 `0x02` |
| 调用方式 | 保持平级选项，**无需改动任何既有调用方式** |
| 环境变量 | 主密钥统一为 `SSHFLEET_KEY`，已持久化该名的环境无需改动 |

---

## 八、附录 A：配置 schema 草案（待定稿）

原则：**结构简单清晰**。旧「程序路径 `paths.exe`」段随双进程架构消亡；旧「执行日志文件名」段因单进程合并而消亡。

```toml
[account]
port = 10022                # 默认 SSH 端口（清单第 2 列留空时用）
user = "root"               # 默认用户名（清单第 3 列留空时用）
secret_dir = "~/.MyPW"      # 凭据目录：相对路径都拼到这里
password_security = 3       # 密码安全等级：1=明文 / 2=base64 / 3=加密
password = "SSHFleet_pw"    # 默认密码文件路径
key = ""                    # 默认私钥文件路径
key_passphrase = ""         # 默认私钥口令文件路径

[execution]
mode = "sudo"               # 执行身份：direct / sudo
timeout_connect = 10        # 连接超时（秒）
timeout_execute = 60        # 执行超时（秒）
timeout_transfer = 300      # 传输超时（秒）

[enable]
output_to_xlsx = true       # 终端输出同时导出 xlsx
results_to_xlsx = true      # 结果固化到 xlsx

[paths]
error_keywords = "./config/error_keywords.yaml"
dangerous_keywords = "./config/dangerous_keywords.yaml"
historys = "historys"       # 历史记录目录名
log = "SSHFleet.log"        # 日志文件名（单进程后只需一个）
asset = "assets"            # 资源备份目录名
output = "output.txt"
output_xlsx = "output.xlsx"
report = "report.txt"
results_xlsx = "results.xlsx"

[upload.concurrency_thresholds]
small_file = 2097152        # < 2MB：全量并发
large_file = 20971520       # > 20MB：串行
medium_concurrency = 10     # 中间：10 并发
```

---

## 九、执行记录

### 2026-09-11 旧工程搬移

- 执行：`D:\Desktop\Code\SSHFleet_old` → `D:\Desktop\Code\SSHFleet\modules\SSHFleet_bak\`
- `.git`（194 文件）按 D7 移出工作区，暂存于 `D:\Desktop\Code\SSHFleet_old_git_backup\`
- **对账**：搬移前 1252 文件 = 就位 1058 + 移出 `.git` 194。**无文件丢失**

### 2026-09-11 参考材料清理（用户执行）

清理后 `modules/SSHFleet_bak/` 保留 8 个顶层项：`.gitignore_old` / `.scratch/` / `AGENTS.md` / `CHANGELOG.md` / `CONTEXT.md` / `README.md` / `docs/` / `modules/`。

**删除**（均为打包产物、构建中间物、平台脚本，与"读旧代码理解功能"无关）：`release/` / `build/` / `test/` / `tools/` / 6 个 `.bat` / `.vscode/` / `.workbuddy/` / 嵌套 `SSHFleet`（新工作区原胚）/ 嵌套 `SSHFleet_old`（用户备份）/ `个人开发规范.md` / `.git`。

**完整性核对**：完整副本（1252 文件）现存于 `D:\Desktop\Code\Old-Code_Bak\SSHFleet_old`，含被删的全部目录。**无内容不可恢复。**

**恢复途径差异**（重要）：

| 被删项 | 能否从 git 恢复 | 说明 |
| --- | --- | --- |
| 6 个 `.bat`、`README.md`、`CHANGELOG.md`、`.gitignore`、13 条 ADR、`modules/**` | **能**（旧 git 追踪，87 文件） | `D:\Desktop\Code\SSHFleet_old_git_backup` 或远端 |
| `test/`、`tools/` | **不能**（旧 `.gitignore` 为"忽略所有 + 白名单"，二者从未被追踪） | 只能从 `Old-Code_Bak\SSHFleet_old` 取 |

---

## 十、参考资料位置（D20 的事实来源）

| 用途 | 位置 |
| --- | --- |
| 旧代码本体（Python + Go） | `modules/SSHFleet_bak/modules/` |
| 旧决策记录 | `modules/SSHFleet_bak/docs/adr/`（ADR-0001–0013）、`CONTEXT.md`、`CHANGELOG.md`、`README.md` |
| 历次重构工作留档 | `modules/SSHFleet_bak/.scratch/`（各次 spec.md + issues） |
| **危险检测测量工具与基线** | `modules/SSHFleet_bak/.scratch/danger-rm-structured/`：`measure_recognition.py`、`verify_old_rules.py`、`old_dk_baseline.yaml` / `v1_baseline.yaml`（各 232 行）—— CHANGELOG「62.1% → 100%」那次改造的对照物，M4 验收用 |
| 危险检测 / 错误分类语料 | `modules/SSHFleet_bak/test/`：`test_dangerous_detection.py`（223 行）、`test_error_classification.py`（205 行）+ `nodes.csv` / `test.csv` / `test2.csv` / `wsl.csv`。**注**：该目录旧版从未进入版本控制，已从完整备份手工恢复 |
| 完整备份（安全网，全程保留） | `D:\Desktop\Code\Old-Code_Bak\SSHFleet_old`（1252 文件） |
