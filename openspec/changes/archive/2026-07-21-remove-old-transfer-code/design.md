## Context

SSHFleet Python 端有两条上传/下载路径：
- **旧路径**: `src/transfer/` 目录，基于 Fabric + paramiko SFTP，约 1700 行代码
- **新路径**: `src/gotogo/` 目录，通过 Go 二进制的 HTTP/SSE 接口执行

上传功能已完全迁移到 Go（`transfer_router.route_upload()` 中旧代码已注释），但下载功能（`-d`）仍使用旧路径。用户确认不再需要下载功能。

## Goals / Non-Goals

**Goals:**
- 删除整个 `src/transfer/` 目录（4 个文件，约 1700 行）
- 移除 `-d` 参数及相关代码路径
- 移除 fabric、paramiko、invoke 依赖
- 清理所有对已删除代码的引用

**Non-Goals:**
- 不迁移下载功能到 Go（直接移除）
- 不修改 Go 端代码
- 不影响上传和命令执行功能

## Decisions

### 1. 整个 transfer 目录删除（而非逐步废弃）

**选择**: 一次性删除 `src/transfer/` 目录全部 4 个文件

**理由**: 旧代码已无活跃调用方，逐步废弃没有意义。一次性删除更干净，避免遗留碎片。

**替代方案**: 标记 deprecated 后逐步删除 → 拒绝，因为没有后续迁移计划

### 2. 移除 -d 参数（而非保留为空）

**选择**: 从 argparse 中移除 `-d` 参数定义

**理由**: 用户确认不需要下载功能。保留空参数会误导用户。

### 3. 保留 transfer_router.py 的 import 结构

**选择**: 删除 `transfer_router.py` 文件，将路由逻辑简化为直接调用 `go_to_go()`

**理由**: router 层已无分支逻辑（旧代码已注释），直接在 sshfleet.py 中调用更清晰。

### 4. 清理 builder.py 的 transfer_command 参数

**选择**: 从 `build_request()` 中移除 `transfer_command` 参数

**理由**: 该参数仅用于旧的文本文件上传路径（已注释），无其他调用方。

## Risks / Trade-offs

- **[风险] 误删共享函数** → transfer_check.py 和 transfer_utils.py 中部分函数被 download 路径使用，但 download 一并删除，所以安全
- **[风险] requirements.txt 遗漏依赖** → 删除 fabric/paramiko/invoke 后需同步更新 requirements.txt
- **[权衡] -d 参数移除是 Breaking Change** → 用户已确认接受
