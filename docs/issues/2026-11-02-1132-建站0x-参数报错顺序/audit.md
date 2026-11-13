# 实测：检查与提示的先后（2026-11-13）

**用途**：ADR-0052 与 `spec.md` 里那些数字的事实底本。做判断时看这一份，不要只看结论。

## 方法与边界

- **机器/形态**：Windows 11 + Git Bash；Go 1.26.10（`C:\Program Files\Go`）。
- **两个二进制**：老版 `modules/SSHFleet_Go` 现编；新版 `modules/SSHFleet2_Go`，HEAD `8927738`。
- **配置**：两版各带自己的 `config/`（两份 `keywords_error.conf` 的段名不同：老版 `[[categories]]`、新版 `[[category]]`，混用会当场读不出分类），主配置取 `SSHFleet-for-windows.conf`，`encrypt` 改成 `false`（否则主密钥警告与凭据预检会盖住要看的先后）。
- **比法**：同一组 argv 分别喂两个二进制，合并 stdout＋stderr，归一化掉程序名、版本行、各自工作目录与主密钥警告后逐字 diff，再看退出码。
- **边界**：Git Bash 会把 `/tmp` 一类绝对路径改写成 `C:/…`，与 Linux 生产形态不同。所以这批只判「谁先说」，不判「那句话长什么样」；受影响的只有含 `-p /tmp` 的那几条。
- **现场**：`build/sb_order/`（`order_ab.py` 是跑表脚本、`ab_report2.txt` 是原始输出、`old/` 与 `new/` 是两个工作目录）。`build/` 不进版本控制，换机器就没了——所以要复现，照上面的方法重搭。

## 总数

**78 场，40 异**。其中 39 条是真差异，另 1 条只是连接耗时不同（`-c "ls" -f ok.csv --yes`）。

## 39 条的分类（每类的输入都在这里，可逐条倒查）

**A. 材料与清单的失败顶掉了闸门（25 条）**——同一个根。输入：`-c " "`、`-c "echo hi" -p /tmp`、`--no-shell -s ./fx/hello.sh`、`--no-shell -u . -p /tmp/`、`-u .`、`-u . -p rel`、`-u . -p /tmp`、`-d /etc/passwd`、`-d rel -p .`、`--change-password pw -p /tmp`、`--change-password pw -a "x"`、`--change-password pw -k`、`-c "rm -rf /"`、`-c "rm -rf /opt" --yes`、`-c "rm -rf /opt"`、`-c "rm -rf //"`、`-c "RM -RF /etc"`、`-c "echo hi" -p /tmp -f /nope.csv`、`-c "echo hi" -p /tmp -f bad.csv`、`-c "echo hi" -p /tmp -f empty.csv`、`-u /nope -f /nope.csv`、`-u /nope -f ok.csv`、`--change-password pw -k -f /nope.csv`、`--change-password " " -f /nope.csv`、`-c "rm -rf /" -f bad.csv`。
**老版**：25 条都报闸门那一句（`-p` 不能配 `-c`、危险框、改密互斥……）。**新版**：25 条都报「清单读不到」或「凭据预检失败」。

**B. 方向相反（2 条）**——`-t abc -c "echo hi" -f /nope.csv`、`-n abc -c "echo hi" -f /nope.csv`。**老版**报清单读不到；**新版**报 `-t`／`-n`。按层序（取值为第三层、材料为第五层），新版对。

**C. 材料族内部互序（2 条）**——`-u /nope -p /tmp/ -f /nope.csv`（老版报 `-u` 源读不到，新版报清单）、`-c "echo hi" -a /nope.csv -f /nope.csv`（老版报 `-a` 读不到，新版报清单）。老版把 `-u` 源与 `-a` 表排在清单之前，新版排在之后。

**D. 提示的位置与静默（6 条）**——`-c "rm -rf /" -f hdr.csv`、`-s ./fx/danger.sh -f hdr.csv`（新版先印「已移除表头」，老版先出危险框）、`-c "rm -rf /opt" -f hdr.csv --yes`、`-c "rm -rf /opt" -f ok.csv --yes`（新版丢「跳过确认」那行）、`-f hdr.csv -c ls --yes`、`-u . -p /tmp/ -f hdr.csv --yes`（新版在 `--yes` 下照印表头提示，老版静默）。

**E. 提示抢在闸门拒绝之前（1 条）**——`-c "ls" -p /tmp -f hdr.csv`：新版先印「已移除表头」再报 `-p`。

**F. 回退问与危险框的顺序（1 条）**——`-s ./fx/danger.xyz -f ok.csv`：老版先问「继续用 bash 吗」，新版先出禁止框。

**G. 分类标签（1 条）**——`-u /nope -p /tmp/ -f ok.csv`：同一句话，老版挂 `[function:cli]`，新版挂 `[function:upload]`。

**H. 措辞（1 条）**——`-c "rm -rf /" -f /nope.csv`：两句都是「清单读不到」，只有第三行提示不同（老版「要临时传一台机器…」，新版「请检查路径是否写对…」）。**这处是新版有意改的**（提交 `e4a372e`，开发者认可），不算回归。

## 定点探头（不进 78 场，单独跑的）

- **密钥命令与批量参数（10 组）**：`--gen-key -c ls`、`--gen-key -f ok.csv`、`--key-status -c whoami`、`--gen-key --zzz`、`--gen-key extra`、`--key-status --gen-key`、`--key-status --gen-key -c x`、`--convert-secret sec.txt -c ls`、`--gen-key --sudo`、`--gen-key -p /tmp`——**两版逐字相同**。主语恒是加密命令那一侧，批量参数只被列出；先后是词法错 ＞ 多余段 ＞ 三条互撞 ＞ 与批量同给。
- **`-r` 影子**：`--gen-key -c ls` 报「同时给了 -c、-r」，`-r` 是工具自己补的自动备注，用户没敲。两版都这样。规格把它列进 Out of Scope，句子归 4.2。
- **规则文件缺一份**（临时移走 `keywords_passwd.conf`）：`-s /nope.sh -f ok.csv` 老版报「规则文件缺失」，新版报脚本读不到；`-c "ls" -p /tmp -f ok.csv` 老版报「规则文件缺失」，新版报「读取改密配置失败」。
- **`-s` 塞命令**：`-s "ls -l"` 两版都是「-s 参数指定的脚本文件读不到：ls -l」，没有方向句。
- **`-c` 塞文件**：`-c ./fx/hello.sh` 两版都在读清单那一步报 `open :`（没有 `-f`）。

## 复跑

`build/sb_order/` 下：`old/` 与 `new/` 各放二进制与自己的 `config/`、夹具；根目录 `python order_ab.py`（Python 3.13）。`build/` 被 `.gitignore` 排除，不在仓库里。
