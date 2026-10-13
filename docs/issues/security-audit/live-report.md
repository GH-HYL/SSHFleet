# SSHFleet 安全分析 · 实测报告

**测试环境**：本机（Ubuntu），独立 sshd 实例监听 `127.0.0.1:2222`，测试账号 `sshfleet_test`。
系统 sshd 的 `service` 与 `socket` 均已 mask，22 端口无对外监听。详见 `build/atest-1/setup-test-env.sh`。

**被测对象**：`release/SSHFleet_2026-10-13_09-17-50_linux` 的 `SSHFleet` 二进制（配置改为明文凭据、非 sudo 模式）。

**测试日期**：2026-10-13

**结论一句话**：静态推断的 6 项里有 **3 项被实测推翻**，1 项方向修正后成立，1 项确证。

---

## 一、实测结果总表

| 编号 | 静态推断 | 实测结果 | 判定 |
|---|---|---|---|
| V5 / D5 | `--yes` 下 high 档静默放行 | 确证（对照实验干净） | ✅ 成立 |
| V1 / D1 | `xargs` 致命令段被整体丢弃 | 推断错误，`xargs` 后命令词正常参与判定 | ❌ 撤回 |
| V2 / D2 | 递归深度上限可被嵌套绕过 | 推断错误，`eval` 嵌套 15 层仍被检出 | ❌ 撤回 |
| V3 / D3 | 解析失败回退致包装剥离失效 | shell 家族内不成立（见下） | ❌ 撤回 |
| V4 / D4 | 非 shell 解释器的内嵌命令不检测 | 确证，且**双向失效**（漏报 + 误报） | ✅ 成立（升为高危） |
| V6 / D7 | 清单 IPv4 假设 | 间接确认（IPv4 为强制校验） | — |

---

## 二、确证的发现

### V5 / D5｜`--yes` 下 high 档静默放行（确证）

**测试方法**：对同一条命令 `rm -rf /tmp/sshfleet-sandbox/probe_b.txt`，分别在交互模式与 `--yes` 模式下发，观察终端输出与文件状态。

**沙盒**：`/tmp/sshfleet-sandbox/`（归 `sshfleet_test` 所有，确保测试账号有删权限）。
**注**：初次测试误将沙盒放在 `/home/hyl/` 下，该目录权限 0700，测试账号无权访问，命令实际未生效——已重建沙盒后重测。

**交互模式输出**：

```
╔════════════════════════════════════════════════════════╗
║                   ⚠  发现危险命令  ⚠                   ║
╠════════════════════════════════════════════════════════╣
║    来源: 命令                                          ║
║    行号: 1                                             ║
║    内容: rm -rf /tmp/sshfleet-sandbox/probe_b.txt      ║
║    规则: 递归强制删除                                  ║
║    级别: 高                                            ║
╠════════════════════════════════════════════════════════╣
║          这条命令有风险，请确认你确实要执行它          ║
╚════════════════════════════════════════════════════════╝

确定继续？ [y/N]:
输入结束，操作已取消
```

**结果**：文件 `probe_b.txt` **保留**（被拦住）。

**`--yes` 模式输出**：

```
 [非交互模式] 跳过执行参数确认环节，直接执行

【127.0.0.1】 连接: 成功 - 0.052s
【127.0.0.1】 执行: 成功 - 0.009s
【127.0.0.1】 分类: 执行成功
DELETED_OK
```

**结果**：文件 `probe_a.txt` **被删除**，全程无警告、无询问。

**对照结论**：

| 模式 | 警告框 | 询问 | 文件 |
|---|---|---|---|
| 交互 | 显示（规则：递归强制删除 / 级别：高） | `确定继续？[y/N]` | 保留 |
| `--yes` | **不显示** | **无** | 被删 |

**定性**（经与开发者确认，见下）：**非交互模式下放行 high 档是设计如此**，且开发者的顾虑成立——把 high 提到 forbidden 会误伤日常操作（如 `rm -rf /opt/app/data`）。

真正的缺陷在**呈现层**，不在安全模型：`main.go` 里 `--yes` 分支把放行事实拼成了 `autoNote` 字符串，但**只写日志、不打终端**。开发者跑完 2000 台看到的是"成功 2000"，不会知道其中有若干台执行了 `rm -rf`。

**信息已被采集，只是没被呈现。**

---

### V4 / D4｜非 shell 解释器的内容检测双向失效（确证，升为高危）

**前提修正**：开发者确认**解释器可自由配置**（配置项决定脚本后缀与解释器的对应），即"什么脚本都可能"。故本条严重度由中危**升为高危**。

**测试 1：命令模式内嵌**

```
./SSHFleet -f nodes.csv -c 'python3 -c "import os; os.system(\"rm -rf /tmp/sshfleet-sandbox/d4target\")"' --yes
```

**输出**：

```
 [非交互模式] 跳过执行参数确认环节，直接执行

【127.0.0.1】 连接: 成功 - 0.051s
【127.0.0.1】 执行: 成功 - 0.033s
【127.0.0.1】 分类: 执行成功
```

**结果**：`d4target/` 目录连同其中的 `precious.txt` **被整个删除**，**无任何危险命令警告**。

**对照**：同一路径若以裸 `rm -rf /tmp/sshfleet-sandbox/d4target` 下发，会命中"递归强制删除 / 高"并弹出警告框。

**测试 2：脚本模式**

```python
# /tmp/sshfleet-test-scripts/d3.py
import os
x = {"key": "value", "other": 123}
y = [i for i in range(10) if i % 2 == 0]
z = lambda a, b: a + b
os.system("rm -rf /tmp/sshfleet-sandbox/py_target")
```

用 `-s` 下发（解释器按配置为 python）：

- **无任何危险命令警告**
- 脚本"执行成功"（退出码 0）
- 但 `py_target/f.txt` **保留**——因为脚本被当作 **shell 脚本**解析，python 语法对 shell 是无效命令，脚本实际没跑成

**双向失效分析**：

`dangercheck` 用 `mvdan.cc/sh/v3/syntax`（**shell 语法**）解析一切内容，而 `ssh/command.go` 的 `interpreterTable` 支持 15 种解释器（`bash`/`sh`/`dash`/`ksh`/`zsh`/`mksh`/`ash`/`python`/`luajit`/`lua`/`node`/`nodejs`/`perl`/`ruby`/`php`）。两者不对称，导致：

| 方向 | 表现 |
|---|---|
| **漏报** | python 的 `os.system("rm -rf /")` 对 shell 解析器只是一行普通命令，不命中规则 |
| **误报** | python 字符串里出现 `rm -rf` 字样时，可能被当成真实命令报"高危" |

**在"什么脚本都可能"的场景下，危险检测对非 shell 脚本整体失效。**

---

### V3 / D3｜语法失败回退（撤回，附修正说明）

**原推断**：构造让 shell 解析器失败、但真 shell 能跑的语法，可绕过包装命令剥离（如 `sudo` 不被剥离导致 `rm` 规则不匹配）。

**实测**：构造了 5 个用例（`sudo rm -rf /tmp/x; if`、`sudo rm -rf /tmp/x )`、`... <<<`、`... $(`、`... \``），**全部未命中检测**——表面上像是绕过了。

**但端到端验证推翻了它**：

```
$ bash -c 'rm -rf /tmp/sshfleet-sandbox/d3/important.txt; if'
bash: -c: 行 2: 语法错误：第 1 行的 "if" 命令中有未预期的文件结束符
```

bash **在执行前就做整体语法检查**，语法错误的命令根本不会执行，文件保留。经 SSHFleet 下发结果同样是"执行失败(退出码2)"，文件保留。

**结论**：**在 shell 家族内，"检测漏了但命令真跑了"无法成立**——因为 shell 自己也是先解析再执行，能跑的必然能解析。

**残余**：该路径对**非 shell 解释器**可能成立（python 语法错误时 shell 解析器失败，但 python 未必）——不过这在效果上已并入 D4（非 shell 内容本就不被检测），不单列。

---

## 三、被推翻的推断

### V1 / D1｜`xargs`（撤回）

**原推断**：`xargs` 在 `wrappers` 表中，`echo "rm -rf /" | xargs` 剥完为空，命令段被丢弃。

**实测**：

```
$ ./SSHFleet -f nodes.csv -c "echo /tmp/x | xargs rm -rf"
║    内容: rm -rf                                        ║
║    规则: 递归强制删除                                  ║
║    级别: 高                                            ║
```

带选项的变体 `cat /tmp/list.txt | xargs -I{} rm -rf {}` 同样命中：

```
║    内容: rm -rf {}                                     ║
║    规则: 递归强制删除                                  ║
║    级别: 高                                            ║
```

**原因**：读错了 `stripPrefixes` 的逻辑。它剥掉 `xargs` 及其选项后，**剩下的正是真实命令词**（`rm -rf`），`base = "rm"`，判定正常。不存在"命令段被丢弃"。

**撤回。**

---

### V2 / D2｜递归深度上限（撤回）

**原推断**：`renderCall` → `extractInnerCommands` → `ParseSource(inner)` 每次重置 `depth`，可无限嵌套绕过 `maxDepth = 5`。

**实测**：用 `eval` 链构造嵌套（不受引号逃逸影响），深度 4 / 6 / 8 / 10 / 15：

```
eval 深度 4  -> 命中警告: 1
eval 深度 6  -> 命中警告: 1
eval 深度 8  -> 命中警告: 1
eval 深度 10 -> 命中警告: 1
eval 深度 15 -> 命中警告: 1
```

全部命中。`bash -c` 嵌套 7 层同样命中。

**原因**：`maxDepth` 限制的是**单次遍历内的语法树结构深度**（嵌套 if / for / 函数体等），而间接执行的递归解析虽然是新解析，**每层嵌套里的危险命令最终都会被某一层解析到并独立成段**。深度上限在此不构成绕过。

**撤回。**

---

## 四、对开发者判断的回应

### 关于 `--yes` 的语义

开发者原话：

> 我那个 yes 是跳过不过「禁止」级别的……当初设计的时候，高会误伤，直接拒绝不太合适，只有禁止级别才是明确的、会有风险的，才不会跳过。

**核查后确认这个设计是自洽的**：

`main.go` 步骤 5 的 `switch` 中，`forbidden` 是**第一个分支**，在 `Confirm` 之前就 `os.Exit(1)`；`--yes` 只影响第二分支（非 forbidden）的 `Confirm` 行为。

```go
switch {
case dangerReport.HasForbidden():
    output.PrintDangerWarning(dangerReport, true)
    logger.Warn(...)
    _ = logger.Close()
    os.Exit(1)                          // ← --yes 也拦不住这一条
case len(dangerReport.Matches) > 0:
    ...
    if in.Disinteractive {
        dangerNote = autoNote           // ← 只进日志
    }
}
```

且 forbidden 档当前 10 条规则覆盖的是"把机器搞死"级别（`rm -rf /`、`mkfs`、`dd of=/dev/sda`、`shutdown`、`reboot`、`init 0`、`kill -9 -1`、`systemctl mask sshd`…），**判断口径合理**。

**故不改档位**。改进方向见下。

### 建议的改进方向（仅供参考，本轮不改代码）

`--yes` 下命中非 forbidden 档时，在**终端**打一行摘要（不询问、不阻断）：

```
本次有 N 个节点命中危险命令（最高级别 高），已按 --yes 放行，详见日志
```

- 不误伤：不阻断、不改档位、不改 `--yes` 语义
- 不静默：放行这件事在屏幕上可见
- 成本低：`autoNote` 字符串已经在拼，只是没往终端走

按开发者规范红线 1，改代码需"授权开工"，本轮仅记录。

---

## 五、复核方式

| 项 | 命令 |
|---|---|
| 环境搭建 | `sudo bash build/atest-1/setup-test-env.sh` |
| D5 交互对照 | `./SSHFleet -f nodes.csv -c "rm -rf /tmp/sshfleet-sandbox/x"` |
| D5 `--yes` | 同上加 `--yes` |
| D4 命令模式 | `./SSHFleet -f nodes.csv -c 'python3 -c "import os; os.system(\"rm -rf /tmp/sshfleet-sandbox/x\")"' --yes` |
| D1 复验 | `./SSHFleet -f nodes.csv -c "echo /tmp/x \| xargs rm -rf"` |
| 环境清理 | `sudo bash build/atest-1/setup-test-env.sh --cleanup` |

**测试环境当前状态**：保留（按开发者规范「沙盒必须清理，但不必建完就删」）。

---

**报告日期**：2026-10-13
**分析者**：AI Agent（实测）
