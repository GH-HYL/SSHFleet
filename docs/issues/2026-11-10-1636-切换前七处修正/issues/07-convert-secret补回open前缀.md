# 07: `--convert-secret` 原因补回 `open <路径>:` 前缀

**要做的**：读不到文件时，那一句原因保留系统原文里的 `open <路径>:`，让使用者看得出是哪个文件。判据：改前原因只剩后半句、改前改后截屏对比。

**Blocked by**：无。

**Status**：done

- [x] 先复现：`--convert-secret nope.txt`，抄下屏上那句原因
- [x] 原因保留 `open <路径>:` 这一截，与执行日志里那句同形
- [x] 只改这一类（读文件失败），不扩大到别的 `SystemReason` 用法
- [x] 已知差异第三条据此转成「已回改」，在 4.4 的结论表里登记
- [x] 用例覆盖：读不到文件时原因含路径前缀
- [x] `go test ./...` 绿

## Comments

**2026-11-10 收账**

- **改前的事实**：`credential/convert.go` 的 `ReadConvertible` 读文件失败时返回 `SystemReason(err)`，而 `credential.SystemReason` 把 `*os.PathError` 的前缀剥成 `pe.Err.Error()`。于是 `--convert-secret nope.txt` 那一屏的「原因」只剩 `no such file or directory`，`open nope.txt:` 那截没了——使用者看不出是哪个文件打不开（审计差 3，判「不合理」）。
- **为什么不改 `SystemReason`**：它是 `message` 与 `credential` 各一份、被许多屏共用的口径（那几屏第一行已经说过路径，前缀是重复）。改它会把别处的原因一并带上前缀，那是另一笔行为改动。
- **改法只落在这一条路径上**：`ReadConvertible` 的读失败分支直接返回 `err.Error()`，保留 `open <路径>:` 原样。注释里写明「别处仍走 SystemReason」。
- **用例** `credential/convert_test.go` 两条：`TestReadConvertibleKeepsOpenPrefix`（原因以 `open ` 开头、且含那条路径）、`TestReadConvertibleOtherProblemsCarryNoReason`（空文件／二进制／多行三种毛病的 reason 必须为空——它们的毛病不在系统那一层，不该顺带带上系统原因）。
- **记账**：09 的已知差异第三条从「行为变了（无点头）」转「已回改」。切换前那批改完，4.4 的 07 号单复跑时这条应当不再出现在差异表里。
- 实跑：同上，十七包全绿。
