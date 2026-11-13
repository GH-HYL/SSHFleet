# 03: CSV 读法与「算不算一条内联记录」各收成一份

**要做的**：`parse/nodelist.go:120` 与 `mask/csv.go:14` 现在是两份 CSV 读法，且遮罩那份多开了 `LazyQuotes`（实测：同一组畸形值，取值侧读不出、遮罩侧读得出）。老版这两件事各只有一份，`ReadCSVRows` 与 `LooksLikeInlineNodeRecord` 同住 `common`，取值判定与脱敏都调它（ADR-0023 决定 5 点名）。这一单把它们搬回一处。

**Blocked by**：无，可立即开工。

**Status**：done

- [x] `criterion` 加 `ReadCSVRows(text string) ([][]string, []int, error)`：剥 BOM、`#` 注释恒跳过、空行跳过、列数不限、引号成对可含逗号与换行；第二个返回值是每行在原文里的行号（1 起，空行与注释行照样占号）
- [x] **不保留 `LazyQuotes`**：全工程一处口径，一律严格，与老版同。畸形值在遮罩侧读不出就原样返回——这条与老版一致；收口前核过 argv 的去处（只经 `mask.CommandLine` 进 `report.txt`，拒绝与起不来不写报告），所以这处改动不会把明文带进日志
- [x] `parse/nodelist.go` 的 `csvRows` 改为薄壳调 `criterion`（行号语义、`FieldPos` 那套搬进去）；`mask/csv.go` 的 `newCSVReader` 删掉，`mask/inline.go:59` `singleRecord` 改调 `criterion`
- [x] `criterion` 加 `LooksLikeInlineNodeRecord(raw string) bool`（恰好一条、首列是 IPv4，照老版签名与语义）；`parse/nodelist.go:173` 与 `mask/inline.go:44` 都改调它，段号那一半留在各自包
- [x] `mask/csv.go:35` `fieldSpans` **不动**：它是清单副本的就地替换（要保留 BOM、引号、空白的原始字节），不是读法，也不是两个包共用的判据
- [x] 用例：`criterion` 侧拿 `audit.md` §三 那四条畸形输入钉住「严格读不出」；`mask` 侧一条（原样返回）、`parse` 侧一条（拒绝），同一组输入
- [x] 清单那条链的既有用例全绿（`parse/nodelist_test.go`、`mask/mask_test.go`、`mask/manifest_test.go`）；`go test ./...` 全绿

## Comments

- 2026-11-13：`criterion/csv.go` 一份读法（`ReadCSVRows(text, comment)`，回 `rows` 与行号）＋ `LooksLikeInlineNodeRecord`。`parse` 的 `csvRows`、`looksLikeInlineNode` 删除；`mask` 的 `newCSVReader` 删除、`singleRecord` 改调 `criterion`，`looksLikeInlineList` 先问 `criterion.LooksLikeInlineNodeRecord`、再取段号。
- **`LazyQuotes` 去掉了**：`audit.md` §三·1 那四条畸形输入现在两处都读不出（`criterion/csv_test.go` 钉住）。原先担心的「被拒的值带明文进日志」收口时核过：argv 只经 `mask.CommandLine` 进 `report.txt`，而报告只在真跑完之后写，拒绝与起不来不写报告——到不了日志。已在 `audit.md` §五 改正。
- `fieldSpans` 与 `joinCSV` 留在 `mask`：前者要保留原始字节（清单副本就地替换），后者是写回，不是读法。
- 验：`go test ./...` 全绿；`parse/nodelist_test.go`、`mask` 两个包既有用例一条没改。
