# 实测报告：阶段 09 的两版对照

**这一份对的是哪两版**：新工程与老工程同一个提交 `e943029c1727`（`modules/SSHFleet2_Go` 与 `modules/SSHFleet_Go` 并排在同一仓库），两侧都用 `go1.26.10 linux/amd64` 在 WSL 里编出来，二进制同名 `sshfleet`。

**怎么复跑**（沙盒 `build/建站09_AB_2026-11-06_1535/`，不进仓库）：

```
wsl -d Ubuntu-26.04 -- su - ubuntu -c 'bash <沙盒>/run.sh'          # 不碰机器的场面
wsl -d Ubuntu-26.04 -- su - ubuntu -c 'bash <沙盒>/run.sh --all'    # 连要机器的也跑
```

`run.sh` 读 `scenes.tsv`（由 `gen_scenes.py` 生成，字段用 `\x1f` 分隔），每场两侧先复位再跑，`cmp.py` 归一化后比屏幕、退出码、`report.txt`、`terminal-output.txt`、`SSHFleetExec.log`、两张表、`assets/`、`latest_history`。

**归一化只动**：两种时间戳写法、两种耗时写法、沙盒路径与两侧目录名／各自 HOME、归档目录名里的秒级时间戳、xlsx 的 zip 时间戳与列宽。

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
| d1-key-empty | D 密钥 | 一致（逐字节） |

其余场次的结论由 07、08、09 号单跑出来之后逐行补进本表。
