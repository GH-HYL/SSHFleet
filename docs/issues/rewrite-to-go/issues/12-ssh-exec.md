# M3-12 ssh：连接 + 命令执行

Type: task
Status: resolved
Resolved: 2026-09-14
Blocked by: 11

## 范围

`internal/ssh` 连接与命令执行部分（对位旧 `ssh_run.go`）：

- `Config`（IP/Port/User/Password/KeyContent/KeyPassphrase/ConnectTimeout/ExecTimeout）+ `Client`（生命周期 → 带方法类型）
- 连接：主机密钥**跳过校验**（ADR-0001）；超时＝dial 超时 `timeout_connect` + 握手 `+2s` 宽限（对位旧 `ClientConfig.Timeout` + `time.After(T+2s)` 双机制）；失败消息「建立连接失败 - …」保持
- 认证（直接沿用旧 `BuildAuthMethods`）：密钥优先、密钥解析失败且配了密码才回退密码；两者皆无 → 「未提供有效的认证方式」；日志/描述用「密钥/密码」这类口径
- 命令执行：
  - 命令/脚本内容经 **session.Stdin 直喂**，命令只剩 `bash -lc '<export LC_ALL…; sudo bash|bash>'`（spec 实现途径 1，去掉 base64 通道）
  - `--nobash`（仅命令模式）原样下发，不套 `bash -lc`、不喂 stdin
  - 脚本解释器按扩展名：`.py` → `python3`，否则 `bash`
  - sudo 由 `-m sudo` 决定（脚本模式为 `sudo <interpreter>`）
  - stdout/stderr 合并写入同一带锁缓冲（顺序保持）；输出**不再 base64 编码**（spec M3 差异）
  - 超时改 `context.WithTimeout` + `defer cancel()`（旧 `select + time.After`）；执行超时文案「命令执行超时(…s)」
  - 退出码：仅 `*ssh.ExitError` → 真实码；超时/中断 → nil（CONTEXT「退出码」定义）
- 结果结构：单一 `Result` 类型承载四种模式（旧三个形状相同的 struct 合并，D8 全面择优）

## 验证

- 对测试节点执行 `echo` / 非 0 退出命令 / 超时命令（带超时）/ `--nobash`，核对退出码与输出
- 核对 `-m direct` 与 `-m sudo`

## Comments

- 2026-09-14 完成。真机验证（测试节点 172.28.118.49，全部带超时）：`echo` → exit 0、输出正确；`exit 7` → 退出码 7；`sleep 30` + `-t 3` → 报「命令执行超时(3s)」耗时 3.00s、退出码 nil；`--nobash` 直发成功；`-m sudo` → `id -u` 返回 0。连接超时改为「dial 超时 + 握手 +2s 宽限」（对位旧双机制，去掉了 goroutine+timer）。
