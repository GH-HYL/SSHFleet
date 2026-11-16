# 05: `mask.Marker` 收回包内

**要做的**：`internal/mask/mask.go:20` 的 `const Marker` 降为未导出 `marker`；`:13-18` 那段「导出只为结构检查类七」的理由删掉——类七不读这个符号（它用 `strings.Repeat("*", 4)` 自拼，`guard/check_mask.go:25`），下游零处引用。

**Blocked by**：无，可立即开工。

**Status**：done

- [ ] `Marker` → `marker`，`Secret` 里两处引用跟着改
- [ ] 注释改成真实理由：屏幕、内联清单、`report.txt`、清单副本四处遮出来是同一串，只此一份
- [ ] `guard/check_mask.go` 一个字不改（`mate()` 本来就不读它，这是这条导出理由不成立的原因）
- [ ] 类七自检仍过：`go test -count=1 -run TestCheckMask ./internal/guard`
- [ ] `grep -rn "mask\.Marker" modules/SSHFleet2_Go` 零处
- [ ] `go test -count=1 ./...` 全绿；`go vet`、`gofmt -l` 干净

## Comments

**2026-11-16 拆单**：类七要认得那个记号，走的是「构造四个连续星号」而不是引用这个常量——`check_mask.go:25` 的注释把理由写清了（抄一份字面量就多一个出处）。所以导出这唯一一条理由本来就不成立。

**2026-11-16 落地**：`Marker` 降成 `marker`，注释改成真实理由（四处遮法是同一串，只此一份），并写明类七为什么不引用它。`guard/check_mask.go` 一个字没动。`grep -rn "mask\.Marker"` 零处；`internal/mask` 与 `internal/guard` 两包绿。
