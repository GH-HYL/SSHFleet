# 04: dangercheck 4 处、verdict 2 处迁到构造点

**要做的**：危险命令规则文件与错误分类关键词文件的加载报错 6 处改走构造点。做完之后：这两个文件出问题时报错走构造点，输出逐字节不变。规则文件里另一套「出处：/出路：」标签**不在此列**，原样保留。

Status: ready-for-agent
Blocked by: 01

- [ ] 两包内门无命中：`grep -rnE --include='*.go' '\\n原因：|"[[:space:]]*原因：' internal/dangercheck/ internal/verdict/ | grep -v _test` 输出为空
- [ ] 报错文本逐字节不变（既有断言一条未改、全绿）
- [ ] 「出处：/出路：」那套标签与文案一字未动（这批不是三行套，属另一套口径）
- [ ] `go test ./internal/dangercheck ./internal/verdict` 全绿

## Comments

- 2026-10-27：按 ADR-0028 建立。这两个包的规则文件报错另有「出处/出路」一族，收不收编是另一个议题，本工单只动带「原因：」的那 6 处。
