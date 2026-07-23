# ADR 0001: 统一 output.txt 写入 + 结构化 xlsx 生成

**状态**: 已采纳  
**日期**: 2026-07-23

## 背景

SSHFleet 有两种执行模式：命令模式（`-c`/`-s`）和上传模式（`-u`）。output.txt 和 output.xlsx 的生成存在两个问题：

1. **output.txt**：命令模式文件为空（主分支不写入），上传模式文件不创建（`if not args.u` 门控）
2. **output.xlsx**：从 txt 正则解析（`re.search(r"【IP】")`），脆弱且不可靠

## 决策

### 1. 统一 output.txt 写入

将 output.txt 的创建和写入提升为两种模式共用的公共逻辑：

- 文件创建：无条件创建（移除 `if not args.u`）
- 结果写入：在 `msg_type == "result"` 主分支中，格式化后立即 `write + flush`
- 格式复用：`_format_result()` 已通过 `args.u` 自动适配两种模式

### 2. 结构化 xlsx 生成

`format_output_to_xlsx()` 改为从 `final_results` 结构化数据直接生成，不再解析 txt：

- 函数签名：增加 `final_results` 和 `args` 参数
- 3 列格式保持不变：IP地址、事件类型、内容详情
- output 内容按行拆分，每行在 xlsx 中独立一行
- 每条结果后加分隔行，便于视觉区分

## 影响

| 文件 | 改动 |
|------|------|
| `go_to_go.py` | 3 处：文件创建无条件化、主分支写入、else 分支写入 |
| `xlsx.py` | `format_output_to_xlsx()` 重写：接受结构化数据，移除正则解析 |
| `sshfleet.py` | 调用方更新：传递 `final_results` 和 `args` |
| `output.xlsx` | 不再依赖 output.txt，数据来源变为结构化结果 |
| `results.xlsx` | 不受影响，保持原有逻辑 |
