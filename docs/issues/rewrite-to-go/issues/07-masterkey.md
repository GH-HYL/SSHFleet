# M2-07 credential：主密钥（SSHFLEET_KEY / --gen-key）

Type: task
Status: resolved
Resolved: 2026-09-11
Blocked by: 01

## 范围

`internal/credential` 主密钥部分：

- 环境变量 `SSHFLEET_KEY` 读取（D15 统一名）
- Windows：**读取**走注册表直读（HKCU\Environment，不起 reg 子进程——spec 实现途径 7）；**写入**维持 `setx`（与旧一致）
- Linux/macOS：检测登录 shell（$SHELL），zsh → `~/.zshrc`、bash/其他 → `~/.bashrc`（D30）；读取已持久化密钥时两份 rc 都查
- `--gen-key`：token_urlsafe(36) 等价生成（crypto/rand 36B → RawURLEncoding）；已存在密钥时先提示再确认覆盖，非交互拒绝自动覆盖（旧版同级兜底）
- 覆盖确认经 `internal/common` 交互器（M2-10 落地）

## 验证

- 编译通过；gen-key 流程不实际执行（避免改动本机注册表），逻辑走查 + 与旧文案对齐

## Comments

- 2026-09-11 完成。Windows 读取走注册表直读（`x/sys/windows/registry`，实现途径 7）、写入维持 `setx`；Unix 按 `$SHELL` 选 rc 文件（D30），读取时 `.zshrc`/`.bashrc` 都查。gen-key 覆盖确认的 EOF 语义按旧 `_confirm_overwrite`（视为「否」，保留原密钥）。为避免改动本机注册表，未实际执行 gen-key，仅逻辑走查。
