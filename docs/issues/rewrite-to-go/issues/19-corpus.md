# M4-19 语料迁入新工程并常驻为回归测试

Type: task
Status: resolved
Resolved: 2026-09-14
Blocked by: 18

## 范围

用户 2026-09-14 裁定：旧工程语料迁入新工程、常驻为 Go 回归测试（而非临时跑一遍删除）。

- `testdata/dangerous_cases.tsv`：81 例（命令 / 期望级别 / 期望分类名），自 `test/test_dangerous_detection.py` 的 CASES **程序化抽取**（AST 取值，不手抄）
- `testdata/error_classification_cases.toml`：24 例（classify 入参 + 期望分类），自 `test/test_error_classification.py` 的 `check(classify(...))` 调用程序化抽取
- 常驻测试：`internal/dangercheck/dangercheck_test.go`（语料 + 规则校验 + 未知字段）、`internal/result/result_test.go`（语料 + 归类断言 + 兜底判定 + AuthFailure 优先级）
- 规则文件本身也在仓库内（`config/*.toml`），测试直接加载，改规则后跑测试即知是否退化

## 验证

- `go test ./internal/...` 全绿：dangercheck（81/81）、result（24/24 + 断言）
- 抽取脚本两次踩坑已修：TOML 无 `null`（改省略表示 nil）、关键字参数与 `is_fallback_category` 调用需按函数名过滤——抽取逻辑写在工单 Comments 备查

## Comments

- 2026-09-14 完成。语料随仓库分发，别人 clone 后可直接跑；后续改解析层或规则时识别率退化会当场被拦。
