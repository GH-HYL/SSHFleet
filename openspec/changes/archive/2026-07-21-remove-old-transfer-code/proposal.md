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
