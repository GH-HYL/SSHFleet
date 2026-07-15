# SSHFleet Go API 接口规范

版本：4.0

---

## 一、概述

SSHFleet Go 是一个一次性批量 SSH 任务引擎，支持命令执行和文件上传两种模式，通过 HTTP API 接收请求。

- **启动方式**：通过环境变量配置端口、日志路径和认证key
- **认证机制**：所有HTTP请求必须携带`X-SSH-Fleet-Key`请求头
- **数据传输**：HTTP POST 请求发送请求体，SSE 流式响应返回结果
- **生命周期**：处理请求后等待关闭信号，收到后退出
- **防护机制**：第二个请求会被拒绝，返回 `ALREADY_USED` 错误
- **自动退出**：1 分钟内无请求连接，自动退出

---

## 二、环境变量

| 环境变量 | 说明 | 必填 |
|---------|------|------|
| `SSH_FLEET_KEY` | 进程认证key，用于验证HTTP请求 | 是 |
| `SSH_FLEET_PORT` | 监听端口 | 是 |
| `SSH_FLEET_LOG_PATH` | 日志目录路径，目录必须已存在 | 否（空字符串表示输出到stderr） |

缺少`SSH_FLEET_KEY`或`SSH_FLEET_PORT`时，程序将直接退出并报错。

---

## 三、API 端点

### 请求头（所有端点必需）

所有HTTP请求必须携带以下请求头：

| 请求头 | 说明 | 示例 |
|-------|------|------|
| `X-SSH-Fleet-Key` | 进程认证key，与启动时的`SSH_FLEET_KEY`环境变量一致 | `X-SSH-Fleet-Key: abc123def456` |
| `Content-Type` | 内容类型 | `Content-Type: application/json` |

缺少或key不匹配时，返回 `401 UNAUTHORIZED` 错误。

### 3.1 POST /api/v1/execute — 执行命令

执行批量 SSH 命令，返回 SSE 流式响应。

#### 请求体

```json
{
  "command": "string",
  "options": {
    "concurrency": "int",
    "connect_timeout": "int",
    "exec_timeout": "int"
  },
  "nodes": [
    {
      "seq": "int",
      "ip": "string",
      "port": "int",
      "user": "string",
      "password": "string"
    }
  ]
}
```

#### 字段说明

**顶层字段**

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `command` | string | 是 | - | 要执行的 SSH 命令 |
| `options` | object | 否 | 见下方 | 执行配置 |
| `nodes` | array | 是 | - | 目标节点列表 |

**options 对象**

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `concurrency` | int | 否 | 节点总数 | 最大并发执行数 |
| `connect_timeout` | int | 否 | 10 | 连接超时时间（秒） |
| `exec_timeout` | int | 是 | - | 命令执行超时时间（秒），必须 > 0 |

**nodes 数组元素**

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `seq` | int | 是 | - | 序列号，用于关联响应结果，不可重复 |
| `ip` | string | 是 | - | 目标服务器 IP 地址 |
| `port` | int | 否 | 22 | SSH 端口号 |
| `user` | string | 是 | - | 登录用户名 |
| `password` | string | 是 | - | 登录密码 |

#### 响应格式（SSE）

**单条结果**

```json
{
  "seq": 0,
  "ip": "10.0.0.1",
  "port": 22,
  "user": "root",
  "connect_success": true,
  "exit_code": 0,
  "output": "base64编码的输出",
  "connect_cost_time": 0.523,
  "exec_cost_time": 1.234,
  "error": null
}
```

**字段说明**

| 字段 | 类型 | 说明 |
|------|------|------|
| `seq` | int | 序列号，与请求中的 seq 对应 |
| `ip` | string | 节点 IP 地址 |
| `port` | int | SSH 端口 |
| `user` | string | 登录用户名 |
| `connect_success` | bool | SSH 连接是否成功 |
| `exit_code` | int | 执行退出码：0=成功，-1=连接失败，-10=其他错误，其他正值=命令失败 |
| `output` | string | 执行输出内容（base64 编码，stdout+stderr 交错顺序） |
| `connect_cost_time` | float | 连接耗时（秒） |
| `exec_cost_time` | float | 命令执行耗时（秒） |
| `error` | string/null | Go 层面的原始错误信息，成功时为 null |

**完成标记**

```json
{
  "type": "done",
  "total": 10,
  "success": 8,
  "failed": 2
}
```

#### 执行流程

1. 收到请求后，校验参数
2. 创建 SSH 任务，按并发数执行
3. 每完成一个节点，通过 SSE 推送一条结果
4. 全部完成后，推送 `done` 标记
5. 等待客户端发送关闭信号（10 分钟超时防御）
6. 收到关闭信号或超时后，关闭服务

---

### 3.2 POST /api/v1/upload — 上传文件

上传本地文件到远程 SSH 节点，返回 SSE 流式响应。

#### 前提条件

- Go 和 Python 运行在同一台机器上
- Go 能直接读取本地文件系统

#### 请求体

```json
{
  "file_path": "/home/user/config.yaml",
  "remote_path": "/etc/app/",
  "options": {
    "concurrency": 10,
    "connect_timeout": 10,
    "exec_timeout": 300,
    "sudo": false
  },
  "nodes": [
    {"seq": 0, "ip": "10.0.0.1", "port": 22, "user": "root", "password": "xxx"}
  ]
}
```

#### 字段说明

**顶层字段**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `file_path` | string | 是 | 本地文件或目录路径（必须是绝对路径） |
| `remote_path` | string | 是 | 远程目标目录（必须已存在且是目录） |
| `options` | object | 否 | 上传配置 |
| `nodes` | array | 是 | 目标节点列表 |

**options 对象**

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `concurrency` | int | 否 | 节点总数 | 最大并发上传数 |
| `connect_timeout` | int | 否 | 10 | 连接超时时间（秒） |
| `exec_timeout` | int | 是 | - | 上传超时时间（秒），必须 > 0 |
| `sudo` | bool | 否 | false | 是否使用 sudo 权限上传 |

**nodes 数组元素**

与 execute 端点相同。

#### 响应格式（SSE）

**单条结果**

```json
{
  "seq": 0,
  "ip": "10.0.0.1",
  "port": 22,
  "user": "root",
  "connect_success": true,
  "exit_code": 0,
  "output": "base64编码的上传结果",
  "connect_cost_time": 0.123,
  "exec_cost_time": 0.567,
  "error": null
}
```

**字段说明**

| 字段 | 类型 | 说明 |
|------|------|------|
| `seq` | int | 序列号，与请求中的 seq 对应 |
| `ip` | string | 节点 IP 地址 |
| `port` | int | SSH 端口 |
| `user` | string | 登录用户名 |
| `connect_success` | bool | SSH 连接是否成功 |
| `exit_code` | int | 失败文件数：0=全部成功，N=N个文件失败 |
| `output` | string | 上传结果（base64 编码），格式见下方 |
| `connect_cost_time` | float | 连接耗时（秒） |
| `exec_cost_time` | float | 所有文件上传总耗时（秒） |
| `error` | string/null | 节点级错误（连接失败等），成功时为 null |

**output 内容格式（base64 解码后）：**

```
total_files=5, success_files=4, failed_files=1
config.yaml: 上传成功 (0.003s)
test.txt: 上传成功 (0.002s)
SSHFleet_Go.exe: 上传成功 (0.208s)
script.sh: 上传失败 - 文件已存在
data.json: 上传成功 (0.001s)
```

**完成标记**

```json
{
  "type": "done",
  "total": 10,
  "success": 9,
  "failed": 1
}
```

done 的 total/success/failed 是节点级统计。

#### 上传行为

| 场景 | 行为 |
|------|------|
| file_path 是相对路径 | HTTP 400 错误 |
| file_path 不存在 | HTTP 400 错误 |
| file_path 不可读 | HTTP 400 错误 |
| 目录为空（全被过滤） | HTTP 400 错误 |
| remote_path 不存在 | 该节点返回错误 |
| remote_path 不是目录 | 该节点返回错误 |
| 远程文件已存在 | 该文件跳过，标记失败 |
| SSH 连接失败 | 该节点所有文件标记失败 |
| SFTP 子系统不可用 | 该节点所有文件标记失败 |
| SFTP 写入失败 | 删除远程半成品文件，该文件标记失败 |
| sudo 模式 | 写入临时目录 + sudo mv |
| root 用户 sudo | 自动跳过 sudo，直接 SFTP 写入 |
| exec_timeout 超时 | 该节点所有未完成文件标记失败 |
| 目录上传 | 自动递归遍历，过滤软链接和快捷方式 |
| 文件权限 | 保留本地权限（SFTP Create + Chmod） |

---

### 3.3 POST /api/v1/shutdown — 关闭服务

通知 Go 服务关闭。

#### 请求

无需请求体。

#### 响应

```json
{
  "status": "ok"
}
```

#### 说明

- 执行请求完成后，Go 会等待此关闭信号
- 收到信号后，Go 优雅关闭 HTTP 服务并退出
- 10 分钟未收到信号，Go 自动退出（防御性超时）

---

## 四、调用方式

### 4.1 启动服务

```bash
SSH_FLEET_KEY=your-secret-key SSH_FLEET_PORT=9090 SSH_FLEET_LOG_PATH=/var/log/sshtask ./SSHFleet_Go
```

### 4.2 发送执行请求

```bash
curl -N -X POST http://localhost:9090/api/v1/execute \
  -H "Content-Type: application/json" \
  -H "X-SSH-Fleet-Key: your-secret-key" \
  -d @request.json
```

### 4.3 发送上传请求

```bash
curl -N -X POST http://localhost:9090/api/v1/upload \
  -H "Content-Type: application/json" \
  -H "X-SSH-Fleet-Key: your-secret-key" \
  -d '{
    "file_path": "/home/user/config.yaml",
    "remote_path": "/etc/app/",
    "options": {"concurrency": 10, "connect_timeout": 10, "exec_timeout": 300, "sudo": false},
    "nodes": [{"seq": 0, "ip": "10.0.0.1", "port": 22, "user": "root", "password": "xxx"}]
  }'
```

### 4.4 发送关闭信号

```bash
curl -X POST http://localhost:9090/api/v1/shutdown \
  -H "X-SSH-Fleet-Key: your-secret-key"
```

---

## 五、超时机制

| 超时类型 | 计算方式 | 说明 |
|----------|----------|------|
| 无请求超时 | 固定 1 分钟 | 启动后 1 分钟内无请求连接，自动退出 |
| 单节点连接超时 | `options.connect_timeout` 参数控制 | 超时后该节点标记为连接失败 |
| 单节点执行/上传超时 | `options.exec_timeout` 参数控制 | 超时后该节点所有未完成任务标记失败 |
| 关闭等待超时 | 固定 10 分钟 | 执行完成后等待关闭信号，超时自动退出 |

---

## 六、错误处理

### 6.1 错误响应

当请求格式错误或服务内部异常时，返回错误响应：

```json
{
  "success": false,
  "error": {
    "code": "INVALID_REQUEST",
    "message": "nodes 数组不能为空"
  }
}
```

### 6.2 错误码

| 错误码 | HTTP 状态码 | 说明 |
|--------|-------------|------|
| `INVALID_REQUEST` | 400 | 请求格式错误（JSON 解析失败） |
| `MISSING_FIELD` | 400 | 必填字段缺失（command/file_path/remote_path 为空、nodes 为空、seq 重复） |
| `INVALID_PATH` | 400 | file_path 不存在、不可读或不是绝对路径 |
| `UNAUTHORIZED` | 401 | 请求头缺少`X-SSH-Fleet-Key`或key无效 |
| `INTERNAL_ERROR` | 500 | 内部错误（不支持流式响应） |
| `ALREADY_USED` | 503 | 服务已被调用，仅支持一次请求 |
| `LOG_PATH_INVALID` | - | 启动失败：日志路径不存在或不是目录，输出到 stderr |

---

## 七、项目结构

```
internal/
├── httpserver/
│   ├── server.go              # HTTP 路由 + 请求处理
│   └── sse.go                 # SSE 写入工具
├── core/
│   ├── batch_executor.go      # 命令执行器
│   └── batch_upload_executor.go # 上传执行器
├── jsonproc/
│   ├── json_type.go           # 请求结构体定义
│   └── json_parser.go         # 请求解析 + 验证
├── localfs/
│   └── collector.go           # 本地文件收集（递归遍历、软链接过滤）
├── ssh/
│   ├── ssh_run.go             # SSH 连接 + 命令执行 + SFTP 上传
│   └── ssh_result.go          # 结果结构体定义
├── interrupt/
│   └── interrupt.go           # 信号中断处理
└── log/
    ├── logger.go              # 日志初始化
    └── sucessLevel.go         # 自定义 SUCCESS 级别
```

---

## 八、注意事项

1. **output 字段为 base64 编码**，调用方需解码后使用
2. **execute 的 output** 包含 stdout 和 stderr，按交错顺序拼接
3. **upload 的 output** 包含统计信息和每个文件的上传状态
4. **exit_code 含义不同**：execute 是命令退出码（0=成功，-1=连接失败，-10=其他错误），upload 是失败文件数（0=全部成功）
5. **seq 用于关联请求与响应**，并发执行时结果顺序可能与请求顺序不同，seq 不可重复
6. **error 只在 Go 层面出错时有值**（连接失败、Go 内部异常），命令执行失败看 exit_code
7. **一次性执行**：每次启动只处理一次请求，第二个请求会被拒绝
8. **优雅退出**：执行完成后需调用 shutdown 端点，否则等待 10 分钟超时退出
9. **上传前提**：Go 和 Python 必须运行在同一台机器上，Go 直接读取本地文件
10. **远程路径要求**：upload 端点的 remote_path 必须已存在且是目录，不存在则报错
