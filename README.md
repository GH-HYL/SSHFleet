# SSHFleet

SSHFleet 是一个 **SSH 批量运维工具**：一次命令输入，派出一支"舰队"（fleet）——批量 ssh 连接每一台目标服务器，批量执行命令、运行脚本、上传/下载文件，结果自动归档。

---

## 目录

- [① 这是什么，能干什么](#①-这是什么能干什么)
- [② 安装](#②-安装)
- [③ 快速上手](#③-快速上手)
- [④ 核心概念](#④-核心概念)
- [⑤ 四种模式与参数](#⑤-四种模式与参数)
- [⑥ 进阶用法](#⑥-进阶用法)
- [⑦ 结果与历史记录](#⑦-结果与历史记录)
- [⑧ 技术架构](#⑧-技术架构)
- [⑨ 常见问题（FAQ）](#⑨-常见问题faq)
- [附录：依赖 / 仓库](#附录依赖--仓库)

---

## ① 这是什么，能干什么

### 定位

把「SSH 登录 → 执行 → 退出」这套逐台操作，变成「**一条命令操作清单里的所有服务器**」。你准备一份服务器清单，告诉工具要做什么，剩下的并发、收集、归档都交给工具。

### 能力

| 能力         | 说明                                   |
| ---------- | ------------------------------------ |
| 批量执行命令     | 一条命令跑遍清单里的所有服务器，实时看进度                |
| 批量执行脚本     | 把本地 `.sh` / `.py` 脚本发到每台服务器执行        |
| 批量上传文件     | 本地文件/目录分发到每台服务器的指定位置                 |
| 批量下载文件     | 收集每台服务器的文件/目录，按 IP 分目录存到本地           |
| 危险命令防护     | 自动识别 `rm -rf /` 等危险命令，执行前要求确认        |
| 密码 / 密钥双认证 | 支持密码登录与密钥登录（密钥优先、密码兜底）               |
| 结果自动归档     | 每次执行生成独立目录：终端输出（txt/excel）、汇总报告、资源备份 |

---

## ② 安装

### 前置要求

| 项      | 要求                         |
| ------ | -------------------------- |
| 操作系统   | Windows / Linux            |
| Python | 3.10+（安装时勾选 "Add to PATH"） |
| Go 引擎  | 一个可执行文件（见下）                |

### 安装步骤

安装 Python 依赖（在项目目录执行）：

```bash
# Windows:
python -m pip install loguru pydantic pyyaml rich openpyxl requests

# Linux:
python3 -m pip install loguru pydantic pyyaml rich openpyxl requests
```

准备 Go 引擎：

> [!NOTE] 📖 深入：Go 引擎

> SSHFleet 的并发批量执行能力由一个 Go 小程序（执行引擎）提供。仓库**不含**编译好的引擎，需要自行放入：

> - **方式一（推荐）**：找作者/发布包要现成的 `SSHFleet_Go.exe`（Windows）或 `SSHFleet_Go`（Linux），放到 `src/go/` 目录
> - **方式二**：源码在 `modules/SSHFleet_Go/`，装好 Go 环境后 `go build`，产物放入 `src/go/`

> 引擎缺失或放错位置时工具会启动报错。

---

## ③ 快速上手

这一章讲**整体怎么用**：从最快到最标准，先跑通一条命令。过程中涉及的概念（CSV 清单、凭据文件、参数）在后续章节详解，这里先用最小例子跑起来。

### 3.1 最快路径：内联清单（不需要建任何文件）

`-f` 可以直接接一段服务器信息（内联清单），最小写法只写 IP：

```bash
python sshfleet.py -f "192.168.1.10" -c "uptime"
```

回车后工具交互询问密码（输入不显示属正常），即可连接该服务器执行命令。

> [!NOTE] 📖 深入：内联清单（-f 的第二种用法）

> `-f` 既能接文件路径（`-f nodes.csv`），也能直接接一段服务器信息。内联清单用英文逗号分隔，最多 6 段，与 CSV 6 列一一对应：

> -f "IP, 端口, 用户名, 密码文件路径, 密钥文件路径, 私钥口令文件路径"

> **只写 IP 时**其余全用默认值：端口/用户名用配置默认，密码由工具交互询问。临时测一台机器时，内联清单比建 CSV 快得多。各段省略时依次回退「配置默认 → 交互输入」（详细规则见 4.1）。

### 3.2 标准流程：清单 + 密码文件 + 模式

准备一台以上服务器、一个账号密码，然后照这个流程走：

```bash
# 第 1 步：准备密码文件（把明文密码写进文件，再用转换命令按当前等级转换，详见 4.2）
echo -n '你的服务器密码' > ~/.MyPW/pw.txt
python sshfleet.py --convert-password ~/.MyPW/pw.txt

# 第 2 步：写清单（所有服务器的端口、账号、密码等配置一样且已经配置默认配置时，只写 IP 即可）
#    nodes.csv:
#    192.168.1.10
#    192.168.1.11
#    192.168.1.12

# 第 3 步：执行
python sshfleet.py -f nodes.csv -c "uptime"
```

### 3.3 常用场景示例

```bash
# 批量执行命令（看磁盘）
python sshfleet.py -f nodes.csv -c "df -h"

# 批量跑部署脚本（sudo 权限）
python sshfleet.py -f nodes.csv -s deploy.sh -m sudo

# 分发文件到所有服务器
python sshfleet.py -f nodes.csv -u ./app.tar.gz -p /opt/

# 收集所有服务器的日志（按 IP 分目录）
python sshfleet.py -f nodes.csv -d /opt/logs/app.log -p ./downloads

# 密钥登录（统一私钥，详见 6.1）
python sshfleet.py -f nodes.csv -c "uptime" -k ~/.ssh/id_rsa

# 非交互批量（跳过所有确认）
python sshfleet.py -f nodes.csv -s deploy.sh --disinteractive
```

---

## ④ 核心概念

批量操作时，你会接触到三个概念：**节点清单 CSV**、**凭据文件**（用于存放密码内容）、**配置默认值**。这一章讲清它们，⑤ 再讲四种模式怎么选。

### 4.1 节点清单 CSV

CSV 是一份"服务器清单"：纯文本，每行一台服务器，英文逗号分隔，允许使用#号注释单行，可以用记事本 / VSCode 编辑。

```csv
192.168.1.10
192.168.1.11
192.168.1.12
```

上面的清单表示 3 台服务器，其余信息（端口/用户名/密码）按 4.3 的规则取默认值。

> [!NOTE] 📖 深入：CSV 文件格式（6 列详解）

> 每行固定 **6 列**，按顺序排列，后面的列可留空：

> | 列 | 字段 | 必填 | 说明 |

> | --- | --- | --- | --- |

> | 1 | IP | 是 | 服务器 IP |

> | 2 | 端口 | 否 | SSH 端口（默认使用配置文件配置端口） |

> | 3 | 用户名 | 否 | 登录用户名（如 `root`，默认使用配置文件配置用户名） |

> | 4 | 密码文件路径 | 否 | **密码文件的路径，不是密码本身**（见 4.2），默认使用配置文件配置密码路径 |

> | 5 | 密钥文件路径 | 否 | PEM 私钥路径（密钥登录时填），在使用 -k 选项后，默认使用配置文件配置密钥路径 |

> | 6 | 私钥口令文件路径 | 否 | 仅当第 5 列私钥本身加密时填，在使用 -k 选项后，默认使用配置文件配置私钥口令路径 |

> **常见写法（从简到全，照着抄）：**

> ```csv
> # ① 单个 IP（最简单）——其余全用配置默认值
> 192.168.1.10
>
> # ② 常用配置：IP + 端口 + 用户名 + 密码文件路径
> 192.168.1.10,22,root,~/.MyPW/pw.txt
>
> # ③ 多台服务器共用一个密码文件（端口/用户名留空用默认）
> 192.168.1.10,22,root,~/.MyPW/pw.txt
> 192.168.1.11,,user,~/.MyPW/pw.txt
> 192.168.1.12,,,~/.MyPW/pw.txt
>
> # ④ 每台密码不同——第 4 列各自指向不同的密码文件
> 192.168.1.10,,,/opt/keys/node10_pw.txt
> 192.168.1.11,,,/opt/keys/node11_pw.txt
>
> # ⑤ 密钥登录（不用密码）——第 4 列留空，第 5 列写私钥路径
> 192.168.1.10,,,,~/.ssh/id_ed25519
>
> # ⑥ 私钥本身需要口令（有 passphrase）——再加第 6 列口令文件
> 192.168.1.10,22,root,,~/.ssh/id_ed25519,~/.MyPW/key_pp.txt
>
> # ⑦ 全满配置：6 列全填（IP,端口,用户名,密码文件,密钥文件,私钥口令文件）
> 192.168.1.10,10022,deploy,~/.MyPW/pw.txt,~/.ssh/id_ed25519,~/.MyPW/key_pp.txt
> ```

> **列留空时的取值顺序**：配置文件默认值优先使用，若没有配置默认值，需运行时交互输入（交互输入在 `--disinteractive` 选项下的非交互模式下则报错）。

> 登录**认证判定**逻辑（一台服务器用哪种方式登录）：

> - 第 4、5 列都填 → **密钥优先**；密钥解析失败且有可用密码时自动回退密码
> - 只填第 5 列 → 纯密钥登录
> - 只填第 4 列 / 都空 → 用密码登录

> [!WARNING] 密码不要直接写进 CSV：CSV 作为清单文件易被复制/分享，明文密码等于裸奔。密码放独立"凭据文件"，CSV 只写路径（见 4.2）。

### 4.2 凭据文件

**密码/私钥口令不直接写进 CSV，而是放在一个"凭据文件"里，CSV 只引用文件路径。** 密码文件的内容格式由配置的「密码安全等级」决定，共三档：

| 等级            | 文件里的内容        | 安全度 | 建议适合场景     | 需要做什么                    |
| ------------- | ------------- | --- | ---------- | ------------------------ |
| **1（明文）**     | 密码原文          | 最低  | 本机 / 临时演示  | 密码直接写入文件                 |
| **2（base64）** | 密码的 base64 编码 | 中等  | 简单的工作环境    | 写明文后转换一次，转换后可后续复用，无须再次转换 |
| **3（加密）**     | 加密密文          | 最高  | 多环境 / 敏感场景 | 先生成主密钥，再转换，复用逻辑同base64   |

> [!NOTE] 📖 深入：密码与凭据文件

> **为什么不能把密码直接写进 CSV？** 清单文件经常被复制、分享、留存、审计或进版本库，密码写进去等于跟着清单到处跑。约定：密码单独放文件，CSV 只写"文件在哪"。

> **凭据文件的内容**由配置 `account.password_security` 决定：

> - 等级 1（明文）：文件内容就是密码原文
> - 等级 2（base64）：文件内容是密码的 base64 编码
> - 等级 3（加密）：文件内容是密文，解密需要主密钥（由 --gen-key 选项生成存放于系统环境变量的密钥进行加解密 ）

> **正确用法：用转换命令按等级转换**（`--convert-password` 自动识别文件内容格式——明文 / base64 / 加密，再转成配置等级对应的格式）：

> ```bash
> python sshfleet.py --convert-password ~/.MyPW/pw.txt
> ```

> 转换命令读的是**文件里的明文内容**，所以先把明文密码写进一个文件（记事本新建或命令行都行）：

> ```bash
> echo -n '你的服务器密码' > ~/.MyPW/pw.txt
> ```

> 然后运行转换命令，文件内容被就地转为配置等级对应的格式。转换后 CSV 第 4 列（或配置 `account.password`）指向该文件，工具运行时会按等级自动还原密码。**转换是自适应的**：文件已是目标格式会提示跳过；从高等级降到低等级（如加密→base64、加密→明文）会自动用主密钥解密转换，无需先改配置。
>
> 转换命令自带防呆校验：文件为空 / 含二进制数据（非文本文件）会直接报错；明文内容含换行（误粘贴多行）会报错提示；需要解密（降级或加密格式转换）但未配置主密钥、或主密钥不匹配时，会明确提示并附配置教程。

> **等级怎么选？** 默认 **2（base64）** 足够；安全性要求高再切 **3（加密）**（先 `--gen-key` 生成主密钥，密钥存于系统环境变量、与凭据文件分离）；完全信任本机环境才用 **1（明文）**。

> **手动生成**转换凭据文件（仅等级 2 适用，等级 3 无法手算）：

> ```bash
> # Linux / macOS:
> echo -n '你的服务器密码' | base64 > ~/.MyPW/pw.txt
>
> # Windows PowerShell:
> [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes('你的服务器密码')) | Out-File -NoNewline ~/.MyPW/pw.txt
> ```

### 4.3 配置默认值与相对路径

配置文件位于 `src/config/SSHFleet.yaml`，提供 CSV 留空字段的默认值：

```yaml
account:
  port: 22                  # 默认端口：CSV 第 2 列留空时用
  user: root                # 默认用户名：CSV 第 3 列留空时用
  password: ~/.MyPW/pw.txt  # 默认密码文件：CSV 第 4 列留空时用
```

> [!NOTE] 📖 深入：相对路径的拼接规则（secret_dir）

> 配置 `account.secret_dir` 是"凭据目录"。CSV/配置里写**相对路径**（如 `pw.txt`）时，会拼接为 `secret_dir/pw.txt`。

> 三种路径写法：

> | 写法 | 含义 |

> | --- | --- |

> | `~/.MyPW/pw.txt` | `~` 展开为用户目录 |

> | `/home/user/pw.txt` | 绝对路径，原样使用 |

> | `pw.txt` | 相对路径，拼接 `secret_dir` |

> 只需在配置里写一次 `secret_dir`，CSV 里写短文件名即可，密码文件集中管理。

---

## ⑤ 四种模式与参数

四种操作模式，**每次只能选一种**：

| 模式 | 参数                | 作用          | 示例                                    |
| -- | ----------------- | ----------- | ------------------------------------- |
| 命令 | `-c "命令"`         | 每台服务器执行一条命令 | `-c "df -h"`                          |
| 脚本 | `-s 脚本文件`         | 上传本地脚本并执行   | `-s deploy.sh`                        |
| 上传 | `-u 本地路径 -p 远端目录` | 分发文件/目录到服务器 | `-u ./app.tar.gz -p /opt/`            |
| 下载 | `-d 远端路径 -p 本地目录` | 收集服务器文件/目录  | `-d /opt/logs/app.log -p ./downloads` |

```bash
python sshfleet.py -f nodes.csv -c "df -h"
python sshfleet.py -f nodes.csv -s deploy.sh
python sshfleet.py -f nodes.csv -u ./app.tar.gz -p /opt/
python sshfleet.py -f nodes.csv -d /opt/logs/app.log -p ./downloads
```

> [!NOTE] 📖 深入：-p 参数的方向

> `-p` 在上传/下载模式下的含义相反：

> - 上传 `-u 本地 -p 远端`：`-p` 是**服务器上的目录**（文件发到哪）
> - 下载 `-d 远端 -p 本地`：`-p` 是**本地的目录**（文件收到哪）

> 一句话记：`-p` 永远是"目标位置"——上传的目标在服务器，下载的目标在本地。

### 参数总表

批量执行（四种模式**四选一**）：

```text
python sshfleet.py  ( -c | -s | -u | -d )  ( -f ) ( -p ) [可选参数]
```

工具选项（**单独使用**，不与批量执行模式搭配）：

```text
python sshfleet.py --gen-key
python sshfleet.py --convert-password 文件路径
```

**① 模式参数（四选一）**

| 参数        | 说明                          |
| --------- | --------------------------- |
| `-c "命令"` | 命令模式：在每台服务器执行一条命令           |
| `-s 脚本文件` | 脚本模式：上传本地脚本并执行              |
| `-u 本地路径` | 上传模式：把本地文件/目录传到服务器（需 `-p`）  |
| `-d 远端路径` | 下载模式：从服务器下载文件/目录到本地（需 `-p`） |

**② 工具选项（单独使用）**

| 参数                      | 说明                               |
| ----------------------- | -------------------------------- |
| `--gen-key`             | 生成主密钥并写入系统环境变量（等级 3 加密凭据用，见 4.2） |
| `--convert-password 文件` | 转换凭据文件：自动识别格式（明文/base64/加密）并按配置等级转换，支持升降级（见 4.2） |

**③ 其他参数**

| 参数                 | 说明                                                                  |
| ------------------ | ------------------------------------------------------------------- |
| `-f csv_file`      | 节点清单：CSV 文件路径，或内联的一段服务器信息（`-c`/`-s`/`-u`/`-d` 时必须带，见 3.1）           |
| `-p path`          | 目标位置（上传/下载必带，方向见上）                                                  |
| `-m mode`          | 执行身份：`direct`=登录用户身份，`sudo`=root 身份（默认取配置 `execution.mode`，默认 sudo） |
| `-t timeout`       | 单台执行/传输超时（秒）；默认命令/脚本 60s、上传/下载 300s                                 |
| `-T timeout`       | 连接每台服务器的超时（秒）；默认 10s                                                |
| `-n number`        | 并发数；不填默认全部并行                                                        |
| `-r remark`        | 任务备注，作为历史记录文件夹后缀                                                    |
| `--nobash`         | 命令模式专用：不套 bash，直接执行原始命令                                             |
| `--disinteractive` | 跳过所有确认/询问直接执行                                                       |
| `-k [路径]`          | 密钥登录开关（三态，见 6.1）                                                    |

---

## ⑥ 进阶用法

### 6.1 密钥登录与 -k 三态

工具**不会**因为 CSV 配了密钥就自动用密钥，必须显式加 `-k` 才启用密钥登录。

> [!NOTE] 📖 深入：-k 的三种用法（三态）

> **① 不写 `-k` = 纯密码登录**  
> 忽略所有密钥配置（CSV 第 5/6 列、配置密钥项），只走密码逻辑。

> **② 仅写 `-k`（不带路径）= 逐节点用自己的密钥**  
> 密钥按「CSV 第 5 列 → 配置默认密钥」解析；私钥口令按「CSV 第 6 列 → 配置默认口令」解析。适合每台密钥不同。

> **③ `-k /path/to/key` = 所有节点统一用这把私钥**  
> 覆盖节点自带的密钥/口令。路径相对终端工作目录（`~` 展开）。私钥加密时运行时交互询问口令，直接回车=无口令。

> ```bash
> # 统一私钥登录（加密私钥会交互问口令）
> python sshfleet.py -f nodes.csv -c "df -h" -k /opt/keys/id_rsa
>
> # 每台用各自的密钥（CSV/配置里配好）
> python sshfleet.py -f nodes.csv -c "df -h" -k
>
> # 纯密码（忽略一切密钥配置）
> python sshfleet.py -f nodes.csv -c "df -h"
> ```

### 6.2 sudo / 并发 / 超时 / 非交互

**sudo 执行**（命令需要 root 权限时，可通过配置文件配置默认执行权限）：

```bash
python sshfleet.py -f nodes.csv -c "systemctl restart nginx" -m sudo
```

**限制并发**（默认全部并行，服务器扛不住时限流）：

```bash
python sshfleet.py -f nodes.csv -c "uptime" -n 10
```

**超时控制**：`-T` 连接超时、`-t` 执行/传输超时：

```bash
python sshfleet.py -f nodes.csv -c "uptime" -T 15 -t 60
```

**非交互模式**：跳过所有确认和询问（密码/口令需交互的部分会直接报错，需提前在 CSV/配置里备好）：

```bash
python sshfleet.py -f nodes.csv -s deploy.sh --disinteractive
```

### 6.3 配置文件全解

> [!NOTE] 📖 深入：配置文件配置项详解（全字典）

> 以下为完整配置（与 `src/config/SSHFleet.yaml` 一致），注释即说明：
>
> ```yaml
> account:                    # 账号信息（CSV 留空时的默认值）
>   port: 10022               # 默认 SSH 端口（CSV 第 2 列留空时用）
>   user: "jx_zyc"            # 默认用户名（CSV 第 3 列留空时用）
>   secret_dir: "~/.MyPW"     # 凭据目录：密码/私钥/私钥口令文件的相对路径都拼到这里
>   password_security: 2      # 密码安全等级，数字越大越安全：1=明文 / 2=base64（默认） / 3=加密
>   password: "SSHFleet_pw"   # 默认密码文件路径（内容格式随 password_security，用 --convert-password 转换）
>   key: ""                   # 默认私钥文件路径（内容为 PEM 私钥原文）
>   key_passphrase: ""        # 默认私钥口令文件路径（内容格式同密码文件，随等级变化）
>
> execution:                  # 执行参数
>   mode: "sudo"              # 执行身份：direct=登录用户 / sudo=root（默认 sudo）
>   timeout_connect: 10       # 连接超时（秒）
>   timeout_execute: 60       # 执行超时（秒）
>   timeout_transfer: 300     # 传输超时（秒）
>
> enable:                     # 功能开关
>   output_to_xlsx: true      # 终端输出同时导出 xlsx；false=仅输出到 txt
>   results_to_xlsx: true     # 结果固化到 xlsx 文件；false=不输出到本地文件
>
> paths:                      # 各类路径（一般不用改）
>   keywords:
>     error_keywords: "./src/config/error_keywords.yaml"          # 错误分类关键词文件
>     dangerous_keywords: "./src/config/dangerous_keywords.yaml"  # 危险命令正则关键字文件
>   exe:
>     batch_tool_windows: "./src/go/SSHFleet_Go.exe"  # Windows 批量执行引擎
>     batch_tool_linux: "./src/go/SSHFleet_Go"        # Linux 批量执行引擎
>   logs:
>     historys: "historys"       # 历史记录目录名
>     tool: "SSHFleetTools.log"  # 工具日志文件名
>     exec: "SSHFleet_Go.log"    # 执行日志文件名
>   files:
>     asset: "assets"               # 资源备份目录名
>     output: "output.txt"          # 终端输出（txt）
>     output_xlsx: "output.xlsx"    # 终端输出（xlsx）
>     report: "report.txt"          # 汇总报告
>     results_xlsx: "results.xlsx"  # 结果明细（xlsx）
>
> upload:                     # 上传并发策略（按文件大小，单位字节）
>   concurrency_thresholds:
>     small_file: 2097152     # < 2MB：全量并发
>     large_file: 20971520    # > 20MB：串行上传
>     medium_concurrency: 10  # 中间文件：10 并发
> ```
>
> 取值原则：**CSV 没填 → 看配置；配置没有 → 交互询问**。大部分配置保持默认即可，通常只需根据环境调整 `account` 段的默认账号密码。

---

## ⑦ 结果与历史记录

每次执行自动归档到 `historys/` 下独立目录：

```text
historys/
├── SSHFleetTools.log                    # SSHFleet 工具运行日志
└── 2026-08-25_14-30-00_command_备注/     # 每次执行一个目录：时间+英文模式+备注（模式为 command/script/upload/download）
    ├── SSHFleet_Go.log                  # Go 引擎执行日志
    ├── output.txt                       # 终端输出（txt）
    ├── output.xlsx                      # 终端输出（Excel，可开关控制）
    ├── report.txt                       # 汇总报告
    ├── results.xlsx                     # 结果明细（Excel，可开关控制）
    └── assets/                          # 资源备份（CSV/脚本/上传文件）
```

`-r 备注` 可让目录名更好认（如 `-r 发布v2`）。回看历史：进 `historys/` 找对应时间目录。

---

## ⑧ 技术架构

- **Python（编排层）**：参数解析、危险命令检查、日志整理、结果输出与报告生成
- **Go（执行引擎）**：高并发 SSH 连接，命令执行、文件上传下载
- **通信**：Python 启动 Go 子进程，Go 起本地 HTTP 服务，Python 通过 SSE 实时接收每台服务器的进度与结果

```text
你的命令 → Python 解析/校验 → 启动 Go 引擎 → Go 并发连接所有服务器
     → 结果通过 SSE 实时回流 → Python 统计/输出/归档
```

---

## ⑨ 常见问题（FAQ）

**Q1：提示找不到 Go 引擎 / 引擎相关报错？** A：Go 引擎缺失或没放对位置。把 `SSHFleet_Go.exe`（Windows）/ `SSHFleet_Go`（Linux）放进项目 `src/go/`，见「② 安装」。

**Q2：连接超时？** A：网络不通或服务器响应慢。用 `-T 30` 加大连接超时；批量前先 `-c "uptime"` 单节点验证连通性。

**Q3：密码文件打不开 / 解码失败？** A：现在工具会自动识别文件内容格式（明文/base64/加密）并提示与配置等级是否匹配；若提示"与当前等级不匹配"，按提示将配置 `account.password_security` 改为文件实际等级，或用 `--convert-password` 直接转换（会自动识别并转换到目标等级），见「4.2 凭据文件」。

**Q4：遇到危险命令提示怎么办？** A：工具检测到危险命令会要求确认，输入 `y` 继续（危险有等级，最高风险直接退出工具，不允许执行，可通过 \`./src/config/dangerous_keywords.yaml\` 配置）；定时任务等场景可加 `--disinteractive` 跳过确认（因会跳过大部分确认信息，请谨慎使用）。

**Q5：密钥登录不生效？** A：必须显式加 `-k`（不带路径=逐节点密钥，带路径=全部目标节点统一密钥），只改 CSV 不会启用。见「6.1 密钥登录与 -k 三态」。

**Q6：历史记录在哪看？** A：`historys/` 目录，每次执行一个时间目录，若在 Linux 环境下，工具会在工作区自动创建软链接 \`latest_history\`, 自动指向最新的历史目录，历史目录拓扑详见「⑦ 结果与历史记录」。

---

## 附录：依赖 / 仓库

**Python 依赖**：loguru（日志）、pydantic（配置校验）、pyyaml（配置解析）、rich（终端美化/进度条）、openpyxl（Excel）、requests（与 Go 引擎通信）。

**Go 依赖**：Go 引擎（源码在 `modules/SSHFleet_Go/`）的编译依赖：需要 **Go 1.25+**；核心依赖 `golang.org/x/crypto`（SSH 协议）、`go.uber.org/zap`（日志）、`github.com/pkg/sftp`（SFTP 传输）。直接使用现成二进制则无需安装 Go 与这些依赖；自行编译时在 `modules/SSHFleet_Go/` 下执行 `go build` 即可。

**仓库**：

- GitHub: <https://github.com/GH-HYL/SSHFleet>
- Gitee: <https://gitee.com/huang-fugui-123/sshfleet>

**许可**说明：本项目仅供学习和内部使用。

> [!WARNING] 该工具可能存在 BUG，请在测试环境验证后再投入使用。数据无价，操作前请再三思量。
