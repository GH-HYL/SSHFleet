# 06: 语言开关那笔账对清

**要做的**：做完之后，代码里不再有任何「随阶段 08」的悬案，`[locale] english` 那一格只剩事实：它只负责下发行里那句把远端提示语钉成英文的片段，另加代填值含中文那道门按同一格判；判据表只有一份、中英关键词同表，与语言无关。另用一次 A/B 把 `english = false` 那一档的下发行钉住。

**Blocked by**：04。

**Status**：done

- [x] `grep -rn "阶段 08" internal/` 一处不剩；`contract` 与 `credential` 里那两句改成事实
- [x] 四个落点全清：`contract/plan.go`（English 那句失实注释）、`credential/credential.go`（「三条命令不在本包」）、`message/screens.go`（四行过渡话连同 `NotWiredKeyTools`／`notWiredHead` 整块删）、`parse/argv.go`（「本阶段不接」那段）
- [x] `english = true` 与 `false` 各跑一次命令模式：下发行差且只差 `export LC_MESSAGES=C` 那一段
- [x] 代填值含中文、`english = true` 那道门照旧拦（只验没被碰坏，05 已落）
- [x] `-h` 里密钥管理那三行与前五组一字不动（阶段 02 已对上，本阶段没碰帮助）

## Comments

**A/B**（`t4.sh`）：真连本机 sshd 跑一条 `whoami`，两档各一次。`english = true` 那侧的下发行里有 `export LC_MESSAGES=C;`，`false` 那侧没有，其余一字不差；取的是执行期日志里「命令行」那一行（两份日志逐字比）。这一格本来就实现完了，本阶段一行代码没动它，只把注释改到与事实一致。

**「判据表按语言选哪一套」是失实注释**：`keywords_error.conf`、`keywords_passwd.conf` 都是中英关键词同表，匹配与语言无关；`english` 只影响远端的提示语说什么。那句已改成「只负责下发行那一句，另加代填值含中文那道门由闸门按同一格判」。
