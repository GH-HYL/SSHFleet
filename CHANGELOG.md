# Changelog

所有重要变更记录在此。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。
只记录 ./modules/** 工程代码相关的记录，其他辅助文件变更不记录

---

## [0.7.1] - 2026-07-31

### Fixed
- 修复 `account.password` 为相对路径时未与 `account.password_dir` 拼接的问题（预检查和实际使用时均找不到密码文件），现在相对路径会拼接为 `password_dir/password`，缺失时错误信息显示拼接后的完整路径
- 修复 `password_dir` 含 `~` 时拼接结果未展开的问题（拼接后的路径带字面 `~`，`os.path.exists` 无法识别），现在 `~` 先展开再拼接
- 修复 `account.password` 为相对路径但 `account.password_dir` 未配置时无明确提示的问题，现在加载配置阶段直接报错说明
- 修复未指定 `-T` 时连接超时为 `None` 导致 `total_timeout` 计算崩溃的问题（`go_to_go` 中 `(args.T + args.t) * 1.5`），未指定 `-T` 时使用配置的 `timeout_connect`
- 修复 `-t` / `-T` / `-n` 显式传参被误判为格式错误的问题（argparse 默认返回字符串，现统一转为 `int` 再校验）
- 修复 `-m` 未指定时 `args.m` 为 `None` 的问题（不传 `-m` 时使用配置的 `execution.mode`，修复配置 `sudo` 却未提权）
- 修复 `-n` 未指定时 `args.n` 为 `None` 的问题（未传 `-n` 时默认 0，确认阶段替换为节点数，修复并发数最终为 `None` 传给 Go 端）

---

## [0.7.0] - 2026-07-24

### Added
- 新增 `POST /api/v1/download` 端点，支持通过 SSH/SFTP 批量下载远程文件/目录到本地
- 新增 `-d` 参数（下载模式），与 `-c / -s / -u / -z` 互斥
- 新增 `DownloadRequest` 请求体类型（`remote_path` + `local_path` + `options` + `nodes`）
- 新增 `ParseDownloadRequest()` 请求解析函数，含完整校验（空路径、空节点、seq 查重、默认值填充）
- 新增 `progressReader` 进度回调读取器，包装 SFTP 读取器，每 500ms 节流上报进度
- 新增 `BatchDownloadExecutor` 批量下载执行器，worker 模式 + channel 通信
- 新增 `DownloadFiles()` 核心下载逻辑：SSH 预检查 + SFTP 流式下载 + 1MB buffer
- 新增 `sftpDownloadFile()` 单文件下载，支持符号链接跳过和重试机制
- 新增 `ProgressMsg.DownloadedBytes` 字段，JSON 标签 `downloaded_bytes`
- Python 端 `build_download_request()` 请求构建函数
- Python 端下载模式进度条显示（复用上传进度条框架，标签改为"下载进度"）
- Python 端 `-d` 参数路径规范化和备注自动生成（取 basename）
- Python 端下载模式校验：`-d` 必须是绝对路径、`-p` 必须是已存在的目录
- Python 端下载模式参数确认表格显示
- 测试：`progressReader` 6 项单元测试（Read、AccumulateBytes、Throttle、TimeBetweenCallbacks、EmptyData、FieldsPopulated）
- 测试：`ParseDownloadRequest` 9 项单元测试（Valid、EmptyRemotePath、EmptyLocalPath、EmptyNodes、ZeroExecTimeout、DuplicateSeq、DefaultValues、InvalidJSON、JSON roundtrip）

### Changed
- `ProgressMsg` 新增 `DownloadedBytes` 字段（`omitempty`，与 `UploadedBytes` 并存）
- `ssh_result.go` 复用 `UploadResult` 作为 `DownloadResult`（设计决策 #27，所有字段语义兼容）
- `classifier.py` 下载成功归入"传输成功"分类（与上传共用）
- `statistics.py` 下载成功归入"传输成功"统计分类
- `go_to_go.py` 进度显示分支扩展：`args.u or args.d` 共用双进度条框架
- `sshfleet.py` 主流程条件新增 `args.d`
- 更新 `args.py` `-h` 帮助信息：新增下载模式示例

### Architecture
- 下载作为新的执行模式（`-d`）融入现有 `-c / -s / -u` 体系，不搞独立分支
- 下载是上传的镜像：架构、协议、进度模型与上传保持一致，降低理解和维护成本
- 传输行为本身不支持 sudo（纯 SFTP），预处理/预检查使用 SSH 命令支持 sudo 提权
- 下载模式不发送 init 消息，不显示全局字节进度条

---

## [0.6.0] - 2026-07-24

### Added
- 新增密码存储改造：CSV 密码列从明文改为 Base64 密码文件路径引用
- 新增 `password_dir` 配置字段，支持相对路径拼接
- 新增 `resolve_password_path()` 函数，支持 `~`、绝对路径、相对路径解析
- 新增 `validate_csv_passwords()` 预检查机制，处理节点前统一验证密码文件
- 新增 `-f` 参数支持内联CSV文本传入（如 `-f "192.168.1.10,22,root,密码路径"`）
- 新增 `--disinteractive` 模式自动跳过所有 `yorn=True` 的确认提示
- 新增并发数提示信息，增强用户确认警告
- 新增 `output.txt` 上传模式写入支持（命令模式和上传模式共用输出文件）
- 上传模式节点进度条新增 Succ/Fail 实时统计（与命令模式一致）

### Changed
- CSV 密码列语义从「明文密码」变为「密码文件路径」
- 重构密码验证逻辑，复用 `args.validate_password_file()` 函数
- 移除 `read_nodes_infos()` 中重复的密码验证调用
- `get_user_confirmation()` 新增 `disinteractive` 参数，支持非交互模式
- `-f` 参数文件不存在时增加交互确认，可选择作为内联CSV文本传入
- `validate_csv_passwords()` 内联校验替代 `validate_password_file()`，错误信息更具体
- `-h` 帮助信息三列对齐，支持中文字符显示宽度计算
- 重命名 Go 工程目录 `SSHFleet_go` → `SSHFleet_Go`，符合个人开发规范
- 修复日志模块文件名拼写 `sucessLevel.go` → `successLevel.go`
- 拆分 `ssh_run.go`（642行）为三个职责清晰的文件：`ssh_types.go`（99行）、`ssh_run.go`（174行）、`ssh_upload.go`（392行）
- `output.xlsx` 改为从结构化数据直接生成，不再从 txt 正则解析（更可靠）
- `format_output_to_xlsx()` 函数签名更新：增加 `final_results` 和 `args` 参数
- 上传并发阈值检查从"提醒模式"改为"默认约束模式"：未指定 `-n` 时自动约束并提示确认，显式指定 `-n` 时仅提示不强制
- 更新 `-h` 帮助信息：修正 `-n` 描述（上传模式也支持）、精简各参数描述、新增上传并发说明

### Fixed
- 修复 `output.txt` 命令模式文件创建但内容为空的问题（主结果分支缺少写入逻辑）
- 修复 `output.txt` 上传模式未创建文件的问题（`if not args.u` 条件门控）
- 修复连接失败时 `exit_code` 默认为 0 导致成功计数错误的问题（Go 端 `ExitCode` 改为 `*int`，连接失败时为 `nil`）
- 修复读取密码文件逻辑，确保正确解码 base64 密码
- 修复 SSH 连接耗时返回与错误处理
- 修复多文件上传进度条累计、回退及速度为负的问题
- 修复 result 消息发送实际上传字节数而非总字节数
- 修复 `--nobash` 模式仍会预处理命令的问题（现在跳过所有预处理：边界符号移除、环境变量、sudo、bash -c）

---

## [0.5.0] - 2026-07-18

### Added
- 新增上传进度跟踪字段，优化上传体验
- 新增健康检查端点
- 新增独立 context 处理执行和上传请求，任务不受客户端断开影响

### Changed
- 使用带锁的写入器确保 SSH 命令输出的并发安全性
- 使用 mutex 保护 SSE 写入，防止并发写入导致分块编码错误
- 使用 SSH + sudo 创建临时目录，解决 SFTP 无 sudo 权限导致上传失败
- sudo 创建临时目录后设置 777 权限，确保 SFTP 有写入权限
- 将日志级别从 Info 调整为 Debug，减少上传过程中的日志冗余

### Fixed
- 修复上传进度条不显示的问题
- 修复密码文件延迟验证至实际使用时

---

## [0.4.0] - 2026-07-15

### Added
- 新增环境变量支持和进程认证 key 机制
- 新增 API 文档，详细说明接口规范和请求格式
- 新增 SSH 连接统计日志、请求拒绝日志、请求体详细信息日志、启动参数日志

### Changed
- 使用 zap 结构化字段替代 fmt.Sprintf，统一日志格式
- 删除请求体预览以避免密码泄露
- 删除未使用的 truncateString 函数

---

## [0.3.0] - 2026-07-10

### Added
- 新增文件上传功能，支持批量上传执行器
- 新增上传并发阈值配置
- 新增上传结果结构，优化日志输出
- 新增无请求自动退出机制

### Changed
- 重构传输逻辑，新增传输路由模块
- 重构上传功能，统一使用 Go API 处理上传请求

---

## [0.2.0] - 2026-07-05

### Added
- 新增 SSH 任务输出记录功能
- 新增 HTTP 服务器关闭信号支持
- 新增日志路径有效性检查

### Changed
- 重构配置文件结构，迁移至 YAML 格式
- 重构 HTTP 服务器启动逻辑
- 重命名项目为 SSHFleet，更新所有路径引用

### Fixed
- 修复 exec_timeout 验证逻辑，确保其必须大于 0
- 修复错误提示中的函数名称拼写
- 修复环境变量设置为 C.UTF-8 以确保 Linux 兼容性

---

## [0.1.0] - 2026-07-01

### Added
- SSHFleet 工具初始版本
- 支持 SSH 连接管理与命令执行
- 支持 Excel 文件格式化输出
- 支持参数解析与帮助信息
