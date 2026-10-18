# 05: 日志与报告留痕

Status: done
Blocked by: 01

**要做的**：`Summary()` 加 `answer` 字段，排在 `k=` 之后——给了就是 `answer=['1,请选择架构']`、没给就是 `answer=[]`。report 的【执行参数】按命令 / 脚本模式增加 `-a` 明细行：每条一行，含代填内容与它的全部触发词，顺序同命令行。

均为全量明文（内容 + 触发词），开发者 2026-09-28 裁定接受明文留痕。

- [ ] 工具日志里能看出这次配了哪几条代填，没配时是 `answer=[]`
- [ ] report.txt【执行参数】每条一行、含全部触发词、顺序同命令行
- [ ] `internal/cli/args_test.go` 里 `TestSummaryMatchesArgparseNamespace` 的逐字断言同步改过
- [ ] `go vet ./...` 干净、`go test ./...` 全绿

## Comments
- 2026-09-28 完成，提交 36d92c6。验证：真机：工具日志的解析结果里 `answer=['1,请选择架构']`（取 `-a` 值原文——该行打在参数合规检查之前，代填表那时还没解析出来）；report.txt 与参数确认屏各列出代填明细；`args_test.go` 的逐字断言同步改过。
