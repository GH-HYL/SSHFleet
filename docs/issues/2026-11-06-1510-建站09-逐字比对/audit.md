# 实测报告：阶段 09 的两版对照

**这一份对的是哪两版**：老工程 `modules/SSHFleet_Go` 自 `e943029c1727` 未动；新工程 `modules/SSHFleet2_Go` 为本文件所在提交的前一笔 `addb301`（07 号单收口时那次端口修正之后）。两侧都用 `go1.26.10 linux/amd64` 在 WSL 里编出来，二进制同名 `sshfleet`。

**怎么复跑**（沙盒 `build/建站09_AB_2026-11-06_1535/`，不进仓库）：

```
wsl -d Ubuntu-26.04 -- su - ubuntu -c 'bash <沙盒>/run.sh'          # 不碰机器的场面
wsl -d Ubuntu-26.04 -- su - ubuntu -c 'bash <沙盒>/run.sh --all'    # 连要机器的也跑
```

`run.sh` 读 `scenes.tsv`（由 `gen_scenes.py` 生成，字段用 `\x1f` 分隔），每场两侧先复位再跑，`cmp.py` 归一化后比屏幕、退出码、`report.txt`、`terminal-output.txt`、`SSHFleetExec.log`、两张表、`assets/`、`latest_history`。

**归一化只动**：两种时间戳写法、两种耗时写法（含两张表里的耗时——`results.xlsx` 的 G、H 两列数值格，`terminal-output.xlsx` 详情里那句 `0.0xxs`）、沙盒路径与两侧目录名／各自 HOME、归档目录名里的秒级时间戳、xlsx 的 zip 时间戳与列宽。

**自检**：把 a1 的屏幕多打一行，`cmp.py` 报「差异：屏幕」且退出码 1；改回后退出码 0。表里的「一致」不是白跑的。

## 逐场结果

| 场景 | 组 | 结论 |
| --- | --- | --- |
| a1-help | A 哨兵 | 一致（逐字节） |
| a2-noargs | A 哨兵 | 一致（逐字节） |
| a3-unknown-option | A 哨兵 | 一致（逐字节） |
| a4-missing-value | A 哨兵 | 一致（逐字节） |
| a5-two-keycmds | A 哨兵 | 一致（逐字节） |
| a6-key-extra | A 哨兵 | 一致（逐字节） |
| a7-key-with-batch | A 哨兵 | 一致（逐字节）——「同时给了 -c、-f、-r」那串两面一字不差 |
| b1-command | B 五模式 | 一致（逐字节），退出码两侧同为 0 |
| b2-command-fail | B 五模式 | 一致（逐字节），退出码两侧同为 3 |
| b3-unreachable | B 五模式 | 一致（逐字节），退出码两侧同为 1 |
| b4-script | B 五模式 | 一致（逐字节） |
| b5-script-answer | B 五模式 | 一致（逐字节）——代填两问的答案与顺序都对上 |
| b6-script-fallback | B 五模式 | 一致（逐字节）——`.xyz` 回退非 bash 那条路 |
| b7-upload | B 五模式 | 一致（逐字节），`assets/` 与上传落地都对上 |
| b8-upload-collide | B 五模式 | 一致（逐字节）——撞名且含软链接那一场 |
| b9-download | B 五模式 | 一致（逐字节）——含软链接 |
| b10-passwd-expired | B 五模式 | 一致（逐字节）——过期账号走改密 |
| b11-passwd-valid | B 五模式 | 一致（逐字节）——未过期账号走改密 |
| b12-inline-f | B 五模式 | 差异＝已知差异第一条（遮法）：确认屏与 `report.txt` 各一处，新版前 2 后 2、老版整段 `****`；工具日志里没有那条命令行，无第三处露更多。口令明文在三份产出里一处都没有 |
| b13-two-nodes | B 五模式 | 一致（逐字节），两台一成一败：统计块「成功 1 失败 1」与退出码两侧相同 |
| d1-key-empty | D 密钥 | 一致（逐字节） |

**版本行**：两版都是 `v1.0.0`，后面 `+e943029-dirty (…)` 是构建标识，按口径只比前缀。

其余场次的结论由 07、08、09 号单跑出来之后逐行补进本表。

## 07 号单记下的几笔

| 什么 | 处置 |
| --- | --- |
| **端口那一格**：老版写数字，新版写文本 | 真差异，改新工程（`addb301`）。文本格在 Excel 里按字符串排序、左对齐，使用者按端口筛会看出来。端口在纸上本就是十进制串，落格时还原成数字；解析不出来的仍按原文写，不吞成 0 |
| `b1-command` 原来没有 `--yes`，两侧都停在确认屏、答「输入结束，操作已取消」 | 场景表错，不是代码差异。补上 `--yes` 后才是「命令成功」那一场 |
| `b12-inline-f` 原来挂 `--yes`，确认屏根本没露出来 | 改成 stdin 答 `y` 走确认屏——已知差异第一条要看的三处里，确认屏是其中一处 |
| 多节点那一场（两台一成一败）在场景表里没有 | 补 `b13-two-nodes`（`two.csv`：2201 通、2299 不通）。票面写「十二场」，这一条是单列的验收 |
| `b13` 最初用 `uptime` | 换成 `whoami`：`uptime` 输出自带墙上时刻，两侧必然差一秒，那不是行为差异 |
| 归一化多了一样 | 两张表里的耗时（`results.xlsx` G、H 两列数值格、`terminal-output.xlsx` 详情里那句 `0.0xxs`）。归到规格第二节「耗时的两种写法」，不是新开一类 |

改完重跑十三场：除 `b12-inline-f` 归到已知差异第一条，其余十二场屏幕、退出码、`report.txt`、`terminal-output.txt`、`SSHFleetExec.log`、两张表、`assets/`、`latest_history` 全一致。

## 认下的重复（不合并，各有边界）

| 这串 | 在哪两处 | 为什么不合并 |
| --- | --- | --- |
| `./config/SSHFleet.conf` | 解析站（读整份配置）、密钥包（只读 `[credential] encrypt` 一格） | 两边不许互相 import；密钥包只读自己那一小格、不引用别站（开发者 2026-11-06 定） |
| `SystemReason` | `message`、凭据包 | 横切包之间只许三条依赖边（ADR-0032 决定四），合并要开第四条 |
| 注册表位置 | `message` 的「写入本机（HKCU\\Environment）」、凭据包的「注册表 HKCU\\Environment」 | 核下来不是同一串：一个是括号里的位置本身、一个是提示行里的完整称呼；合并会改屏上的字 |
| 凭据包那几句中文 | `credential/words.go` | 「字只从 `message` 出」的唯一口子（ADR-0032 第四条 2026-11-06 更新记录）；屏上外壳仍归 `message` |

各自都在代码里留了一句「另一处也有一份，改这里要一起改」。
