# SSHFleet 全 Go 重写 · 方向稿

Status: needs-info

> **只记录与旧实现不同的决策，以及重要架构决策。**
> 与旧代码一致的行为不在此记录——重构时以旧代码为准（见 D20）。
> 旧工程位于 `modules/SSHFleet_bak/`，只读参考。

---

## 一、目标

以 Go 重写整个工具，产出**单一可执行文件**：原 Python 编排层与原 Go 执行引擎合并为同一进程内的一条主干流程。

**这是重构，不是从头开发** —— 目标是一个经得起后续演进的好架构，而不是尽快跑通功能。

---

## 二、架构决策（与旧实现根本不同）

| # | 决策 | 与旧实现的差异 |
| --- | --- | --- |
| D1 | 单可执行文件 | 旧：Python 入口进程 + 独立 Go 引擎进程 |
| D2 | **不保留事件流模型** | 旧：`init` / `progress` / `result` / `done` 四类 SSE 消息。新：进度与统计由进程内**聚合器**承载 |
| D3 | 双进程设施全部消亡 | 子进程管理、端口探测、健康检查线程、关闭信号、`X-SSH-Fleet-Key` 认证、`ALREADY_USED` 单次限制、无请求自杀、`/api/v1/*` 全部端点、`SSH_FLEET_KEY` / `SSH_FLEET_PORT` / `SSH_FLEET_LOG_PATH` 环境变量 |
| D4 | 旧术语作废 | "编排层 / 执行引擎 / SSE 消息 / SSE 会话"不再出现 |
| D12 | 骨架：`main.go` 主干十步 + `internal/` 11 个目录 + 工程根 `config/` | 见第五节 |
| D21 | 框架冻结 | 骨架定稿即硬边界，此后所有改动都在框架内调整 |
| D22 | 内聚优先 | 逻辑上属一体的保持整体，不为拆而拆；允许函数调用链加深 |

---

## 三、技术选型（与旧实现不同）

| # | 决策 | 与旧实现的差异 |
| --- | --- | --- |
| D6 | 配置格式改 **TOML** | 旧：YAML |
| D13 | 节点标识**只支持 IPv4 字面量**，严格校验（`netip.ParseAddr`），域名明确报错 | 旧：正则不校验每段范围；**且第一行的域名会被当表头静默丢弃（缺陷）** |
| D14 | 凭据加密新写入改 **AES-256-GCM**（`crypto/aes` + `crypto/cipher`，密钥派生 `crypto/hkdf`），格式版本 `0x02`；旧 `0x01` 保留**只读**解密 | 旧：自建 SHA256 计数器密钥流 XOR + 独立 HMAC。全标准库、零外部依赖，且**旧密文无需迁移** |
| D15 | 主密钥环境变量统一为 **`SSHFLEET_KEY`** | 旧：Py 侧 `SSHFLEET_KEY`、Go 侧 `SSH_FLEET_KEY` 两个名字并存 |
| D24 | 命令行**保持平级选项**，不引子命令；模式互斥**手写校验**、提示文案自撰 | 旧同为平级选项——此处是**刻意维持**：比较过子命令方案，最终选择不动使用习惯（手写互斥的提示比框架原生报错可读） |
| D25 | 危险检测解析层引 `mvdan.cc/sh/v3/syntax` | 旧：368 行手写 shell 词法器。**判定层（平级正则 + 旗标归一化 + 等级排序）仍自己实现** |
| D26 | CLI 用 `spf13/pflag`（**不引 cobra**） | 其余：终端 UI 只引 `lipgloss`、进度条自写；日志 `zap`；SFTP `pkg/sftp`；xlsx `excelize/v2`；SSH `golang.org/x/crypto/ssh` |
| D28 | 配置位置：基准**当前工作目录**，`./config/SSHFleet.conf`（TOML 格式、`.conf` 后缀） | 旧：硬编码 `src/config/SSHFleet.yaml` 相对 cwd |

---

## 四、工作方式

| # | 决策 |
| --- | --- |
| D5 | **功能优先**：有成熟好用的库就用，不为"极简依赖"自造轮子 |
| D8 | 全 Go 重写即**全面择优**，不做一比一复刻 |
| D20 | **不做功能台账**。以旧代码为事实来源，逐个功能三问：① 能不能优化 ② 优化后是否**保持原功能** ③ 保持不了、或确有更优方案 → **ask 用户裁定**。**与旧代码一致的内容不落文档** |
| D23 | 不规划测试策略（属 TDD 实施范畴） |

---

## 五、骨架与硬性约束

依据 `个人开发规范.md`（原文为硬性约束）：

- 入口 `main.go` 置于工程根目录，承载「初始化 → 运行 → 退出」主干全流程
- 其余文件均为主干某一环节的分支实现，由入口统一调用并回收结果
- **禁止**子模块反向驱动主流程（回调控制生命周期、全局状态跨模块流转）
- 工程根必须建 `internal/`：其内只建功能目录、不放散文件，且必须含 `log` 模块
- 工程根另置 `config/`（与 `main.go` 同级）存放模板配置
- 按功能分目录；单文件过大按**功能边界**拆分，不按行数机械切割
- 提交信息中文，格式 `类型(作用域): 简短描述`；`CHANGELOG.md` 只记用户能直接体验到的变化

### 主干十步与承载目录

| # | 主干步骤 | 承载目录 |
| --- | --- | --- |
| 1 | 加载配置 | `internal/config` |
| 2 | 初始化日志 | `internal/log` |
| 3 | 解析命令行 | `internal/cli` |
| 4 | 工具模式分流：keygen / convert-password | `internal/credential` |
| 5 | 参数合规检查 + 危险命令检测 | `internal/dangercheck` |
| 6 | 读取清单 + 字段补全 + 输入记忆 | `internal/nodelist` |
| 7 | 参数确认（交互） | `internal/confirm` |
| 8 | 并发执行 SSH / SFTP | `internal/ssh` + `internal/batch` |
| 9 | 结果聚合 + 错误分类 | `internal/result` |
| 10 | 呈现 / 报告 / 归档 | `internal/output` |
| — | 跨功能共用的**无状态**小工具 | `internal/common` |

**`internal/common` 准入标准**：「被 ≥2 个功能目录调用，且自身不持有状态、不承载生命周期、不做跨模块状态中转」。达不到就放回使用它的目录。

> 反面教材：旧 Go 的 `httpserver.Start()` 是典型的子模块接管主流程（内部起服务、等请求、等关闭信号，`main` 调完即止）。

---

## 六、里程碑（按骨架推进）

| 里程碑 | 内容 |
| --- | --- |
| M1 框架层 | 主干十步就位 + `common` + `config` + `log` + `cli`（平级选项 / 手写互斥 / 自撰提示） |
| M2 数据层 | `credential`（三等级 + 主密钥 + 转换 + 新 AEAD）+ `nodelist` + `confirm` |
| M3 执行层 | `ssh`（连接 / 命令 / SFTP）+ `batch`（并发池 + 进度聚合） |
| M4 判定层 | `dangercheck`（解析层 + 判定层）+ `result`（聚合 + 错误分类） |
| M5 输出层 | `output`（终端 + 报告 + xlsx + 归档） |
| M6 收尾 | 交叉编译打包 + 文档重写 + 配置迁移说明 → 5.0.0 |

---

## 七、迁移影响（对使用者）

| 项 | 影响 |
| --- | --- |
| 配置文件 | YAML → TOML，且位置改为 `./config/SSHFleet.conf`，需重写一份 |
| 规则文件 | 格式转 TOML，**语义与正则一字不动** |
| 凭据文件 | 旧密文照常可读，无需迁移；`--convert-password` 可升级到新格式 |
| 调用方式 | **不变**（平级选项维持原样） |
| 主密钥环境变量 | 统一为 `SSHFLEET_KEY`，已持久化该名的环境无需改动 |
| 版本号 | `CHANGELOG.md` 从 **5.0.0** 起（不兼容重写） |
| 文档 | 旧 `CONTEXT.md` 与 ADR-0001–0013 全部作废 |

---

## 八、参考资料位置

| 用途 | 位置 |
| --- | --- |
| 旧代码本体 | `modules/SSHFleet_bak/modules/` |
| 旧决策记录 | `modules/SSHFleet_bak/docs/adr/`、`CONTEXT.md`、`CHANGELOG.md`、`README.md` |
| 危险检测测量工具与基线 | `modules/SSHFleet_bak/.scratch/danger-rm-structured/`（`measure_recognition.py`、`verify_old_rules.py`、`old_dk_baseline.yaml`、`v1_baseline.yaml`） |
| 危险检测 / 错误分类语料 | `modules/SSHFleet_bak/test/` |
| 完整备份（安全网） | `D:\Desktop\Code\Old-Code_Bak\SSHFleet_old` |
