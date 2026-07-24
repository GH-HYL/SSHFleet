# SSHFleet_Go

批量 SSH 任务执行引擎，通过 HTTP API 提供服务，SSE 流式返回结果。

## 特性

- HTTP API + SSE 流式响应
- 支持命令执行和文件上传两种模式
- 多节点并发执行
- 一次性设计：处理一次请求后自动退出

## 编译

```bash
# Windows
go build -o SSHFleet_Go.exe .

# Linux
GOOS=linux GOARCH=amd64 go build -o SSHFleet .
```

## 使用

```bash
# 启动服务（通过环境变量配置）
SSH_FLEET_KEY=your-secret-key SSH_FLEET_PORT=9090 SSH_FLEET_LOG_PATH=/var/log/sshtask ./SSHFleet_Go

# 执行命令
curl -N -X POST http://localhost:9090/api/v1/execute \
  -H "Content-Type: application/json" \
  -H "X-SSH-Fleet-Key: your-secret-key" \
  -d '{"command":"df -h","options":{"concurrency":10,"connect_timeout":10,"exec_timeout":60},"nodes":[{"seq":0,"ip":"10.0.0.1","port":22,"user":"root","password":"xxx"}]}'

# 上传文件
curl -N -X POST http://localhost:9090/api/v1/upload \
  -H "Content-Type: application/json" \
  -H "X-SSH-Fleet-Key: your-secret-key" \
  -d '{"file_path":"/home/user/config.yaml","remote_path":"/etc/app/","options":{"concurrency":10,"connect_timeout":10,"exec_timeout":300,"sudo":false},"nodes":[{"seq":0,"ip":"10.0.0.1","port":22,"user":"root","password":"xxx"}]}'

# 关闭服务
curl -X POST http://localhost:9090/api/v1/shutdown \
  -H "X-SSH-Fleet-Key: your-secret-key"
```

## API 文档

API 接口规范见 `API.md`。

## 环境变量

| 环境变量 | 说明 | 必填 |
|---------|------|------|
| `SSH_FLEET_KEY` | 进程认证 key，用于验证 HTTP 请求 | 是 |
| `SSH_FLEET_PORT` | 监听端口 | 是 |
| `SSH_FLEET_LOG_PATH` | 日志目录路径（空字符串输出到 stderr） | 否 |
