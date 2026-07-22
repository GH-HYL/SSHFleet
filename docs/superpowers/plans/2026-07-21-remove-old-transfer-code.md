---
change: remove-old-transfer-code
design-doc: docs/superpowers/specs/2026-07-21-remove-old-transfer-code-design.md
base-ref: 35ceeb3ffc9d29a476b1f55fd538cf8518879291
---

# Implementation Plan: 移除旧 Transfer 代码

## 执行策略

直接在当前分支执行，不创建 worktree。按 tasks.md 顺序执行，每个任务完成后验证。

## Task 1: 移除 -d 参数及入口逻辑

修改 `sshfleet.py`、`src/core.py`、`src/check.py`、`src/utils.py`，移除所有 `-d` 相关代码。

## Task 2: 删除 transfer 目录

删除 `src/transfer/` 整个目录（7 个文件）。

## Task 3: 清理引用和依赖

清理 `builder.py`、`requirements.txt` 中的残留引用。

## Task 4: 验证

运行验证命令确认功能正常。
