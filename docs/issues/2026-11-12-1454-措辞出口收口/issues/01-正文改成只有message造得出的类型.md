# 01: 那一屏的正文改成只有 message 造得出来的类型

**要做的**：往后往「起不来／拒绝」那一屏塞正文，只能先说出「哪一类、什么参数」，由 `message` 取模板成形；随手拼一个字符串塞进去这件事，编译不过。屏上字节与现在一字不差。

**Blocked by**：无，可立即开工。

**Status**：done

- [x] `message` 里有一个具名类型承载那一屏的正文，字段不导出；本包之外造不出值
- [x] `Fatal`、`TraceRefusal` 只收这个类型；`Shell` 与 25 个 `Refuse*` 改成返回它，闸门与塔尖跟着改完
- [x] `CannotStartText` 仍给测试与日志一个字面量（底层与上屏那份同一实现，不各拼一遍）
- [x] `go build ./...` 与 `go vet ./...` 干净；`go test ./...` 除已知假红外全绿
- [x] 取一屏「起不来」与一屏「拒绝」的现状文本留档，改完逐字比对，报告里给出结论
- [x] 反向验证：在 `message` 之外写一处 `Fatal(where, "…", logPath)`，确认编译不过；撤销后恢复绿

## Comments

- 2026-11-12：类型叫 `FatalBody`（`internal/message/body.go`），不叫 `Body`——契约里 `CannotStart.Body` 那一格刚在 0044 的 03 号单删掉，重名会把「纸里的句子」与「屏上的正文」混成一件事。
- 改的返回类型不止 25 个 `Refuse*`：另有 `Shell`、`RefuseEnvelopeMulti/Empty/Wrong`（住在 `confirm.go`）、以及新加的 `CannotStartBody`。`CannotStartText` 保留返回 `string`（80 余处测试与日志在用它），底层与上屏那份共用 `cannotStartText`，不各拼一遍。
- 拒绝那 34 个调用点里 29 个在 `gate/run.go`、5 个在 `gate/refusal_trace_test.go`；塔尖 `fatal` 闭包改收 `FatalBody`，落日志时取 `.Text()`。
- 反向验证：改完 `Fatal` 的签名之后先不迁塔尖，`go build` 直接把剩下两处点出来——`main.go:130` `aerr.Error()` 与 `main.go:176` `rerr.Error()`，正是 02 号单要收的那两处。这就是「闸门在类型上成立」的证据，比事后造样本硬。
- 屏字节：取改前二进制，在同一个目录、同一份 `config/`（Windows 版）下跑五场——`--zzz`（不认识的选项）、`-c`（缺值）、`--change-password --sudo`（互斥）、`-c ls`（起不来）、`-f`（模式零个）——`stdout` 与 `stderr` 各自 `cmp`，五场全 `same`。
- 测试现状：`go test ./...` 仍红 5 条，全部是环境：`TestRedlineTipCannotStart`／`TestRedlineTipRefused`（`go build -o` 出的无扩展名文件在 Windows 上 exec 不了，报 `executable file not found in %PATH%`）、`TestCollect{SkipsSymlinks,RootSymlinkFails,AllFilteredFails}`（造软链接被 360 安全大脑拦，报 `A required privilege is not held by the client`）。**改前树上同样红**：在同一 HEAD 上另开一个 worktree 复跑过这两组，报错一模一样。
