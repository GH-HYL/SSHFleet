# 01: 测试临时根钉进 build/tmp

**要做的**：不 export 任何环境变量，直接 `go test -count=1 ./...`，测试跑出来的临时目录全在 `build/tmp/test/<包>` 下；工作区外的系统临时目录里不再出现本工程的测试目录。

**Blocked by**：无，可立即开工。

**Status**：done

- [x] 每个有测试的包都有入口，在跑任何测试之前把 `TMPDIR`／`TMP`／`TEMP` 指到 `build/tmp/test/<包>`，目录不存在就建
- [x] 干净 shell（不带任何 `TMPDIR`／`TMP`／`TEMP`）下 `go test -count=1 ./...` 全绿
- [x] 同一条件下跑完，`%TEMP%`（Windows）或 `/tmp`（Linux）下没有新增本工程的测试目录
- [x] `build/tmp/test/` 下按包分目录，名字与被测包对得上
- [x] 不改任何测试逻辑，不碰生产代码；`go vet ./...`、`gofmt -l` 干净

## Comments

**2026-11-16**：入口取的是 `init`，不是 `TestMain`。先按 `TestMain` 写，`guard` 当场红——类三「退出动作」连 `_test.go` 一起看，`TestMain` 收尾那句 `os.Exit` 在站包里就是违规；名单那一条又把 `TestMain` 当红线测试要登记，等于为了钉个临时目录去改红线。改成 `init`：测试进程一起来就钉，不碰退出，也不进任何名单。

17 个有测试的包各一个 `tmpdir_test.go`，内容同一个形状：上溯找 `go.mod`，落到「模块根上两级/build/tmp/test/<包>」；定位不到就原样不动（宁可回落到系统默认，也不猜一个地方写产物）。

实测：清空 `build/tmp/test` 后 `env -u TMPDIR -u TMP -u TEMP` 跑全量，17 包全绿，`build/tmp/test/` 下长出 17 个按包分的目录，`%TEMP%` 目录数前后都是 1193。
