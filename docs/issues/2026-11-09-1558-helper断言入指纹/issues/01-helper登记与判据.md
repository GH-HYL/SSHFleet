# 01: helper 登记与判据

**要做的**：把 `wantOne` 里的一个断言删掉 → `go test` 红，指出是哪个 helper、次数从几变成几；新增一个喊失败的 helper 不登记 → 红，指出该补哪一行。

**Blocked by**：无。

**Status**：done

- [x] 扫 `guard` 包所有 `_test.go` 的非 `Test*` 顶层函数，数出「喊失败」的调用点
- [x] 登记表落 `internal/guard/`，与 `redline` 同住或紧邻；初值 `loadFixture` 1、`wantOne` 4
- [x] 喊了没登记 → 红，给出要补的那一行字面量
- [x] 次数对不上 → 红，报「名单里 N 次，现在是 M 次」
- [x] 登记了但函数不在了 → 红
- [x] 判据挂在「红线名单与指纹」这一类里，随 `CheckAll` 跑；不新开类号
- [x] 副本里做两次真实拆除（删一个 `t.Errorf`、加一个未登记的 helper），每次红；还原后全绿
- [x] `go test ./...` 绿（`gofmt`／`go vet` 干净）

## Comments

- 2026-11-09：落地。登记表 `helperAsserts` 与名单同住 `internal/guard/redline.go`，判据 `helperViolations` 由 `CheckRedline` 调用，类号仍是「红线名单与指纹」。
- 规格写的初值只数到两个（`loadFixture` 1、`wantOne` 4），扫全包实际是七个：另有 `closedPort` 3、`readText` 1、`tipEndingsProbe` 4、`copyRedlineDangerRules` 3、`writeRedlineConfig` 2。都补进了登记表——ADR-0046 决定一要求扫全包，只钉两个正是它否掉的方案。
- 登记表只对本工程生效：`helperHomeModule` 挡住样本模块。`testdata/testsite` 里那个同名 guard 包没有这些函数，不加挡板时每一条都会被报成「已经没有了」。
- 样本 `testdata/helper/`（`shout` 两次喊失败、`TestThing` 一行调用）单测覆盖三种红与一种绿；真拆验证在 `build/自反_helper_20261109_1630/SSHFleet2_Go`。
- `go test ./...` 除 `parse` 的 `TestCollectUnreadableFileFails` 外全绿；那条是 `/mnt/c` 为 drvfs、`chmod` 不生效导致的假红，与本轮改动无关。
- 两轴审查后补三处：`guard/doc.go` 里「删测试」的归属写错（名单在 `redline.go`，不在 `registry.go`）；ADR-0046「影响」那条仍写两个 helper，回填成七条；测试加一段「真模块不许空转」——清空登记表必须报出没登记的 helper。最后这条同时钉住 `helperHomeModule`：这个常量一写错，判据整段静默跳过而真工程仍旧全绿，正是这张表要防的假绿灯。
