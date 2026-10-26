# 03: `secret_dir` 只服务清单第 4/6 列，第 5 列原值交给系统

**要做的**：配置里的凭据目录只在一个地方起作用——内联清单与 csv 的第 4、6 列写相对名时，把它补成完整路径（且只在打开加密时，那两格才是文件路径）。此外一律不碰：第 5 列的私钥路径与配置里的凭据路径，原值直接交给系统——照 `--convert-secret` 的做法，不拼、不展开 `~`、不去空白。加密关着时 `secret_dir` 既不参与补齐也不校验，写相对目录不拦人。第 4/6 列写了相对名却没配 `secret_dir` 时，报错要能落到某一行上，并指出去配置文件的 `[credential]` 段配，或那一格改写绝对路径。**凭据读不到时也要给系统原文**：清单那一侧是逐行的汇总，原因放在该行括号里；`--change-password` 读不到密文文件是单值场景，走三行套。两条共用一个错误标签，别把汇总格式拆散。

Status: done
Blocked by: 无（可立即开工）

- [x] 加密开着、第 4 列写 `db_pass.sec`、`secret_dir` 配着、目录里确有该文件：解出来的密码正确——内联（`-f 10.0.0.1,22,root,db_pass.sec`）与 csv 文件各验一次
- [x] 加密开着、第 4 列写相对名、`secret_dir` 为空：报错里同时出现**行号**、`secret_dir`、`[credential]` 三样，并给两条去路（配上它 / 那一格写绝对路径）
- [x] 第 5 列写 `id_rsa`、`secret_dir` 目录里真有 `id_rsa` 而当前目录没有：报错里的路径是 `id_rsa` 本身，**不带 `secret_dir` 前缀**；前缀里点明行号与列名（「密钥文件」）
- [x] 第 5 列写 `~/id.pem`：`~` 不展开，报错里的路径是字面 `~/id.pem`
- [x] 配置里 `credential.key` 写绝对路径照常读通，且不再经过补齐那一趟
- [x] 加密关着、`secret_dir` 写相对路径：`SSHFleet -h` 能正常打印帮助（这条错在配置装载那一关，不需要连任何机器）
- [x] 加密开着、`--change-password nosuch.sec`：报错三行齐，原因行是 `no such file or directory`
- [x] 清单凭据读不到时，汇总仍是逐行列表，系统原文附在该行里（不被改成三行套、也不整段退化成一句）
- [x] 第 6 列取不到口令仍不报错（既有决策，不动）
- [x] 改写断言旧文案的既有用例（`TestReadPrecheckRelativePathNeedsSecretDir` 里「密码文件不存在」「凭据列包含相对路径，但 secret_dir 未配置」那两条断言），不留兼容用例
- [x] 改前改后各跑一次「第 5 列写 `id_rsa` + 配了 `secret_dir`」，两次输出贴进 `## Comments`（改前会拼、改后不拼）
- [x] 全量 `go test ./...` 除开工前就红的那条（清理归 06）外无新增红

## Comments

- 2026-10-26 落地。沙盒两套：`build\win-verify\enc`（加密开、`secret_dir` 配着）与 `enc2`（加密开、`secret_dir` 为空）。
- 第 4 列相对名拼得上：`-f 127.0.0.1,22,wbverify,db_pass.sec` 与 csv 文件各跑一次，都是一次真登录成功（`whoami` 回 `root`，`sudo` 生效）——解出来的明文确实是 `wbverify` 的密码。
- `secret_dir` 为空时那一行的话：`行 1 (IP: 127.0.0.1): 密码文件写的是相对路径，但配置里没配 credential.secret_dir：db_pass.sec（提示：在 config/SSHFleet.conf 的 [credential] 段配上 secret_dir，或这一格改写本机绝对路径）`。
- 第 5 列不拼（A/B）：改前把 `id_rsa` 拼成 `keys\id_rsa` 且**读到了内容**（后续报的是「密钥解析失败」，说明文件已找到）；改后 `行 1 (IP: 127.0.0.1): 密钥文件不存在 → id_rsa（提示：第 5 列不拼 secret_dir，写本机绝对路径最稳妥）`。
- `~/id.pem`：报错里就是字面 `~/id.pem`，没展开。
- 加密关着 + `secret_dir = "keys"`（相对）：`-h` 正常打印帮助。
- `--change-password nosuch.sec`：三行齐，`原因：The system cannot find the file specified.`（Linux 上是 `no such file or directory`）。
- 清单凭据读不到：汇总仍是逐行列表，原文附在该行括号里——`行 1 (IP: …): 密码文件不存在（…） → …\keys\x (The system cannot find the file specified.)`。
- 第 6 列取不到口令仍不报错：实跑一次（第 5 列给一份能过 BEGIN 检查的假 PEM），没进「CSV凭据预检查失败」，直接到执行阶段才报「密钥解析失败」。
- 配置 `credential.key` 不再经过补齐：代码走查（`precheck.go` 里那处调用已删），行为不变（它本来就是绝对路径）。
- 复查又补了两处：① 第 5 列找不到时现在也带系统原文（`密钥文件不存在 → id_rsa (The system cannot find the file specified.)（提示：…）`），`ReadCredentialPEM` 的 `missing` / `read_error` 都改成剥掉路径的那一段；② 「加密关着 + `secret_dir` 写相对路径」与「第 6 列取不到口令仍不报错」补了用例（`config_test.go` 走真装载入口、`nodelist_test.go` 走预检入口），不再只靠手跑。
