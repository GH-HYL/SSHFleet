# 01: 十七份临时根收成一处

**要做的**：新建 `internal/testtmp`，把「把测试二进制的临时根钉进工作区」写一次；十七个包的同名 `tmpdir_test.go` 各缩成一行空引用；三处直接取沙盒路径的测试改读 `TMPDIR`。做完之后改沙盒路径只改一处。

**Blocked by**：无，可立即开工。

**Status**：done（done）

- [x] `internal/testtmp/tmpdir.go` 一个 `init` 推到 `build/tmp/test/<包名>`，定位不到 `go.mod` 就不改（照旧靠人 export）
- [x] 十七份 `tmpdir_test.go` 变成 `import _ "sshfleet2/internal/testtmp"`
- [x] `guard/redline_tip_test.go`、`guard/redline_tip_endings_test.go`、`keytool/run_confirm_test.go` 改从 `TMPDIR` 取沙盒
- [x] 规则表加 `testtmp` 一行（横切、依赖空），两处报错文案与两条 guard 用例的包数同步到二十
- [x] `gofmt -l internal/` 空、`go vet ./...` 干净、`go test -count=1 ./...` 全绿

## Comments

- 2026-11-18：**已落地**。行数从 816 降到 `internal/testtmp/tmpdir.go` 62 行 + 十七个三行桩。桩里的注释只说「见那个包」，理由（为什么不用 TestMain、为什么必须钉）集中在新包的头注释里，不再抄十七份。
- 验收：`bash tools/test.sh ./...` 二十项全绿（`contract` 与 `testtmp` 无测试）；`build/tmp/test/` 下仍按包名分目录，`guard` 那份里能看到红线测试编出来的二进制。
- 一处副作用：模块根那一份的沙盒目录名从 `root` 变成 `SSHFleet2_Go`（按规则取当前目录末一段），旧目录留在 `build/` 里不影响任何东西。
