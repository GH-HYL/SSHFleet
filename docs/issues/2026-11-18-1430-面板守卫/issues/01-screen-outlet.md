# 01: 入口带一格屏幕出口，面板那一环进默认套件

**要做的**：`execute.Run` 增第四个参数 `screen io.Writer`（零值＝生产，流从 `console` 拿、按终端判；递了流就写它并照画）；补一条上传用例，读注入的流，断言传输面板最后一帧写着「1/1 台完成」。做完之后删掉收口那一次回填，默认套件会红。

**Blocked by**：无，可立即开工。

**Status**：done（done）

- [x] `Run` 增一格参数；`newScreen` 收零值时行为与今天逐字相同
- [x] 七个调用点各补 `nil`（塔尖、四处站内用例、两处红线测试）
- [x] 新用例 `transfer_panel_test.go`：按面板标签挑行，判「1/1」
- [x] 反证：注掉 `closeNode` 里的 `ag.onResult(...)`，新用例红；恢复后绿
- [x] `gofmt -l internal/` 空、`go vet ./...` 干净、`go test -count=1 ./...` 全绿

## Comments

- 2026-11-18：**已落地**。两条红线测试只改调用点的实参，引用到的符号与断言数没变，指纹不动（ADR-0037 决定五：新增不算改，真正算改的是删掉、改名、收窄引用面）。
- 反证记录：注掉回填那一行之后 `--- FAIL: TestRunTransferPanelCountsCompletedNodes`，报「传输面板最后一帧没写「1/1 台完成」」；`cp` 回来复跑绿。
- 一处容易写松的地方：命令那一行与传输面板共用一个流，命令那一行也有「已完成 1/1」。断言必须按 `message.NodeProgressLabel()` 挑行，否则缺陷回来它照样绿。
