# 01: helper 登记与判据

**要做的**：把 `wantOne` 里的一个断言删掉 → `go test` 红，指出是哪个 helper、次数从几变成几；新增一个喊失败的 helper 不登记 → 红，指出该补哪一行。

**Blocked by**：无。

**Status**：ready-for-agent

- [ ] 扫 `guard` 包所有 `_test.go` 的非 `Test*` 顶层函数，数出「喊失败」的调用点
- [ ] 登记表落 `internal/guard/`，与 `redline` 同住或紧邻；初值 `loadFixture` 1、`wantOne` 4
- [ ] 喊了没登记 → 红，给出要补的那一行字面量
- [ ] 次数对不上 → 红，报「名单里 N 次，现在是 M 次」
- [ ] 登记了但函数不在了 → 红
- [ ] 判据挂在「红线名单与指纹」这一类里，随 `CheckAll` 跑；不新开类号
- [ ] 副本里做两次真实拆除（删一个 `t.Errorf`、加一个未登记的 helper），每次红；还原后全绿
- [ ] `go test ./...` 绿（`gofmt`／`go vet` 干净）
