# 规格：helper 断言入指纹

**状态**：done
**底本**：`docs/adr/0046-helper的断言也要入指纹.md`（决定）
**事实底本**：`docs/issues/2026-11-09-1315-检查器自反补硬/issues/02-断言强度入指纹.md` 的 Comments（`TestCheckDepsClock` 记 0）、`modules/SSHFleet2_Go/internal/guard/helpers_test.go`

## Problem Statement

检查器现在能看见三种拆法：删入口、删测试、掏空测试体里的断言。还剩一层没看见——**把断言搬进 helper 再掏空 helper**。

`TestCheckDepsClock` 整条测试只有一行调用，四句 `t.Fatalf`／`t.Errorf` 住在 `helpers_test.go` 的 `wantOne` 里；`loadFixture` 也在替测试喊失败。把它们删掉，几条调用它们的红线一起变松，而符号指纹、次数指纹一个都不动。

## Solution

`guard` 包里凡是替测试喊失败的 helper，都要在名单旁边登记「它是谁、喊几次」。没登记的新 helper、次数对不上的改动、登记了却不在了的函数，`go test` 都红。

## Implementation Decisions

- **扫描面**：`guard` 包里所有 `_test.go` 的非 `Test*` 顶层函数，函数体里出现 `Fatal`／`Fatalf`／`FailNow`／`Error`／`Errorf`／`Fail` 的调用点即算「喊失败」。
- **登记表**：与 `redline` 同住一处（`internal/guard/redline.go` 或紧邻的小文件），形态 `{名字, 次数}`；初值 `loadFixture` 1、`wantOne` 4。
- **判据**：喊了但没登记 → 红，并给出该补的那一行；次数对不上 → 红，报前后两次；登记了但函数没了 → 红。
- **口径**：只数调用点，不数条件表达式（与 `ADR-0045` 决定二同一把尺）。
- **位置**：判据进 `CheckRedline` 一族（类「红线名单与指纹」），在 `CheckAll` 里与名单一起跑；不新开类号。

## Testing Decisions

- 接缝是 `go test ./internal/guard` 本身，不新开接缝。
- 在 `build/` 的模块副本里做两次真实拆除，每次跑一次：
  1. 删掉 `wantOne` 里一个 `t.Errorf` → 红，指出 `wantOne` 的次数从 4 变 3；
  2. 在 `helpers_test.go` 新增一个喊 `t.Fatalf` 的 helper、不登记 → 红，指出它没登记。
- 两次都还原，再跑一次确认全绿。
- 不 mock 检查器自身，不做假绿灯。

## Out of Scope

- 断言**内容**的强弱（把 `!=` 改成 `==`、把话术断言删到只剩符号引用）：`ADR-0045` 与本篇都只钉子数，读文本的成本不成比例，不做。
- 红线区里那约 10 处行为断言与「50 个等号」（审计 11-J1／11-H3）：属改红线，另开议题，排在 0044 之后。
- 其它包的测试 helper：本判据只看 `guard` 包。

## Further Notes

- 改登记表与改红线同程序，`ADR-0032` 更新记录第 7 条「单独点头」档。
- 与 `ADR-0045` 的两张表合起来，`go test` 能看见四种拆法；把这四种写进 `guard/doc.go` 一句话，省得下一个人再问「这都在防什么」。
