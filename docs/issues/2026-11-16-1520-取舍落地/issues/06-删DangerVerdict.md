# 06: 删掉契约里的死类型 `DangerVerdict`

**要做的**：`internal/contract/danger.go:20-25` 的 `DangerVerdict` 删掉，连同那段指向被禁方法的注释（「契约不许有函数，所以这里是字段——由闸门判定时写好」）。同处删 `internal/guard/paper.go:71` 的一行 `{"DangerVerdict", "Highest", PaperFact}`。

**Blocked by**：无，可立即开工。

**Status**：done

- [ ] `contract/danger.go` 里只剩 `DangerMatch`、`DangerReport` 两个类型
- [ ] `guard/paper.go` 那一行删掉；`grep -rn "DangerVerdict" modules/SSHFleet2_Go` 零处
- [ ] 闸门一个字不改：它用的是 `DangerReport` + `message.DangerBox(report, true/false)`（`gate/run.go:343-369`）
- [ ] 契约类检查仍绿：`go test -count=1 ./internal/guard ./internal/contract`
- [ ] `go test -count=1 ./...` 全绿；`go vet`、`gofmt -l` 干净

## Comments

**2026-11-16 拆单**：核现场确认这一格是空的——闸门那条路根本走 `DangerReport` 加一个 bool 参数，没有任何一处造 `DangerVerdict`。它进纸面名单的唯一原因是 `Highest` 是裸 `string`，要名单兜着。

**2026-11-16 落地**：`contract/danger.go` 只剩 `DangerMatch`、`DangerReport`；`guard/paper.go` 那一行删掉。`grep -rn DangerVerdict` 只剩历史文档里的记述，代码零处。闸门未动（`gate/run.go:343-369` 用的是 `DangerReport` 加一个 bool）。`internal/guard` 全绿（含类四纸面那条与「名单多一行不误报」那条）。
