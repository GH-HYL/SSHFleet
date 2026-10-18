# 04: 删掉 `Interactor` 上两个没有调用者的方法

Status: done（2026-10-18 实施）
Blocked by: None（可立即开工）

**要做的**：交互入口 `Interactor` 上那两个「读一行文本」「读一行密码」的方法，包外没有任何调用者，**删掉**，连同只服务于它们的内部测试。删完之后 `Interactor` 对外只保留 `Confirm`、`Notice` 与构造、开关这些还在用的东西，编译与现有测试照旧全绿。

**被谁阻塞**：无，可立即开工。

---

## 现状

`Interactor` 是工具唯一的交互入口。它上面有两个导出方法：

- 读一行文本（`Prompt`）
- 读一行密码（`PromptPassword`，内部转调前者）

**两者都没有包外调用者**：全仓搜 `.Prompt(` / `.PromptPassword(` 只命中方法自身内部那一处。

`Interactor` 本体是在用的——`Confirm`、`Notice`、`Disinteractive`、`NewInteractor` 都被正常调用，所以这不是整块死代码，只是这两个方法悬空。

它们曾经有用途：补输入交互整体移除之后留下的。注释写着「保留供日后改回来」，但开发者 2026-10-18 已确认**无调用者，可以清理**。

## 要做的事

1. 删掉这两个导出方法。
2. 删掉只检查这两个方法的测试。
3. 顺带检查二者用到的依赖是否也随之不再需要（读密码那条用到了终端无回显读取）。

## 验收

- [x] 全仓搜 `.Prompt(` / `.PromptPassword(` 零命中
- [x] `go build ./...` 通过
- [x] `go test ./internal/common/` 全绿

## Comments

- 2026-10-18 完成。开工前核过：包外零命中，`interactor_test.go` 也没有针对这两个方法的用例。删除时连带移除了 `interactor.go` 里随之不再使用的 `golang.org/x/term` 导入；`readLine` 仍被 `Confirm` 使用，保留。构建、vet、`internal/common` 测试均通过。
- 2026-10-18 补记依赖核对（工单「要做的事」第 3 条）：`golang.org/x/term` **仍被 `internal/cli/args.go` 使用**（读终端宽度），`go.mod` 里的直接依赖因此照旧保留，不从 `go.mod` 摘除。
