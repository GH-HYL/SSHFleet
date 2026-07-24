# Changelog

所有重要变更记录在此。格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

---

## [0.6.0] - 2026-07-24

### Added
- 新增密码存储改造：CSV 密码列从明文改为 Base64 密码文件路径引用
- 新增 `password_dir` 配置字段，支持相对路径拼接
- 新增 `resolve_password_path()` 函数，支持 `~`、绝对路径、相对路径解析
- 新增 `validate_csv_passwords()` 预检查机制，处理节点前统一验证密码文件
- 新增并发数提示信息，增强用户确认警告
- 新增 `output.txt` 上传模式写入支持（命令模式和上传模式共用输出文件）
- 上传模式节点进度条新增 Succ/Fail 实时统计（与命令模式一致）
- 新增 `CONTEXT.md` 领域术语表和数据流图
- 新增 `docs/adr/0001-unified-output-and-structured-xlsx.md` 决策记录
- 新增 `docs/adr/0002-upload-concurrency-enforcement.md` 决策记录
- 新增测试脚本 `test/test_password_path.py`

### Changed
- CSV 密码列语义从「明文密码」变为「密码文件路径」
- 重构密码验证逻辑，复用 `args.validate_password_file()` 函数
- 移除 `read_nodes_infos()` 中重复的密码验证调用
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
