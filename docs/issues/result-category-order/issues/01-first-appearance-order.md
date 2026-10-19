# 01: 失败分类次序按「首次出现」定死（统计块与归档报告两处）

**要做的**：同一批节点跑两次，终端统计块的「失败分类统计」与归档报告 `report.txt` 的「节点清单统计」段里，分类的先后完全一致。数量不同的按台数排（终端降序、报告升序，与现在一致）；**数量相同时按该类在清单里第一次出现的先后**排——这条次序两次运行都相同，不再是碰运气。顺带把这条判据的两处出口分开：报告侧不再自己去遍历分类图取键，改用统计已排好的次序。

Status: done（2026-10-19 实施并提交 beefb0a）
Blocked by: 无

- [x] 复现先跑过一次：`GOTMPDIR=$PWD/../../build/gotmp-1019 TMP=$GOTMPDIR` 下连跑 `go test -count=1 -run TestStatisticsFailCategoriesSortedByCountDesc ./internal/result/`，见到 FAIL（改前 43 跑 6 红，约 14%）；报告侧用沙盒 `build/atest-9/` 里的桩见「同数量分类出现 3 种排列」
- [x] 统计侧：`SortedFailCategories` 的汇集次序改为分类首次出现的先后，`sort.SliceStable` 及其后逻辑不动
- [x] 统计侧：分类首现序记在扫描 items 的那一趟里（与该分类的计数、退出码同一处来源），不另起一趟扫描——`categoryOrder` 与 `counts[category]++` 同处，顺序为「先记首现、再计数」
- [x] 成功分类依旧不进失败分类表、也不留在分类 IP 图里（`TestStatisticsSuccessLinesUseModeCategory` 保持绿）
- [x] 报告侧：「节点清单统计」段的分类键改为复用已排好的失败分类次序，不再遍历分类图取键
- [x] 报告侧：报告要的是按 IP 数量**升序**（对位旧报告），这条不变；同数量时的先后接住上游那层首现序
- [x] 报告侧：新增测试，接缝取导出函数 `WriteReport`（归档目录用 `t.TempDir()`），断言报告里分类段落的先后是确定的字面量次序
- [x] 报告侧：同一测试里断言**键集一致**——统计给出的失败分类与报告里出现的分类集合相同（钉住「报告侧复用统计次序」所依赖的不变量：两个来源都已剔除成功分类）
- [x] 测试不测内部实现：不读私有字段、不断言内部调用次数；报告侧断言的是文件内容
- [x] 验收：统计侧那条既有测试**连跑 20 次全绿**——实跑 20 次 0 红
- [x] 验收：报告侧新测试 `-count=20` 连跑稳定（3 条测试 ×20 轮全绿）；另跑全量 `go test -count=1 ./...` 与 `-count=3 ./...` 无红
- [x] 已扫过全仓其余产出次序的地方（结果流水线 `sortBySeq`、xlsx、分类 IP 值序、退出码提示、env 拼装），确认无同类问题；范围外的 map 遍历（配置校验报错先报哪条）只报告不改
- [x] CHANGELOG 当天段落记下这条修正，按最终形态写（同数量分类的次序固定为首现序），不写沿革
- [x] spec 与工单的 `Status:` 落回实际状态

## Comments

- 2026-10-19：实现完成。改动落在两个包、各一处：`internal/result/statistics.go`（汇集次序改按首现序）、`internal/output/report.go`（取键改用统计已排好的次序），另新增 `internal/output/report_test.go`。
- **复现/回归**（工单第 1 条）：
  ```bash
  cd modules/SSHFleet_Go
  export GOTMPDIR=$PWD/../../build/gotmp-1019 TMP=$GOTMPDIR   # 临时目录留在工作区内
  go test -count=1 -run TestStatisticsFailCategoriesSortedByCountDesc ./internal/result/
  ```
  改前 43 跑 6 红（约 14%）；改后 20 跑 0 红。
- **新测试抓得住原缺陷**：把 `report.go` 还原成改前版本（`git show HEAD:…`）后跑新测试，`TestWriteReportCategoryOrderStableAcrossWrites` 在第 6 次写入时红——
  `第 6 次写入的次序变了：[拒绝网络连接 原因未知 连接超时]，首次是 [连接超时 拒绝网络连接 原因未知]`。
- **端到端实测**（沙盒 `build/atest-9/`，清单里两两同台数）：清单序「拒绝网络连接的地址在前」连跑 12 次，统计块 12 次全是 `拒绝网络连接 ×2  连接超时 ×2`；把清单反序后连跑 12 次，12 次全是 `连接超时 ×2  拒绝网络连接 ×2`。**次序严格跟随清单首现序，不是恰好固定成字母序**——这一条排除了「碰巧稳定成另一种确定序」的误判。归档 `report.txt` 的「节点清单统计」段同一次运行里也是同序（`连接超时（2）` 段在 `拒绝网络连接（2）` 段之前）。
- **报告侧判据与终端不同、未混为一谈**：终端按台数**降序**、报告按台数**升序**（对位旧报告），本次只改「同数量时谁在前」，两个方向都保持原样。
- **未新增导出字段**：`Stats` 结构未动，报告侧复用 `SortedFailCategories` 的次序（Q1 选定方案 A），「两者键集相同」由 `TestWriteReportCategorySetMatchesStats` 钉住。
- **越界以外的发现（只报告）**：工作区里 `.dsh/skills/pick-and-explain/` 于 11:07 出现，不在本轮改动范围内，未动。
