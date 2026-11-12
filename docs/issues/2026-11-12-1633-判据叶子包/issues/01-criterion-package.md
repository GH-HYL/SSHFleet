# 01: 建判据叶子包，搬合法 IPv4 与剥系统前缀

**要做的**：建第十个横切叶子包 `criterion`（只依赖标准库），把「是不是合法 IPv4」与「剥掉系统前缀」各收成一份。做完之后，解析站与 `mask` 认的 IP 是同一把尺子，`message` 与凭据包剥前缀是同一把尺子；`010.0.0.1` 在清单文件与内联记录两条路上都被判非法；两条判据的边界由测试钉住（跟着包走，自定义区——红线区不碰值断言），谁放宽跑测试就红。

**Blocked by**：无，可立即开工。

**Status**：done

- [x] `go test ./...` 全绿；`TestProjectIsClean` 零违规，包数为十九（规则表加 `criterion` 那一行、塔尖依赖清单加一个名字、`contractAndCrosscut` 加一个名字）
- [x] 全工程 `func isIPv4` 只剩 `criterion` 一处，解析站三处、`mask` 两处都改成调用它
- [x] 全工程 `func SystemReason` 只剩 `criterion` 一处，`message`、凭据包、`keytool` 的调用点都改成调用它
- [x] `criterion` 包内两条测试：`TestIsIPv4`（前导零、超界、段数不足、IPv6、带掩码、空、前后空格、正常值）与 `TestSystemReason`（`*fs.PathError`、`*os.LinkError`、普通错误）
- [x] 反向验证各做一次：放宽 `IsIPv4`、删掉 `LinkError` 一支，两条各红一条；改回即绿（记录进 `## Comments`）
- [x] `010.0.0.1` 与 `256.0.0.1` 分别走 `-f` 清单与内联记录两条路，都不被当成合法节点

## Comments

- 2026-11-12：`go test ./...` 16 包全绿（`criterion` 是新出现的第 16 个），`TestProjectIsClean` 零违规、规则表 19 行。
- 2026-11-12：反向验证两条都做了——`IsIPv4` 放宽成「含点即真」，`TestIsIPv4` 报出前导零、超界、段数、掩码共 7 条；删掉 `LinkError` 一支，`TestSystemReason` 报「链接类前缀没剥掉：got rename a b: permission denied，want permission denied」。改回即绿。
- 2026-11-12：真机进程面（`build/criterion_2026-11-12_1640/` 沙盒）：`010.0.0.1` 走 `-f` 文件路报「CSV 中未解析出任何有效节点」；走内联路不进内联分支（现报「文件读不到」——这正是交接件 §七.2 那句报错文案没落的地方，另案）；`10.0.0.1` 走过解析、进凭据预检。`256.0.0.1` 由单测覆盖（`10.0.0.256` 同类）。
- 2026-11-12：`SystemReason` 收拢时凭据侧多出 `os.LinkError` 一支（向老版 `common.SystemReason` 对齐）；该支在现有 37 场 A/B 不覆盖，属已知增量（ADR-0051 影响）。

