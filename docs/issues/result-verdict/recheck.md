# 结果统一 · 真机复跑比对表（交给审计）

- 日期：2026-09-29
- 实施提交：`fe13bcb`（ADR 入档）→ `af3a97b`（结果统一重构主体）→ `ee103eb`（D23 单独一笔）
- 沙盒：`build/tmp/recheck/`（新建，与基线沙盒 `build/tmp/baseline/` 分开；两份 `history/` 都保留到审计结束）
- 测试机：WSL2 `172.28.118.49`（Ubuntu 26.04 / OpenSSH 10.2p1），账号 sftest_a / sftest_b（`chage -d 0`）/ sftest_c

## 一、七场复跑 vs 基线（spec 9.2）

| 场 | 基线分类名 | 本次分类名 | 一致 | 配套观察 |
| --- | --- | --- | --- | --- |
| 0 对照 | 执行成功 | **执行成功** | ✔ | 成功 1；exec log 只一行「成功：连接…」；xlsx 报错原文列为空（无 MOTD，D34 ✔） |
| 1 密码过期 | 密码过期 | **密码过期** | ✔ | 失败 1；xlsx 退出码列 **1**（定论退出码照旧保留）；log「命令执行失败，退出码 1」 |
| 2 代填未命中 | 触发词未命中 | **触发词未命中** | ✔ | 失败 1；执行耗时 6.07s（与基线一致）；**退出码列 0 而计入失败**（9.5 #3 / D1 活证 ✔）；log「命令执行失败，退出码 0」 |
| 3 上传部分失败 | 部分成功 | **部分成功** | ✔ | 失败 1；log「上传完成：成功 2/3 个文件（有失败项 1 个）」；CommandExitCode 空、Steps 空 |
| 4 改密未过期 | 密码未过期 | **密码未过期** | ✔ | 失败 1；log「命令执行失败，退出码 无」；`:` 进 Steps（代码落地，D33） |
| 5a 拨号失败 | 连接超时 | **连接超时** | ✔ | 失败 1；`dial tcp … i/o timeout` |
| 5b 认证失败 | 身份验证失败 | **身份验证失败** | ✔ | 失败 1；`unable to authenticate, attempted methods [none password]` |

**结论：七场分类名与基线逐场一致，无未注明的变动。**

三处对齐（9.4）：每场的 终端统计块 / `report.txt`【结果统计】/ `results.xlsx` 与 `terminal-output.xlsx` 分类列 / `SSHFleetExec.log` 分类行，已逐场核对一致。

## 二、专项确认项（spec 9.5）

| # | 项 | 结果 |
| --- | --- | --- |
| 1 | D27：两侧「远程路径不存在」 | ✔ 上传到不存在目录 → 远程路径不存在；下载不存在路径 → 远程路径不存在（归档 `…_upload_专项1上` / `…_download_专项1下`） |
| 2 | D17/D33：CommandExitCode 是选组依据 | ✔ 场 1（=1 → 第二组 → 密码过期）与场 2（=0 但一级定性）分叉成立；语料测试 `TestJudgeStructuralFacts` 钉住（抹掉 CommandExitCode 场 1 即掉第一组） |
| 3 | D1：成败与退出码解绑 | ✔ 场 2 退出码 0 计入失败，四处（终端/报告/两 xlsx/日志）一致 |
| 4 | D34：banner 独立字段后呈现无变化 | ✔ 场 5c 报错原文列 = 原文 + banner 合成（`建立连接失败 - …EOF\nYour account has expired; …`）；场 0 正常行无 banner；分类「账号过期」命中 |
| 5 | 无回归 | ✔ 七场 + 专项全过；单测全绿（`go test ./...`），语料 30/30 |

## 三、有意的用户可见变化（无未注明变动）

1. **「退出码」列语义**：现在是判定给的定论退出码——代填未命中显示 0（此前被抹成空）；传输/改密/预检失败保持空。这是本轮的目的本身（D1/D4）。
2. **失败行「报错原文」不再携带结论文案**：「触发词未命中：第 1 条（…）」「密码未过期，跳过改密」两句话不再写进报错原文——这些信息由「分类」列表达（spec §六"不再拼文案"）。诊断细节（未命中第几条、触发词原文）现在只在事实字段里，未在界面上展示——**如果需要，可以后续把 AnswersMissed 渲染回报错原文列，等你裁定**。
3. 其余（成败口径、分类名、banner 呈现）与基线一致。

## 四、W1：§三 新增字段逐条核对

| 字段 | 落地 | 真机验证 |
| --- | --- | --- |
| CommandExitCode | exec/interactive 写；sftp/passwd 恒空 | 场 0/1/2（值对），场 3/4（空）✔ |
| Steps | test -e / find / `:` 三条 | 专项 1 下载侧走第一组 ✔（test -e 路径）；其余代码路径有语料钉 |
| TimedOut / Canceled | 超时/中断写 | 本轮七场未造；语料与成功规则引用，纸面走查覆盖 |
| SessionBegun | 四会话路径写 | 场 0/1/2/4 判定依赖它 ✔ |
| ServerBanner | 无条件写 | 场 5c ✔、场 0 无 ✔ |
| AnswersMissed / AbortLine / AnswersExhausted | 代填写 | 场 2 ✔；中止词/表已空超时未造（语料钉） |
| EnterSignalSeen / PasswdEarlyClose | 改密写 | 场 4 ✔；早收场未造（语料钉） |

未造的场（超时、中断、中止词、早收场）分类路径均被 30 条语料 + `TestJudgeStructuralFacts` 覆盖；需要真机补造随时可补。

## 五、留给审计的其它事项

- **沙盒口令**：`build/tmp/recheck/list_*.csv` 内含本轮自定的测试口令（未写入任何文档）；审计结束后连 `build/tmp/recheck/` 一起删。
- 机器现状：sftest_b 保持密码过期（场 1 夹具）；`/home/sftest_a/up/` 本轮跑后有 a.txt/b.txt 残留（复跑场 3 前要清）；sftest_a 已复原（chage -E -1）。
- baseline 沙盒未动。
- W4 命名：包名 `verdict`、入口 `Judge`、字段名按 spec §三 字面落地（`StepResult{Name, ExitCode}`），W10 的"以后还会有几条"未定——现状三条。
