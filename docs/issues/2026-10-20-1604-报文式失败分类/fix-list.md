# 报文分类议题 · 遗留问题清单

Status: A 类已修（2026-10-20），B 类已按作者裁定落地

> 本文件只记**审计发现的问题**与**待裁定项**，已确认无问题的内容不记。
> 基准：`cfb7ecc...HEAD`，以 ADR-0021 / ADR-0022 的「决定」为纲。

## A. 自决项（已修）

| # | 问题 | 修法 | 验证 |
| --- | --- | --- | --- |
| A1 | **取消时已完成结果被随机丢弃**：回滚执行了，统计却报「未执行」 | `internal/batch/pool.go`：去掉 `select { case resCh <- res: case <-ctx.Done(): }`，结果一律投递（通道容量=任务数，不会阻塞）；注释改成「取消不等于没执行」 | 真机 3 次 Ctrl+C：3/3 显示「任务已取消」，远端无残留（`build/repro-2/out_s7b.stdout`） |
| A2 | **逐文件失败原因仍带中文结论进报错原文**，两处连报文都没有 → 未归类 | `internal/ssh/sftp.go:122/135/161`：报错原文格改为报文（`remote_exists` / `local_unreadable` / 上传函数自带的报文，兜底 `remote_file_failed`）；「上传失败」只留在输出明细行 | `go test ./internal/ssh/` 通过；`sftp.go` 三处均为报文 |
| A3 | **spec 计数与退役口径未回填** | spec.md：补「实施终值」段（报文 40、语料 50、断言以 audit 为准）；三处 `finished` 改为退役口径；预算表换成现行 15 条 transfer 环节；格式落点写实（`internal/ssh`）；audit.md §九/§十四回填终值 | 文档自查，`grep finished` 仅剩退役注记 |
| A4 | **工单写 `file:line`、引用已删 README 成死链** | 不改历史正文；以本文件与 `audit.md` §十四 为准，定位以当前代码为准 | 记录在案 |
| A5 | **语料缺回滚两条与代填中止** | 补 3 条：`rolled_back`、`rollback_partial`（同格带 `permission denied`，锁次序）、`abort_line` 密码过期；语料结构补 `abort_line` 字段 | `go test ./internal/verdict/` 通过（50 条） |
| A6 | 半成品删除失败被静默吞掉；sudo 跨 fs `mv` 中途失败可留半成品 | **已按工单 09 实现**：上传/下载函数把残留路径带出来，新增 `partial_left` / `local_partial_left` 两条报文；`mv` 失败先 `Stat` 确认存在才报，不误报 | 真机正常上传回归 ✓；报文单测 + 语料 ✓ |
| A7 | `OpenChannelError` 未按 `Reason` 细分 | **核实为误报**：`Error()` 已带 Reason 原文，调用点已按位置选对环节名，信息没丢 | 不改 |

## B. 待裁定项（已定，已落地）

| # | 问题 | 作者裁定 | 落地 |
| --- | --- | --- | --- |
| B1 | 回滚分类给「原因」还是「状态」 | **状态优先** | `keywords_error.conf` 把「上传回滚未清干净」「上传失败已回滚」移到第一组最前；语言与 CHANGELOG 一致。真机：只读目录触发回滚 → 「上传失败已回滚」✓ |
| B2 | 取消时结果语义 | **要，且保留原分类** | 同 A1；真机 3/3「任务已取消」 |
| B3 | 6 条 connect 报文无认领 | **能预知会落到未分类的，现在就补** | 6 条全部补直接关键词：`no_route` / `dial_failed` → 网络不可达，`timeout` → 连接超时，`auth_failed` → 身份验证失败；`handshake_failed` 新增兜底分类「握手失败」（排所有具体条目之后）。`dns_failed` 后来按「全部是 IP」删掉了 |
| B4 | `code_defined` 痕迹 | **按 spec 删掉痕迹** | CHANGELOG 11.0.0 的 Removed 段删除，不留字段名；启动期仍按未知字段拦下 |
| B5 | 报文格式是否迁独立包 | 暂不迁，记为后续优化 | 未动 |

## C. 已按裁定处理

1. **半成品残留**（A6）→ 工单 09 已实现并验证。
2. **nologin 分类**→ `docs/5-常见问题.md` §2 补了一条问答，说明「登录被禁止」与「无法建立会话」的区别。
3. **DNS**→ 作者定「全部是 IP」：删除 `dns_failed` 分支、环节常量、判据表条目与 `DNS解析失败` 分类；`no such host` 现按 `dial_failed` 报（`网络不可达`）。
4. **非 TTY 下上传明细不上屏**：既有行为，与本次报文体系无关，未动。

## 已核实无问题（不再列项）

- `Unclassified = "原因未知"` 属兜底语义延伸，不是第二处「代码给名」。
- 报文定义落 `internal/ssh` 的依赖理由属实，无环。
- 追加不覆盖、原文在前报文在后、无译文层、并发数全模式显示——均落实。
- 全量测试 12 包 ok、3 包无测试文件（跑法：`GOTMPDIR` 压进 `build/`，否则 360 静默删测试二进制）。
