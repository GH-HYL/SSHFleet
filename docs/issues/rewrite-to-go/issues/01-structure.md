# M1 · 结构面问题清单

Type: grilling
Status: needs-info

覆盖 `plan-coverage.md` 的 **F2 结构面**：`internal/common` 边界、错误出口约定、`main.go` 主干形态、`internal/log` 形态。

---

## 一、已查明的事实

### 1.1 旧 `src/common/` 逐个体检归属

| 旧文件 | 内容 | 真实归属 |
| --- | --- | --- |
| `constants.py` | 成功分类名（执行成功 / 传输成功 / 部分成功）+ 8 个 ANSI 颜色常量 | 分类名 → `internal/result`；颜色 → 被 `output` 与 `dangercheck` 共用，见 S5 |
| `error_handler.py` | `print_error_information_and_exit(func, msg, isexit=True)` —— 打印到 stderr 并 **`sys.exit(1)`**；另有一个函数级异常装饰器 | 见 S2 |
| `format_utils.py` | 结果呈现公共函数（模式名 / 状态行 / IP 排序） | `internal/output` |
| `loader.py` | 配置加载 + `resolve_secret_path` 路径梯子 | `internal/config` |
| `text_utils.py` | `clean_for_excel`（xlsx 清洗）/ `display_width`（终端列宽）/ `format_size`（大小格式化）/ `args_normalize_path`（CLI 路径规范化） | 前两个 → `output`；`format_size` → `output` / `batch`；最后一个 → `cli` |

**结论：旧 `src/common` 里真正"跨 ≥2 个功能、无状态、无归属"的东西极少。**
若照搬成 `internal/common`，这个目录会变成杂物间——正好撞上 D22 立的红线。

### 1.2 旧代码里最典型的"反向驱动主流程"

`print_error_information_and_exit` 在 **20+ 处深层模块**里被直接调用，内部 `sys.exit(1)`。
按 `个人开发规范.md` §二「MUST NOT 在子模块反向驱动主流程」，这是必须重设计的一处——
**决定"程序以什么退出码结束"的权力，只应属于入口。**

---

## 二、问题

### ❓ S1 — `internal/common` 放什么

- (a) 只放**真正跨 ≥2 个功能目录**的无状态纯函数。按 1.1 的体检，实际只有：`display_width`（cli 的 help 对齐 + output 的表格）、`format_size`（output + batch 进度）、颜色常量、错误提示文本的构造函数
- (b) 不复建 `common`，每个函数就近放到使用它的目录；重复的少数几个各自实现
- (c) 保留较宽的 `common`（照搬旧 `src/common` 的内容）

➡️ 推荐 **(a)**。`common` 的准入标准写成一句话：**"被两个以上功能目录调用，且自己不持有状态"**；达不到就放回使用它的目录。(b) 的问题是 `display_width` 这类"全工具只能有一份实现"的东西（旧版三份并存导致过表格错位）会重新分叉。

### ❓ S2 — 错误出口约定

旧模式是"深层模块直接打印 + 退出"，出口有 20+ 个。规范要求入口独占主干。

- (a) **全链路返回 `error`**，`main.go` 是唯一决定退出的地方；错误信息随 error 一路上抛，主干的收尾步骤统一打印
- (b) 深层模块 `panic`，`main` 用 `recover` 兜住
- (c) 保留"就地退出"，但把出口收敛成一个函数

➡️ 推荐 **(a)**。(b) 会把控制流藏起来，且 Go 社区对非致命错误用 panic 是有共识的反对；(c) 治标不治本。

**但有一个必须保留的区分**：旧代码里的"错误"分两种——
- **致命**：报错后 `exit(1)`（如参数非法、配置文件缺失）
- **警告后继续**：只打印不退出（如 `latest_history` 同名文件已存在则跳过、脚本换行符自动转换提示）

→ 新版要保持这个区分：警告用日志（非 error 级）继续走，致命才终止。**不能一刀切成"全部返回 error 就终止"。**

### ❓ S3 — `main.go` 主干的具体形态

主干十步已知（见 `spec.md` 第五节）。要定的是**写法**：

- (a) 十步各抽成一个函数（`loadConfig()` / `initLog()` / `parseArgs()` …），`main()` 里顺序调用；每步返回 `error`，遇到即走统一收尾
- (b) 十步直接内联在 `main()` 里

➡️ 推荐 **(a)**。`main()` 保持"能一眼看完主干"的长度，符合规范里"入口承载主干全流程"的意图；内联会把主干淹在细节里。

### ❓ S4 — `internal/log` 是一个日志文件还是两个

事实：旧架构有两个日志落盘——工具日志 `SSHFleetTools.log`（Python 侧流程）与执行日志 `SSHFleet_Go.log`（Go 引擎侧）。单进程后只剩一个进程，但**两种用途仍在**：

- 工具日志：初始化、参数、路径、阶段分割线、异常
- 执行日志：每台节点的连接/执行明细（旧 Go 端写入，随 `--disinteractive` 影响不大）

- (a) 合并为**一个**日志文件（配置里 `paths.logs.tool` / `paths.logs.exec` 两字段合一，已在 `spec.md` 附录 A 草案中体现）
- (b) 仍写**两个**文件，只是都由同一进程写

➡️ 推荐 **(b)**。理由：`SSHFleet_Go.log` 里是逐节点执行明细，量大且性质不同；合并后工具日志会被淹没，排查时要在一堆节点输出里翻流程记录。反过来，两个文件不影响"单进程"这个架构结论。

### ❓ S5 — 颜色常量归哪

颜色被 `output`（终端呈现）和 `dangercheck`（危险提示着色）共用。

- (a) 放 `internal/common`（跨两个目录，符合 S1 的准入标准）
- (b) 放 `internal/output`，让 `dangercheck` 依赖 `output`
- (c) 各自定义一份

➡️ 推荐 **(a)**。符合 S1 标准，且避免 `dangercheck` 反向依赖呈现层。

---

## Comments

### 2026-09-11 分流

按用户既定基调「实现层不设偏好，择优即可」（D20 / D5）与「用户可见行为变更必须裁定」（D8），本文件问题分流如下：

**归为实现层——按推荐推进，用户不反对即定案：**

| 问题 | 定案 |
| --- | --- |
| S1 `common` 边界 | 选项 (a)：准入标准 =「被 ≥2 个功能目录调用且自身不持状态」 |
| S2 错误出口 | 选项 (a)：全链路返回 `error`，`main.go` 独占退出权。**保留"致命 / 警告后继续"的区分**——警告只记日志不终止 |
| S3 主干写法 | 选项 (a)：十步各抽函数，`main()` 顺序调用，遇 `error` 走统一收尾 |
| S5 颜色归处 | 选项 (a)：放 `internal/common`（满足 S1 准入标准，且避免 `dangercheck` 反向依赖呈现层） |

**用户可见，仍需用户裁定：**

- **S4 日志一个文件还是两个** —— 决定配置字段数量与落盘文件数。推荐 (b) 仍写两个（工具日志 + 执行日志），理由：执行明细量大且性质不同，合并会淹没流程记录。
