

# sshexec_go

使用 Go 编写的批量 SSH 命令执行工具。

## 项目简介

sshexec_go 是一个高效的批量 SSH 命令执行工具，支持并发执行命令、结果收集、错误分类和日志记录等功能。该工具设计用于在多台远程服务器上批量执行运维任务。

## 主要功能

- **批量执行**：支持在多台服务器上批量执行 SSH 命令
- **并发控制**：可配置的最大并发数，有效控制资源使用
- **结果处理**：自动收集和处理命令执行结果
- **错误分类**：自动识别和分类常见的 SSH 执行错误
- **日志记录**：支持带成功级别标记的日志功能
- **中断处理**：支持优雅处理中断信号
- **JSON 配置**：支持通过 JSON 格式配置任务参数

## 项目结构

```
sshexec_go/
├── main.go                          # 程序入口
├── internal/
│   ├── core/
│   │   ├── batch_executor.go       # 批量执行器
│   │   └── result_consumer.go      # 结果消费者
│   ├── interrupt/
│   │   └── interrupt.go            # 中断处理
│   ├── jsonproc/
│   │   ├── json_parser.go          # JSON 解析器
│   │   └── json_type.go            # JSON 数据类型定义
│   ├── log/
│   │   ├── logger.go               # 日志记录器
│   │   └── sucessLevel.go          # 成功级别日志
│   ├── socket/
│   │   └── client.go               # Socket 客户端
│   └── ssh/
│       ├── ssh_result.go           # SSH 执行结果
│       └── ssh_run.go              # SSH 客户端实现
```

## 核心组件

### BatchExecutor
负责批量执行 SSH 任务，支持可配置的并发数，实时推送执行进度。

### ResultConsumer
收集并处理命令执行结果，支持结果格式化输出到指定目录。

### SSHClient
SSH 连接客户端，支持密钥认证和命令执行，包含超时和取消支持。

### ResultConsumer
处理中断信号，提供可取消的上下文。

## 使用方法

### 配置文件格式

程序支持 JSON 格式的配置文件，主要包括：
- 节点信息（IP、端口、用户名等）
- 执行命令
- 认证配置

### 运行程序

```bash
./sshexec_go -config config.json
```

具体命令行参数请参考程序帮助信息。

## 依赖

- Go 1.16+
- golang.org/x/crypto/ssh
- go.uber.org/zap

## 许可证

请查看项目中的 LICENSE 文件获取详细信息。