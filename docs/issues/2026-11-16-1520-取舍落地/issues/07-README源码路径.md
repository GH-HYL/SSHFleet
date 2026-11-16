# 07: README 的源码路径写的是老工程

**要做的**：`README.md:115` 那句「源码在 `modules/SSHFleet_Go/`，需要 **Go 1.27+**」改成新工程的路径；Go 版本那一句按 `modules/SSHFleet2_Go/go.mod` 的 `go 1.26.0` 核一遍。

**Blocked by**：无，可立即开工。

**Status**：done

- [ ] 路径改成 `modules/SSHFleet2_Go/`
- [ ] Go 版本那一句与 `go.mod` 对齐（写 `1.26+`，或保留更高要求但在 `## Comments` 写明为什么）——两者不可各写一个
- [ ] `grep -n "SSHFleet_Go" README.md` 零处（老工程名整份 README 里只这一处）
- [ ] `CHANGELOG.md` 的 `## 未发布` 加一条 `Changed`
- [ ] 其余不动：这句话只是「自行编译（可选）」那一段，README 里别的段不谈源码位置

## Comments

**2026-11-16 拆单**：README 随包分发（`tools/release/release.py:239` 拷进包根），使用者照着 `modules/SSHFleet_Go/` 找会找不到——包里根本没有 `modules/`。属说明性文字，按规范一不受「字节不变」约束；但它是使用者可见面，所以记 `CHANGELOG`。1.0.0 的包不重打，随下一个版本进包。

**2026-11-16 落地**：路径改成 `modules/SSHFleet2_Go/`；Go 版本照 `go.mod` 的 `go 1.26.0` 写成 **1.26+**，并把「在工程目录下」改成「在那个目录下」（原来的「工程目录」正是让人去仓库根 `go build` 的来处）。`grep -n "SSHFleet_Go" README.md` 零处；`CHANGELOG.md` 的 `## 未发布` 记了 `Changed`。1.0.0 的包没重打，随下一个版本进包。
