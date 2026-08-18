# ADR-0004: 移除打包模式（-z）功能

状态：已接受（2026-08-18）

## 背景

工具长期保留一个「打包模式」（`-z`）：把最近一次执行的历史记录目录打包为 ZIP/TAR 压缩包，输出到工作目录或 `paths.logs.zip` 配置指定路径。该功能自引入后使用频率极低，且存在以下问题：

- **独立成一条执行分支**：`main` 中 `if args.z:` 提前退出，与其余四种执行模式（命令/脚本/上传/下载）并列，但实际只是"历史记录管理"附属能力，与"批量 SSH 执行"的主定位不匹配。
- **交互路径特殊**：`-z` 与其他参数互斥（参数检查 + `archive.py` 内二次 `sys.argv` 长度校验双重防御），`--disinteractive` 还要单独禁止与 `-z` 同用，规则面多、维护成本高。
- **有更简单的替代**：查看历史记录直接进入 `historys/` 目录即可，打包场景可手动压缩，功能价值与维护成本不成比例。

## 决策

**移除打包模式（`-z`）全部代码与文档，不再提供该命令行入口。配置加载同步启用严格模式（`extra="forbid"`），任何已移除配置项的残留字段直接报错，不留兼容路径。**

### 1. 删除范围（工作源码 + 根 README）

- `sshfleet.py`（main）：`zip_latest_history` 导入、`if args.z:` 提前退出分支
- `src/output/archive.py`：`zip_latest_history` 函数本体及其专属 import（`re`、`sys`、`color`、`args_normalize_path`、`get_user_confirmation`）；`save_execute_resource_files`（资源备份）保留
- `src/output/__init__.py`：导出列表去掉 `zip_latest_history`
- `src/input/args.py`：`-z` 参数定义、usage、示例
- `src/check/arguments.py`：互斥模式列表去掉 `-z`、`--disinteractive` 与 `-z` 的互斥检查
- `src/config/SSHFleet.yaml` + `src/common/loader.py`：`paths.logs.zip` 配置项与 `Logs.zip` 字段一并删除（不留死配置）
- `tools/Pyinstaller/SSHFleet.spec`：模块注释中的 `-z` 说明
- 根 `README.md`：6 处 `-z` 说明（示例、usage、参数表、FAQ）

### 2. 明确不动的范围

- **`release/` 发布产物快照**（源码版/打包版副本）：按时间戳存留的历史发布，修改无意义；下次发布新版本自然不含 `-z`。
- **Go 侧**：确认无 `-z` 相关代码，不涉及。

## 后果

- **正面**：命令行入口收敛为四种执行模式，参数互斥规则简化；`archive.py` 职责回归"保存执行资源文件"；删掉约 150 行代码与一项死配置。
- **负面**：用户无法再用一条命令打包最新历史记录，需要手动进入 `historys/` 目录处理（README FAQ 已同步说明）。
- **彻底移除，不做任何兼容保留**：`-z` 命令行参数不再被识别（提示 `unrecognized arguments`）；配置加载启用严格模式（Pydantic `extra="forbid"`），配置中若残留 `paths.logs.zip` 等未知字段，加载时直接报错并指出字段位置，逼使用户删除残留，不再静默忽略。就当该功能从未存在。
