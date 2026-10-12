# 03: 单引号转义规则收成一份

Status: resolved
Blocked by: None

**要做的**：把 `'\''` 这个转义规则从两处字面收成一个函数（原报告候选五）。

**现状**：

- `ssh/command.go:85-87` `shellQuote`：包裹 + 转义，真下发用。
- `ssh/command.go:99-107` `QuoteForShell`：给人看的命令行，**已委托** `shellQuote`（没问题）。
- `ssh/sftp.go:469-472` `escapeShellArg`：只做转义、不包裹，调用点 `:198/:200/:241/:461`（预检 / find / mv）。

转义规则（把单个单引号换成 `'\''`）在 `command.go:86` 与 `sftp.go:471` **各写了一遍**。`sftp.go:197` 还留着注释「单引号转义与 find/mv 同口径（2026-09-14 审计修复）」——**靠注释提醒人保持同口径本身就是症状**；这个口径已经出过一次真毛病（CHANGELOG 记过「下载模式的远程路径预检对含单引号的路径做了转义，不再拼出坏命令」）。

**改法**：新增一个**只做转义**的函数（如 `escapeSingleQuotes`），`shellQuote`（包裹）与 `escapeShellArg`（嵌入既有单引号对）都调它。两个函数**仍然分开**——语义确实不同（一个整包，一个嵌入），要消掉的只是那份写了两遍的规则本身。

- [ ] 全仓搜 `'\''` 字面只剩一处（新函数内）
- [ ] `shellQuote`、`QuoteForShell`、`escapeShellArg` 行为不变（构造带单引号的路径与命令逐条核）
- [ ] `ssh/sftp.go:197` 那句「同口径」注释删除或改为指向单源
- [ ] `go vet ./...` 干净、`go test ./...` 全绿

## Comments
- 2026-10-07 完成，提交 `83ff09f`。新增 `ssh.escapeSingleQuotes`，`shellQuote` 与 `escapeShellArg` 都调它；`command.go:86`、`sftp.go:471` 两处字面合一。
