# ADR-0003: 退出码语义统一——只属于命令执行

状态：已接受（2026-08-18）

## 背景

线上日志出现 3 个节点下载 `/var/log/messages-20260816` 失败，报错 `打开远程文件失败: permission denied`，但 Python 侧分类为 `执行失败(退出码1)` 而非"权限不足"。

根因：Go 侧 `ssh_download.go` / `ssh_upload.go` 在传输阶段失败时（`failedFiles > 0`）自行构造 `ExitCode = 1`，而 Python 分类器 `classifier.py` 的规则是"有退出码即按退出码分类，跳过关键词匹配"。传输失败被伪退出码遮蔽，error 中的 `permission denied` 从未参与匹配。

更深层的问题：**退出码语义被两处混用**——execute 模式它是真实命令退出码（权威），upload/download 模式它却成了 Go 自造的传输状态码（信息量极低）。这违背了"退出码是命令执行的产物"这一基本语义。

## 决策

### 1. exit_code 语义统一为"命令执行的产物"

- **execute 模式**：命令真实退出码，现状不变（`ssh_run.go` 已正确提取 `*ssh.ExitError.ExitStatus()`）。
- **upload/download 预处理命令失败**：用 `errors.As` 提取真实非 0 退出码填入 `ExitCode`；非 `*ssh.ExitError`（超时/中断等）保持 `nil`，仅设 `Error`。
  - download：`test -e`（路径存在性检查）、`find`（文件列表获取）在顶层预处理，失败即 return，提取退出码。
  - upload：顶层预处理（SFTP Stat 目标路径、本地文件预检）是非命令错误保持 `nil`；`sudo mkdir` / `sudo mv` 位于传输阶段内部的 `sftpUploadWithSudo`，属逐文件传输流程的一部分，失败按传输失败语义处理（`nil` + error 报文关键词），不提取退出码。
- **传输阶段（SFTP 读写）失败**：**不再设置 `ExitCode`（保持 `nil`）**——传输不是命令执行，没有退出码，失败信息通过 `Error` 报文 + `failed_files` 计数表达。
- **全流程成功**：`ExitCode = 0`。

由此形成三态语义：

| exit_code | connect_success | 含义 | Python 分类 |
| --------- | --------------- | ---- | ----------- |
| `0` | true | 全流程成功 | 传输成功 / 执行成功 |
| 非 `0` | true | 有命令失败（execute=命令本身，transport=预处理） | 退出码权威：`执行失败(退出码N)`，不匹配关键词 |
| `nil` | true | 传输阶段失败（SFTP 读写） | 关键词匹配 error，未命中 → 返回 error 原文作为分类（保留具体失败内容） |
| `nil` | false | 连接失败 | 关键词匹配 error（身份验证失败等），未命中 → 返回 error 原文作为分类 |

### 2. 分类职责全部在 Python

Go 只传原始数据（`exit_code` / `error` / `success_files` / `failed_files`），不做任何分类、不构造状态码。`error_keywords.yaml` 是主要的分类知识库；关键词未命中时返回 error 报文的原文作为分类（而非笼统的"错误未分类"），保证失败节点始终有具体可读的分类内容，后续可据原文补充关键词配置。

### 3. classifier.py 主逻辑不动

现有逻辑（有退出码→权威；nil→关键词匹配）在该语义下天然正确，无需修改。部分成功分类暂不引入（保持 成功/失败 两态，明细看 output 逐文件行）。

## 后果

- **正面**：退出码语义跨模式统一；本次 bug 场景（SFTP Open 报 permission denied → `exit_code=nil` → 关键词命中"权限不足"）被修复；`error_keywords.yaml` 成为分类演进的入口，未命中时 error 原文兜底保证分类始终有具体内容。
- **负面**：预处理命令失败时不再显示具体失败原因分类（如"远程路径不存在"），统一显示 `执行失败(退出码N)`，具体原因需查看 error/output 明细；未命中的 error 原文较长时分类标签会显得冗余（但信息完整）。
- **改动面**：Go 侧两个文件（`ssh_upload.go` / `ssh_download.go`）+ 配置（`error_keywords.yaml` 已加"权限不足"分类）。
