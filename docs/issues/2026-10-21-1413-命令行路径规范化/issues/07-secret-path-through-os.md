# 07: 清单第 4/6 列的凭据路径以 OS 库为核心

**要做的**：这一列是唯一「程序要替你拼」的地方（`secret_dir` + 相对名），但拼的每一步都要落在 OS 库上，程序自己不做路径解析。分三层：**简单处理**（去首尾空白）程序自己做，不交给 OS；**识别**（是不是绝对路径）交 `filepath.IsAbs`；**拼接**（目录 + 文件名）交 `filepath.Join`，**拼完的完整路径再交回 OS 库解析一次**（`filepath.Clean`），此后只交给 `os.Stat` / `os.ReadFile` 消费，不再被任何字符串加工碰。`~` 展开**取消**——清单第 4/6 列与第 5 列同口径，`~` 是字面字符。

Status: done
Blocked by: 无（可立即开工）

- [x] `ResolveSecretPath` 只剩四步：去首尾空白 → `filepath.IsAbs` → `filepath.Join` → `filepath.Clean`；`expandHomeTilde` 整个删掉，仓库里不再有第二个调用者
- [x] 第 4/6 列写 `~/db_pass.sec`：不再展开，报错里出现的是字面 `~/db_pass.sec`（拼上 `secret_dir` 之后也一样，前缀照拼、`~` 照留）
- [x] 第 4/6 列写绝对路径：`filepath.IsAbs` 为真则原样用，不拼 `secret_dir`
- [x] 第 4/6 列写相对名：`filepath.Join(secret_dir, 名)` 之后 `filepath.Clean`，得到的路径交给 `os.Stat` / `os.ReadFile`
- [x] 第 5 列的行为不变（本来就不拼、不展开）
- [x] 既有用例按新行为改写，不留兼容用例；新增一条「`~` 不展开」的断言
- [x] 手册/配置注释/`GLOSSARY.md` 里凡暗示「清单里可以用 `~`」的说法一律删掉；`3-进阶配置手册.md` 的「不支持 `~`」那条要把清单第 4/6 列也写进去
- [x] 两平台 `go test ./...` 全绿（Windows 本机 + WSL2 Ubuntu）

## Comments

- 2026-10-26 落地。`ResolveSecretPath` 现在四步：`strings.TrimSpace`（简单处理，程序自己做）→ `filepath.IsAbs`（识别）→ `filepath.Join`（拼接）→ `filepath.Clean`（拼完交回 OS 库复解析）。**绝对路径那一支也过 `Clean`**：验收项写的「原样用」指不拼 `secret_dir`，形态仍交回 OS 库规范一次（`/a/../b` 会变 `/b`）——这是「拼完必须复解析」那条原则的延伸，不是漏做。`expandHomeTilde` 整个删掉，全仓库已无调用者。
- 实跑（客户端 Windows，目标机 WSL2 Ubuntu，`secret_dir` 配着且有 `db_pass.sec`）：
  - 第 4 列写 `~/db_pass.sec` → `行 1 (IP: 127.0.0.1): 密码文件不存在（…） → …\enc\keys\~\db_pass.sec (The system cannot find the path specified.)`——`~` 原样留着，没有主目录的痕迹；
  - 第 4 列写 `db_pass.sec` → 拼到 `secret_dir` 下，一次真登录成功（`成功：1`）。
- 新增用例 `TestReadPrecheckSecretColumnKeepsTildeLiteral`：断言报错里保留 `~`、且**不含**主目录（期望值是 `os.UserHomeDir()` 的字面值，不参与被测算法）。既有用例没有一条依赖 `~` 展开，无需改写。
- 文档：ADR-0025 加了决定 9 与更新记录；`3-进阶配置手册.md` 的「不支持 `~`」扩到第 4/6 列；CHANGELOG 12.0.2 那段把「第 5 列写 `~/id_rsa`」改成「第 4、5、6 列写 `~/…`」；spec 追加一句落点。
- 两平台 `go test ./...` 全绿（Windows 本机 + WSL2 Ubuntu），`gofmt` / `go vet` 干净。
