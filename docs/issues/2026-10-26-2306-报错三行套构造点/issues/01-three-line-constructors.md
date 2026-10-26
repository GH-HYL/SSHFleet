# 01: 28 处三行套改走 common 构造点

**要做的**：`internal/common` 新增 `ErrUser(problem, reason, hint)` 与 `ErrUserShort(problem, hint)` 两个构造点，然后把现状 28 处手写的 `\n原因：…\n提示：…` 逐一改走它们。**第一行的拼法原样保留，本条不改任何一句文案**——做完之后，`internal/` 下除构造点外 grep 不到 `\n原因：` 字面量。例外是两条只有一行的（`--change-password 的值有问题`、`值读不出新密码`）：迁成 `ErrUserShort`，缺的那句「提示」在实施时起草、写进 Comments 给作者过目。

Status: ready-for-agent
Blocked by: 无（可立即开工）

- [ ] `internal/common` 两个构造点就位，注释写清「有系统原文用哪个、纯形态判定用哪个」
- [ ] 28 处全部迁移，迁移前后报错文本逐条一致（两平台 `go test ./...` 全量兜）
- [ ] `grep -rn '\\n原因：' internal/ | grep -v common` 无命中
- [ ] `--change-password` 那两条一行报错补上「提示」，措辞进 Comments 过目
- [ ] 既有断言零改动（若有一条非改不可，停下来在 Comments 里说清为什么）
- [ ] 两平台 `go test ./...` 全绿

## Comments

- 2026-10-26：建立。来源 `2026-10-21-1413-命令行路径规范化` 议题回顾审计的判断题 D26；同议题那两条漏网的（`--change-password 的值有问题`、`值读不出新密码` 只有一行）是「缺构造点」最直接的受害者，随本条一并补齐。
- 28 处的清单以实施当天的 `grep -rn '\\n原因：' internal/ --include=*.go | grep -v _test` 为准（2026-10-26 计数 28，分布：batch 2、cli 8、config 3、credential 4、dangercheck 3、output 2、ssh 1、verdict 2、common 除外）。
