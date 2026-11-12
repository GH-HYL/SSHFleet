# A/B 比对车架

同一份输入让老工程与新工程各跑一遍，收屏幕、退出码、归档产出，按字节比。**它是手段，不是目标**——目的是防重构搬错，不是把老版的行为当标准照抄；老版本身有问题、或照抄会破坏新结构时，允许开口不一致，但要在议题里写明理由（见「差异怎么归」）。

原先这套脚本只躺在 `build/` 的沙盒里。`build/` 不进版本控制、按天清理，而这一轮改过 `cmp.py`（两张登记表、面板耗时 `<CLOCK>`、面板帧折最后一帧）与 `ab.sh`（TTY 路径吃引号），改一次丢一次不划算，所以搬到这里。

## 怎么跑

现场（两侧二进制、配置、`home_*`、`out/`）与代码分开放，现场由 `SSHFLEET_AB_SB` 指：

```bash
cd build/<现场目录>                      # 现场：old/ new/ fixtures/ pristine/ 在这里
SSHFLEET_AB_SB="$PWD" ../../tools/ab/build.sh          # 重编两侧（同名 sshfleet）
SSHFLEET_AB_SB="$PWD" ../../tools/ab/run.sh --all       # 跑全部场景
SSHFLEET_AB_SB="$PWD" python3 ../../tools/ab/selftest_cmp.py --real   # 尺子自检
```

不给 `SSHFLEET_AB_SB` 就当场跑（脚本与现场同目录时的老行为，一字不差）。

两个环境变量：

- `SSHFLEET_AB_SB`——现场根。缺省：脚本所在目录。
- `SSHFLEET_AB_OLD_SB`——`selftest_cmp.py --real` 拿真实产物回归时用的 09 旧沙盒。缺省：`<现场>/../建站09_AB_2026-11-06_1535`。

`build.sh` 里 go 与仓库根都从脚本位置算（`<仓库>/tools/ab`），不再写死路径；它假定在 WSL 里跑（`GO=/home/ubuntu/.local/opt/go/bin/go`）。

TTY 那几场只能在 WSL 里跑（Git Bash 没有 `script`）：`a1-help`、`b1-command`、`b10-passwd-expired`。

## 文件分工

| 文件 | 管什么 |
| --- | --- |
| `run.sh` | 入口：读场景表，逐场调 `ab.sh` 与 `cmp.py` |
| `ab.sh` | 单场采集：两侧各跑一遍，收屏幕／退出码／归档目录／HOME |
| `cmp.py` | 尺子：归一化后按字节比，输出「一致」或差异项 |
| `scenes.tsv` | 场景表，字段用 `\x1f` 分隔（制表符会被 `read` 折叠） |
| `gen_scenes.py` | 生成 `scenes.tsv`；改场景表就改这里再跑一遍，别手改表格 |
| `selftest_cmp.py` | 尺子的自检：造样本断言红／绿，`--real` 额外拿真实产物回归 |
| `build.sh` | 按当前代码重编两侧 |
| `reset.sh` | 每场开跑前复位一侧现场 |
| `prep.py` | 改现场配置的一格（`--conf key=value`） |
| `mkbook.py` | 造样本 xlsx／csv |

## 差异怎么归

只许归三处：**真差异**／**已知差异之一**／**环境抖动**（要写取样次数与发生率）。归不进的当场停下报告，不许用「解释得通」放过。

`cmp.py` 里两张登记表（`OLD_ONLY_MARKS`、`NEW_ONLY_MARKS`）不是垃圾桶：每条都要能说出「哪一条差异、谁定的」，加不出就别加。抖动只认「同一个二进制也会差」的。

开口（允许不一致）要写清两件：哪个条件变了、为什么照抄反而更糟。写在对应议题的 `audit.md` 里，不写在这里。
