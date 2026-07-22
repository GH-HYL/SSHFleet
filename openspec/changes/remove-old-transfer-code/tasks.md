## 1. 移除 -d 参数及入口逻辑

- [x] 1.1 在 `sshfleet.py` 中移除 `-d` 参数的 import 和 `transfer_router.route_download()` 调用
- [x] 1.2 在 `src/core.py` 中移除 `-d` 参数定义（argparse add_argument）
- [x] 1.3 在 `src/core.py` 中移除 `-d` 相关的互斥校验（与 `-c`、`-s`、`-u`、`-z` 的互斥检查）
- [x] 1.4 在 `src/core.py` 中移除 `-d` 相关的确认展示逻辑（`arguments_confirm` 中的下载模式显示）
- [x] 1.5 在 `src/check.py` 中移除 `-d` 相关的参数校验（`-p` 对下载模式的特殊校验）
- [x] 1.6 在 `src/utils.py` 中移除 `-d` 相关的日志目录配置

## 2. 删除 transfer 目录

- [x] 2.1 删除 `src/transfer/transfer_router.py`
- [x] 2.2 删除 `src/transfer/transfer.py`
- [x] 2.3 删除 `src/transfer/transfer_precheck.py`
- [x] 2.4 删除 `src/transfer/transfer_check.py`
- [x] 2.5 删除 `src/transfer/transfer_utils.py`
- [x] 2.6 删除 `src/transfer/__init__.py`（如果存在）
- [x] 2.7 删除 `src/transfer/` 目录本身

## 3. 清理引用和依赖

- [x] 3.1 在 `sshfleet.py` 中移除 `transfer_router` 的 import 语句
- [x] 3.2 在 `src/gotogo/builder.py` 中移除 `build_request()` 的 `transfer_command` 参数（保留：command 模式仍使用）
- [x] 3.3 在 `src/yaml.py` 中移除 `timeout_transfer` 配置项（保留：上传模式仍使用）
- [x] 3.4 在 `requirements.txt` 中移除 fabric、paramiko、invoke 依赖（无 requirements.txt 文件）
- [x] 3.5 检查是否有其他文件引用了 transfer 模块，清理残留 import

## 4. 验证

- [x] 4.1 运行 `python3 sshfleet.py --help` 确认 `-d` 参数已移除
- [x] 4.2 运行 `python3 sshfleet.py -f nodes.csv -u /tmp/test -p /remote/` 确认上传功能正常
- [x] 4.3 确认 `import fabric`、`import paramiko`、`import invoke` 不再存在于代码中
- [x] 4.4 检查无残留的 transfer 相关引用
