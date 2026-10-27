# 03: config 6 处、credential 4 处迁到构造点

**要做的**：配置加载（含环境变量段、路径形态、改密提示词表）6 处与凭据读取/转换 4 处改走构造点。做完之后：这两处报错全部走构造点，输出与今天逐字节一致——它们现在多是「问题+原因」两行，**不补提示**，形态保持原样。

Status: done
Blocked by: 01

- [x] 两包内门无命中：`grep -rnE --include='*.go' '\\n原因：|"原因：' internal/config/ internal/credential/ | grep -v _test` 输出为空
- [x] 报错文本逐字节不变（既有断言一条未改、全绿）
- [x] `credential` 里 `--key-status` 那段状态输出**不动**——它不是报错，只是恰好也有一行「原因：」
- [x] 配置里那条路径检查是一个闭包产出多条报错，迁移时保持一处闭包、不拆成多条各拼一遍
- [x] `go test ./internal/config ./internal/credential` 全绿

## Comments

- 2026-10-27：按 ADR-0028 建立。这两包的报错多为「问题+原因」，走「问题/原因」那个名字，不新增提示。
