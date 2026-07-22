# Comet Design Handoff

- Change: remove-old-transfer-code
- Phase: design
- Mode: compact
- Context hash: e37a524eb27005a28dfd01cc02ddea9914924dae913ea7fb12e61ec0e4d2f69c

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/remove-old-transfer-code/proposal.md

- Source: openspec/changes/remove-old-transfer-code/proposal.md
- Lines: 1-31
- SHA256: d5218070b7c57f469601b3631a277cb82089096a0a7b0fca5b53c911e1f03e96

```md
## Why

Python 端存在一套旧的 transfer 上传/下载实现（基于 Fabric + paramiko SFTP），在上传功能迁移到 Go 后已成死代码。下载功能（`-d` 参数）虽仍在使用旧路径，但用户确认不再需要 `-d` 功能。保留这些代码增加维护负担，且 fabric/paramiko 依赖增加了部署复杂度。

## What Changes

- 删除 `src/transfer/` 整个目录（transfer.py、transfer_precheck.py、transfer_check.py、transfer_utils.py）
- 删除 `transfer_router.py` 中的路由逻辑
- 移除 `-d`（下载）参数及相关验证逻辑
- 移除 `-d` 模式的确认展示、日志配置、help 信息
- 移除 `fabric`、`paramiko`、`invoke` 第三方依赖
- 清理 `builder.py` 中旧的 `transfer_command` 参数
- 清理 `sshfleet.py` 中 `-d` 相关的导入和调度逻辑

## Capabilities

### New Capabilities

（无新能力，纯清理工作）

### Modified Capabilities

（无能力需求变更，仅删除代码）

## Impact

- **代码删除**: 约 1700 行 Python 代码（transfer 目录 4 个文件）
- **依赖移除**: fabric、paramiko、invoke 三个第三方包
- **参数变更**: `-d` 参数不再支持（**BREAKING**）
- **功能影响**: 仅影响 `-d` 下载模式用户，上传和命令执行不受影响
- **涉及文件**: sshfleet.py、core.py、check.py、utils.py、gotogo/builder.py、yaml.py

```

## openspec/changes/remove-old-transfer-code/design.md

- Source: openspec/changes/remove-old-transfer-code/design.md
- Lines: 1-54
- SHA256: 46d7a75072dc33b984777322ee27bc6884115e07985961c16d7452f052824092

```md
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

```

## openspec/changes/remove-old-transfer-code/tasks.md

- Source: openspec/changes/remove-old-transfer-code/tasks.md
- Lines: 1-33
- SHA256: 393e3b09751e910a487ae6a075dd6ba4f5512d1177041dcbc583ba9ceae43433

```md
## 1. 移除 -d 参数及入口逻辑

- [ ] 1.1 在 `sshfleet.py` 中移除 `-d` 参数的 import 和 `transfer_router.route_download()` 调用
- [ ] 1.2 在 `src/core.py` 中移除 `-d` 参数定义（argparse add_argument）
- [ ] 1.3 在 `src/core.py` 中移除 `-d` 相关的互斥校验（与 `-c`、`-s`、`-u`、`-z` 的互斥检查）
- [ ] 1.4 在 `src/core.py` 中移除 `-d` 相关的确认展示逻辑（`arguments_confirm` 中的下载模式显示）
- [ ] 1.5 在 `src/check.py` 中移除 `-d` 相关的参数校验（`-p` 对下载模式的特殊校验）
- [ ] 1.6 在 `src/utils.py` 中移除 `-d` 相关的日志目录配置

## 2. 删除 transfer 目录

- [ ] 2.1 删除 `src/transfer/transfer_router.py`
- [ ] 2.2 删除 `src/transfer/transfer.py`
- [ ] 2.3 删除 `src/transfer/transfer_precheck.py`
- [ ] 2.4 删除 `src/transfer/transfer_check.py`
- [ ] 2.5 删除 `src/transfer/transfer_utils.py`
- [ ] 2.6 删除 `src/transfer/__init__.py`（如果存在）
- [ ] 2.7 删除 `src/transfer/` 目录本身

## 3. 清理引用和依赖

- [ ] 3.1 在 `sshfleet.py` 中移除 `transfer_router` 的 import 语句
- [ ] 3.2 在 `src/gotogo/builder.py` 中移除 `build_request()` 的 `transfer_command` 参数
- [ ] 3.3 在 `src/yaml.py` 中移除 `timeout_transfer` 配置项（如果不再需要）
- [ ] 3.4 在 `requirements.txt` 中移除 fabric、paramiko、invoke 依赖
- [ ] 3.5 检查是否有其他文件引用了 transfer 模块，清理残留 import

## 4. 验证

- [ ] 4.1 运行 `python3 sshfleet.py --help` 确认 `-d` 参数已移除
- [ ] 4.2 运行 `python3 sshfleet.py -f nodes.csv -u /tmp/test -p /remote/` 确认上传功能正常
- [ ] 4.3 确认 `import fabric`、`import paramiko`、`import invoke` 不再存在于代码中
- [ ] 4.4 检查无残留的 transfer 相关引用

```

## openspec/changes/remove-old-transfer-code/specs/transfer-cleanup/spec.md

- Source: openspec/changes/remove-old-transfer-code/specs/transfer-cleanup/spec.md
- Lines: 1-41
- SHA256: d2660afe73ce50710ae2644f5288a5927febd52f83810aa1d7c5697c1864e1ad

```md
## REMOVED Requirements

### Requirement: Python Fabric/SFTP upload path
旧的上传路径使用 Python Fabric + paramiko SFTP 库直接执行文件传输。

**Reason**: 上传功能已完全迁移到 Go 二进制（通过 `/api/v1/upload` HTTP 接口），旧代码为死代码。
**Migration**: 无需迁移，上传功能已通过 Go 路径正常工作。

#### Scenario: Upload via old SFTP path
- **WHEN** 用户执行 `-u` 上传命令
- **THEN** 系统 SHALL 通过 Go 二进制的 `/api/v1/upload` 接口执行上传（而非 Python Fabric SFTP）

### Requirement: Python Fabric/SFTP download path
旧的下载路径使用 Python Fabric + paramiko SFTP 库直接执行文件下载。

**Reason**: 用户确认不再需要 `-d` 下载功能。
**Migration**: 无替代方案，功能已移除。

#### Scenario: Download via old SFTP path
- **WHEN** 用户尝试使用 `-d` 参数
- **THEN** 系统 SHALL 报错提示该参数不再支持

### Requirement: -d download parameter
`-d` 参数允许用户指定远程文件/目录路径进行下载。

**Reason**: 用户确认不再需要下载功能。
**Migration**: 无替代方案，功能已移除。

#### Scenario: -d parameter usage
- **WHEN** 用户在命令行中使用 `-d` 参数
- **THEN** 系统 SHALL 显示未知参数错误

### Requirement: fabric/paramiko/invoke dependencies
项目依赖 fabric、paramiko、invoke 三个 Python 包用于 SSH 连接和 SFTP 传输。

**Reason**: 这些依赖仅被旧的 transfer 路径使用，删除后不再需要。
**Migration**: 无需替代，SSH 连接由 Go 端处理。

#### Scenario: Dependency removal
- **WHEN** 旧 transfer 代码被删除
- **THEN** requirements.txt 中 SHALL 不再包含 fabric、paramiko、invoke 依赖

```
