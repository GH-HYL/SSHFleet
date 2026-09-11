# M1 · 数据面问题清单（主配置）

Type: grilling
Status: needs-info

覆盖 `plan-coverage.md` 的 **F3 数据面**中「主配置」一项。规则文件（危险规则 / 错误关键词）转 TOML 已在 M4 范围，此处不展开。

---

## 一、已查明的事实

### 1.1 旧配置模型（pydantic）

```python
class StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid")   # ← 未知字段直接报错，不静默忽略
```

结构：`account` / `execution` / `enable` / `paths{keywords, exe, logs, files}` / `upload`。

**字段必填性（旧模型）**：

| 段 | 必填字段 | 有默认值字段 |
| --- | --- | --- |
| `account` | `port` / `user` / `secret_dir` / `password` | `key=""` / `key_passphrase=""` / `password_security="2"` |
| `execution` | `mode` / `timeout_connect` / `timeout_execute` / `timeout_transfer` | — |
| `enable` | `output_to_xlsx` / `results_to_xlsx` | — |
| `paths.keywords` | `error_keywords` / `dangerous_keywords` | — |
| `paths.exe` | `batch_tool_windows` / `batch_tool_linux` | **随双进程架构消亡** |
| `paths.logs` | `historys` / `tool` / `exec` | 见 01-structure 的 S4 |
| `paths.files` | `asset` / `output` / `output_xlsx` / `report` / `results_xlsx` | — |
| `upload.concurrency_thresholds` | `small_file` / `large_file` / `medium_concurrency` | — |

### 1.2 加载期校验（`load_config`）

| # | 规则 | 行为 |
| --- | --- | --- |
| 1 | 配置文件不存在 | `FileNotFoundError` → 报错退出 |
| 2 | YAML 解析失败 | 报错退出 |
| 3 | `secret_dir` 为空或字面量 `none`（**大小写不敏感**） | 视为"未配置"，置空字符串，**且把处理结果写回配置对象** |
| 4 | `secret_dir` 非空 | 做 `~` 展开 |
| 5 | `password_security` 取值 | 统一 `str()` 后校验只允许 `"1"/"2"/"3"`，否则报错（兼容 YAML 写 `2` 或 `"2"`） |
| 6 | `account.password` 为相对路径但 `secret_dir` 未配置 | 报错 |
| 7 | `account.key` 同上 | 报错 |
| 8 | `account.key_passphrase` 同上 | 报错 |
| 9 | 未知字段 | `extra="forbid"` → 报错 |

### 1.3 凭据路径梯子（`resolve_secret_path`，全工具单一事实来源）

```
输入 → 去空白 → ~ 展开
      ├─ 是绝对路径 → 原样返回
      └─ 否 → secret_dir 空或 "none" → 返回 None（由调用方决定报错文案）
              secret_dir 有效     → path.join(secret_dir, 输入)
```

### 1.4 路径基准 —— 全部相对**当前工作目录**

- `check_files_exist` 里**硬编码** `"src/config/SSHFleet.yaml"`，再与 `os.getcwd()` 拼接
- `paths.keywords.*` 旧值是 `./src/config/error_keywords.yaml`（相对 cwd）
- 这意味着：**工具必须从项目目录启动**，换个目录跑就找不到配置

### 1.5 加载失败的原文案

```
[ERROR] 加载配置文件失败：{路径}
原因：{异常}
请检查该文件是否存在、YAML 格式是否正确后重试
```

---

## 二、问题

### ❓ D1 — TOML schema 逐字段确认

草案见 `spec.md` 附录 A。相比旧版的变化有三处：

| 变化 | 说明 |
| --- | --- |
| 删 `paths.exe` 段 | 双进程消亡，引擎路径配置无意义 |
| `paths.logs` 拆开 | 按 01-structure S4 的裁定，可能只留 `historys` + `tool` + `exec` 三项中的部分 |
| 新增/重命名 | `paths.keywords.*` → `paths.error_keywords` / `paths.dangerous_keywords`（扁平化） |

➡️ 待你对 01-structure 的 S4 一并裁定后定稿。

### ❓ D2 — 未知字段必须报错（`extra="forbid"` 的等价物）

旧版对配置里的**未知字段零容忍**——这是你刻意加的（注释写明"如已移除的 `paths.logs.zip`"）。这条行为必须保住，否则删字段后旧配置会静默跑偏。

Go 侧做法：`BurntSushi/toml` 解码后调 `md.Undecoded()` 拿未识别键，非空即报错。

- (a) 保留零容忍，并**列出具体是哪个键**
- (b) 保留零容忍，但不提高具体键名
- (c) 放宽为告警

➡️ 推荐 **(a)**。`md.Undecoded()` 恰好能给出键名，比 Python 版的报错更具体——属于"优化且不改变功能"，正合 D20。

### ❓ D3 — `password_security` 的类型

旧 YAML 里写 `3`（int），模型声明为 `str` 再 `str()` 兜。TOML 是强类型，得选一个：

- (a) `password_security = 3`（int）
- (b) `password_security = "3"`（string）

➡️ 推荐 **(a) int**，并在加载时校验取值只能是 `1/2/3`。TOML 里写 `"3"` 会别扭；旧版之所以用 str 是 YAML 的弱类型遗留。

### ❓ D4 — `secret_dir` 的字面量 `none` 视为未配置

旧版**大小写不敏感**地把 `none` 当未配置——这是修过一个分叉 bug 后统一的（见 CHANGELOG 待定段）。

- (a) 保留（`none` / `None` / `NONE` 一律视为未配置）
- (b) 取消，让 `none` 就当目录名

➡️ 推荐 **(a)**：保留这个宽容性。TOML 里写 `secret_dir = "none"` 同样会踩这个坑，行为一致最省心。

### ❓ D5 — 配置文件路径与工作目录基准（最需要你拍板的一题）

旧版把配置路径**硬编码**为 `src/config/SSHFleet.yaml`（相对 cwd），因此**必须从项目根目录启动**。重构后工程结构变成 `modules/SSHFleet_Go/`，这个硬编码路径必然失效。

- (a) 配置文件放**可执行文件同级目录**（`./config/SSHFleet.toml`），基准 = 可执行文件所在目录 → 工具可以随便放、随便在哪调用
- (b) 保持相对**当前工作目录**（`./config/SSHFleet.toml`），行为跟旧版一致："在哪跑就在哪找配置"
- (c) 支持 `--config` 指定，默认回落到 (a) 或 (b)

➡️ 推荐 **(a)**。理由：旧版"必须从项目目录启动"是打包形态带来的约束（Py 脚本 + 相对路径），不是设计意图；你实际会把它当命令敲，从任意目录调用是常态。(c) 灵活性最高但多一个选项要维护，且你刚定了"只界定必须的选项"。**注意这会改变可见行为**，所以列出来给你定。

### ❓ D6 — 必填字段是否维持

旧模型里 `account.port` / `user` / `secret_dir` / `password` 与 `execution.*`、`enable.*`、`paths.*` 全是**必填**——配置缺一段就直接报错。

- (a) 维持全必填（缺啥报啥）
- (b) 给合理默认值（如 `port=22`、`user=root`、`mode="sudo"`）
- (c) 只给无歧义的默认值（超时、开关类），账号类保持必填

➡️ 推荐 **(c)**。超时值旧配置里本来就有明确语义（10/60/300），给默认省事；而账号、凭据目录、输出路径这些"因人而异"的字段保持必填，缺了就该报错——否则用默认值连错机器比报错更糟。

### ❓ D7 — 校验失败的文案

旧版三条专属文案（`account.password` / `account.key` / `account.key_passphrase` 相对路径无 `secret_dir`）各不相同，是针对字段写的。新版：

- (a) 逐字段专属文案（照旧）
- (b) 统一一条通用文案

➡️ 推荐 **(a)**：旧文案已经写到"哪个字段、什么路径、为什么不行"，是明确好于通用文案的，属于该保留的资产。

---

## Comments

### 2026-09-11 裁定

**D5 — 配置文件位置与基准 → 相对「当前工作目录」的 `config/` 目录**

用户裁定（原文）：Go 会编译出一个单文件，到时候这个单文件会放在 `/usr/bin` 下面；所以读取配置文件的时候，就按照运行的工作目录下的 `config/` 里面找就行了。`config` 里放 toml。至于工程目录，可以把 `config` 目录放入口 `main` 文件同级。

落定结果：

| 项 | 结论 |
| --- | --- |
| 查找基准 | **当前工作目录（cwd）** |
| 查找路径 | `./config/` |
| 文件格式 | TOML |
| **文件后缀** | **`.conf`**（不用 `.toml`） |
| 完整路径 | `./config/SSHFleet.conf` |
| 工程内位置 | `modules/SSHFleet_Go/config/`（与 `main.go` 同级），作为模板/默认配置 |
| 部署形态 | 单个可执行文件放 `/usr/bin/`，配置随工作目录走 |

**由此产生的已知后果（记录在案，非缺陷）**：工具**必须从含 `config/` 的目录启动**，从其他目录调用会找不到配置——与旧版"必须从项目目录启动"的行为一致，属有意保留。

> 待议（不阻塞 M1）：是否需要"cwd 下找不到时回落到可执行文件同级的 `config/`"作为兜底。留待 M6 迁移面一并考虑。

**其余 D1–D4、D6、D7 待裁定。**
