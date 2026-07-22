---
comet_change: remove-old-transfer-code
role: technical-design
canonical_spec: openspec
---

# Design: 移除旧 Transfer 代码

## 背景

SSHFleet Python 端有两条文件传输路径：
- 旧路径：`src/transfer/` 目录，基于 Fabric + paramiko SFTP
- 新路径：`src/gotogo/` 目录，通过 Go 二进制 HTTP/SSE 接口

上传功能已完全迁移到 Go。下载功能（`-d`）虽仍使用旧路径，但用户确认不再需要。

## 删除清单

### 直接删除（7 个文件）

| 文件 | 行数 | 说明 |
|------|------|------|
| `src/transfer/__init__.py` | ~1 | 包初始化 |
| `src/transfer/transfer_router.py` | ~74 | 路由层（上传已注释，下载将删除） |
| `src/transfer/transfer.py` | ~1005 | SFTP 上传/下载引擎 |
| `src/transfer/transfer_precheck.py` | ~239 | 文本文件检测（已死代码） |
| `src/transfer/transfer_check.py` | ~203 | 预检（磁盘空间、权限） |
| `src/transfer/transfer_utils.py` | ~267 | 工具函数（Fabric 连接、tar 压缩） |
| `src/transfer/` 目录 | - | 整个目录 |

### 需修改的文件

| 文件 | 修改内容 |
|------|---------|
| `sshfleet.py` | 移除 `-d` 调度逻辑、transfer_router import |
| `src/core.py` | 移除 `-d` argparse 定义、互斥校验、确认展示 |
| `src/check.py` | 移除 `-d` 参数校验、`-p` 下载模式特殊校验 |
| `src/utils.py` | 移除 `-d` 日志目录配置 |
| `src/gotogo/builder.py` | 移除 `build_request()` 的 `transfer_command` 参数 |
| `requirements.txt` | 移除 fabric、paramiko、invoke |

## 执行顺序

1. **先修改入口文件**（sshfleet.py、core.py、check.py）— 移除 `-d` 参数和调度
2. **再删除 transfer 目录** — 确保无引用后删除
3. **最后清理依赖** — requirements.txt、builder.py

## 验证策略

1. `python3 sshfleet.py --help` — 确认 `-d` 已移除
2. `python3 sshfleet.py -f nodes.csv -u /tmp/test -p /remote/` — 确认上传功能正常
3. `grep -r "import fabric\|import paramiko\|import invoke" modules/SSHFleet_py/` — 确认无残留引用
4. `grep -r "transfer" modules/SSHFleet_py/src/ --include="*.py"` — 确认无残留 transfer 引用

## 风险

- **Breaking Change**: `-d` 参数移除，用户需知悉
- **误删共享代码**: transfer_check.py 和 transfer_utils.py 中部分函数被 download 路径使用，但 download 一并删除，安全
- **requirements.txt 遗漏**: 需确认 fabric/paramiko/invoke 已从 requirements.txt 移除
