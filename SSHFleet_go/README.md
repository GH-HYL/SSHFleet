# SSHExec_go

批量 SSH 命令执行引擎，通过 HTTP API 提供服务，SSE 流式返回结果。

## 特性

- HTTP API + SSE 流式响应
- 多节点并发执行
- 一次性设计：处理一次请求后自动退出
- 防护机制：拒绝第二个请求
- base64 编码输出，防止 JSON 序列化问题
- 错误信息透传，不做分类

## 编译

```bash
# Windows
go build -o batch_ssh.exe .

# Linux
GOOS=linux GOARCH=amd64 go build -o batch_ssh .
```

## 使用

```bash
# 启动服务
./batch_ssh --port 9090 --log-path /var/log/sshexec.log

# 发送请求
curl -N -X POST http://localhost:9090/api/v1/execute \
  -H "Content-Type: application/json" \
  -d '{
    "command": "df -h",
    "options": {"concurrency": 10, "connect_timeout": 10, "exec_timeout": 60},
    "nodes": [
      {"seq": 0, "ip": "10.0.0.1", "port": 22, "user": "root", "password": "xxx"}
    ]
  }'
```

## 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--port` | 监听端口 | 9090 |
| `--log-path` | 日志文件路径 | 空（输出到 stderr） |

## 请求格式

```json
{
  "command": "string",
  "options": {
    "concurrency": "int",
    "connect_timeout": "int",
    "exec_timeout": "int"
  },
  "nodes": [
    {"seq": "int", "ip": "string", "port": "int", "user": "string", "password": "string"}
  ]
}
```

## 响应格式（SSE）

```
data: {"seq":0,"ip":"10.0.0.1","connect_success":true,"exit_code":0,"output":"base64...","error":null}

data: {"type":"done","total":1,"success":1,"failed":0}
```

## 错误码

| 错误码 | 说明 |
|--------|------|
| `INVALID_REQUEST` | JSON 解析失败 |
| `MISSING_FIELD` | 必填字段缺失或 seq 重复 |
| `INTERNAL_ERROR` | 不支持流式响应 |
| `ALREADY_USED` | 服务已被调用 |

## 项目结构

```
internal/
├── httpserver/
│   ├── server.go      # HTTP 路由 + 请求处理
│   └── sse.go         # SSE 写入工具
├── core/
│   └── batch_executor.go  # 并发执行器
├── jsonproc/
│   ├── json_type.go   # 请求结构体定义
│   └── json_parser.go # 请求解析 + 验证
├── ssh/
│   ├── ssh_run.go     # SSH 连接 + 命令执行
│   └── ssh_result.go  # 结果结构体定义
├── interrupt/
│   └── interrupt.go   # 信号中断处理
└── log/
    ├── logger.go      # 日志初始化
    └── sucessLevel.go # 自定义 SUCCESS 级别
```

## 设计原则

- Go 不做数据处理，只返回原始数据
- Go 不主动打印执行结果，通过 SSE 返回
- error 返回原始文本，分类交给调用方
- 一次性工具，处理一次请求后退出
