# SSHFleet

基于 Python + Go 混合开发的 SSH 批量运维工具，支持命令/脚本执行与文件传输，面向大规模服务器运维场景。

---

## 目录

- [功能概览](#功能概览)
- [安装说明](#安装说明)
- [快速开始](#快速开始)
- [使用指南](#使用指南)
  - [参数说明](#参数说明)
  - [CSV 文件格式](#csv-文件格式)
  - [配置文件](#配置文件)
- [详细使用示例](#详细使用示例)
- [技术架构](#技术架构)
  - [项目结构](#项目结构)
  - [执行流程](#执行流程)
  - [历史记录结构](#历史记录结构)
- [常见问题 (FAQ)](#常见问题-faq)
- [依赖](#依赖)
- [仓库](#仓库)
- [许可证](#许可证)

---

## 功能概览

| 能力 | 说明 |
| --- | --- |
| 命令执行 | 通过 Go 协程引擎多协程并发，支持数千节点，实时进度条显示 |
| 脚本执行 | 远程执行本地 shell/python 脚本，base64 编码传输 |
| 文件上传 | Go 引擎 SFTP 上传，大文件流式传输 |
| 文件下载 | Go 引擎 SFTP 下载，支持远程文件/目录批量下载到本地 |
| 安全防护 | 危险命令正则规则，风险等级提示，交互式确认 |
| 密钥登录 | 支持 PEM 私钥认证，密钥优先、密码兜底 |
| 错误分类 | SSH/网络错误自动归类 |
| 日志归档 | 每次执行独立目录，含终端输出(txt/xlsx)、执行日志、汇总报告、资源备份 |

---

## 安装说明

### 前置要求

- Python 3.10+
- Go 引擎需自行编译：源码在 `modules/SSHFleet_Go/`，用 `go build` 编译后，把生成的 `SSHFleet_Go`（Linux）/ `SSHFleet_Go.exe`（Windows）放进 `src/go/` 目录（仓库不含预编译二进制）。如果你不会编译 Go，可临时从作者处获取对应二进制放入该目录
- 支持的操作系统：Windows / Linux

### 安装步骤

1. **安装 Python 依赖**

```bash
# 建议使用虚拟环境
python -m venv venv

# 激活虚拟环境
# Windows:
venv\Scripts\activate
# Linux:
source venv/bin/activate

# 安装依赖
pip install loguru pydantic pyyaml rich openpyxl requests
```

1. **配置默认参数（可选）**

编辑 `src/config/SSHFleet.yaml`，配置默认端口、用户名、密码等参数：

```yaml
account:
port: 22
user: root
password: '/path/to/password_file'  # 密码文件路径，文件内容为base64编码的密码
```

### 密码 base64 编码方法

```python
import base64
password = "你的密码"
encoded = base64.b64encode(password.encode('utf-8')).decode('utf-8')
# 将 encoded 写入文件，文件路径填入配置文件的 password 字段
with open('/path/to/password_file', 'w') as f:
    f.write(encoded)
```

---

## 快速开始

> 💡 运行前先准备好两样东西（不知道怎么弄？看下面的「CSV 文件格式」一节，手把手教你）：

> 1. 一份节点清单 `nodes.csv`（每行一台服务器）
> 2. 一份「密码文件」（里面是经 Base64 编码的密码，**不是密码原文**）

> 下面是最常用的几条命令，挑你需要的用：

```bash
# 命令模式
python3 sshfleet.py -f nodes.csv -c "ls -l"

# 脚本模式
python3 sshfleet.py -f nodes.csv -s script.sh

# 上传模式
python3 sshfleet.py -f nodes.csv -u /local/path -p /remote/path/

# 下载模式
python3 sshfleet.py -f nodes.csv -d /opt/logs/app.log -p ./downloads

# 内联CSV（单个节点，无需编辑CSV文件）
python3 sshfleet.py -f "192.168.1.10,22,root,~/.MyPW/pw.txt" -c "ls"

# 非交互模式（跳过所有确认提示）
python3 sshfleet.py -f nodes.csv -c "df -h" --disinteractive
```

---

## 使用指南

### 参数说明

```
python3 sshfleet.py  ( -c | -s | -u | -d )  ( -f ) ( -p ) [可选参数]
```

> 白话解释：每次只能选一种「模式」（`-c`/`-s`/`-u`/`-d` 四选一，`|` 表示「或」）；选了 `-c`/`-s`/`-u`/`-d` 时必须再带 `-f`（节点清单），上传/下载还要带 `-p`；其余都是可加可不加的选项。

#### 必填参数（四种模式，每次只能选一种）

| 参数 | 说明 |
| --- | --- |
| `-c command` | 命令模式：在多台服务器上执行一条命令 |
| `-s script` | 脚本模式：在多台服务器上执行一个本地脚本（.sh/.py） |
| `-u upload` | 上传模式：把本地文件或目录传到服务器 |
| `-d download` | 下载模式：从服务器下载文件或目录到本地 |

> 选 `-c`/`-s`/`-u`/`-d` 时，必须再带上 `-f`（节点清单）。

#### 条件必填参数

| 参数 | 说明 |
| --- | --- |
| `-f csv_file` | 节点清单：CSV 文件路径，或直接在命令行写一行节点信息（-c/-s/-u/-d 时必须带） |
| `-p path` | 目标路径：上传到服务器的目录 / 从服务器下载到的本地目录（-u/-d 时必须带） |

#### 可选参数

| 参数 | 说明 |
| --- | --- |
| `-m mode` | 执行身份：`direct`=用登录用户身份，`sudo`=用 root 身份执行（默认 direct） |
| `-t timeout` | 单台执行或传输的超时时间（秒） |
| `-T timeout` | 连接每台服务器的超时时间（秒） |
| `-n number` | 并发数：同时操作几台服务器；不填则全部并行（默认同时跑全部节点） |
| `-r remark` | 给这次任务起个名字，会作为历史记录文件夹的后缀（不填自动生成） |
| `--nobash` | 命令模式专用：不套一层 bash 环境，直接执行原始命令 |
| `--disinteractive` | 跳过所有确认提示直接执行（批量跑脚本时常用） |
| `-k [KEY_PATH]` | 密钥登录开关（三态，详见下方「密钥登录与 `-k` 选项」）：不指定=纯密码；仅 `-k`=用 CSV/配置默认密钥；`-k 路径`=所有节点统一私钥 |

### CSV 文件格式

CSV 就是一份"服务器清单"：**纯文本文件，每行一台服务器，列之间用英文逗号 **`,`** 分隔**。第一行直接写服务器 IP 就行，不用写标题行；以 `#` 开头的行是注释、会被忽略。你可以用记事本 / VSCode 直接编辑。

固定 **6 列，按顺序排列**（后面的列可以空着不写；某列空着时程序会自动找默认值，规则见下方「某一列留空会怎样」）：

| 列 | 字段 | 必填 | 这一列填什么 |
| --- | --- | --- | --- |
| 1 | IP | 是 | 服务器 IP，如 `192.168.1.10` |
| 2 | 端口 | 否 | SSH 端口（默认 22），留空用配置 |
| 3 | 用户名 | 否 | 登录用户名，如 `root`，留空用配置 |
| 4 | 密码文件路径 | 否 | **不是密码本身**，而是"放密码的文件"的路径（见下方第 1 步） |
| 5 | 密钥文件路径 | 否 | PEM 私钥文件路径（用密钥登录时填） |
| 6 | 私钥口令文件路径 | 否 | 仅当第 5 列的私钥本身加密了才需要 |

> ⚠️ 最容易踩的坑：**密码和私钥口令都不要直接写进 CSV**，而是写一个"文件的路径"，那个文件里才是真正的内容（且密码/口令要做 Base64 编码）。这样能避免敏感信息以明文暴露在清单里。

#### 第 1 步：准备"密码文件"（第 4 列要用）

密码文件 = 一个普通文本文件，内容是你服务器密码的 **Base64 编码**（不是密码原文）。下面给出各系统「一行命令」生成法；如果你更习惯 Python，安装说明里也有对应的 Python 写法。

- **Linux / macOS** 终端：

```bash
  echo -n '你的服务器密码' | base64 > ~/.MyPW/pw.txt
```

- **Windows** PowerShell：

```powershell
  [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes('你的服务器密码')) | Out-File -NoNewline ~/.MyPW/pw.txt
```

  （`~` 指你的用户目录，Windows 下即 `C:\Users\你的用户名`）

然后 CSV 第 4 列就写这个文件的路径，例如 `~/.MyPW/pw.txt`。程序会读取该文件、Base64 解码后得到真正的密码去登录。

#### 第 2 步：写 CSV（挑适合你的场景抄）

**场景 A：所有服务器端口 / 用户名 / 密码都一样（最常见）** 只要按第 1 步准备好一个密码文件，CSV 可以极简——每行只写 IP，其余全留空（自动用配置默认值）：

```csv
192.168.1.10
192.168.1.11
192.168.1.12
```

**场景 B：每台服务器密码不同** 每台准备各自的密码文件，第 4 列分别指向：

```csv
192.168.1.10,22,root,/opt/keys/node10_pw.txt
192.168.1.11,22,root,/opt/keys/node11_pw.txt
```

**场景 C：用密钥登录（不用密码）** 第 4 列留空，第 5 列写你的 PEM 私钥文件路径（如 `~/.ssh/id_ed25519`）：

```csv
192.168.1.10,22,root,,~/.ssh/id_ed25519
```

私钥文件就是 SSH 标准的 `-----BEGIN ... PRIVATE KEY-----` 文本，**原样放进文件即可，不需要 Base64**。

**场景 D：密钥本身加密了（有 passphrase）** 再加第 6 列，指向"口令文件"（内容是该口令的 Base64，生成方式与第 1 步相同）：

```csv
192.168.1.10,22,root,,~/.ssh/id_ed25519,~/.MyPW/key_pp.txt
```

完整 6 列示范：

```csv
# IP,端口,用户名,密码文件路径,密钥文件路径,私钥口令文件路径
192.168.1.10,10022,root,~/.MyPW/pw.txt,~/.ssh/id_ed25519,~/.MyPW/key_pp.txt
```

#### 路径怎么写（三种写法都支持）

- `~/.MyPW/pw.txt` — `~` 自动展开成你的用户目录
- `/home/user/pw.txt` — 绝对路径
- `./pw.txt` — 相对路径，会拼接配置里的 `account.secret_dir`（密码 / 私钥 / 私钥口令文件都在此目录）

#### 某一列留空会怎样（找不到值时的顺序）

1. 先看 CSV 这一列填了没；
2. 没填 → 用配置文件 `src/config/SSHFleet.yaml` 里的默认值（端口 / 用户名 / 密码 / 私钥 / 口令都有默认项）；
3. 配置也没有 → 运行时让你交互输入（加 `--disinteractive` 非交互模式则会直接报错退出）。

#### 一台服务器到底用哪种方式登录（认证判定）

- 第 4、5 列都填：**优先用密钥**；若密钥解析失败且还有可用密码，自动回退到密码
- 只填第 5 列：纯密钥登录，不需要密码
- 只填第 4 列，或都空：使用密码（CSV 值 → 配置默认密码 → 交互输入）

> 第 6 列（私钥口令）可在 CSV 里**按每台服务器单独设置**；留空时回退到全局配置 `account.key_passphrase`。
> 第 5 列（私钥）同理，**留空时回退到全局配置 **`account.key`（所有节点共用同一个私钥时，只在此配一次即可）。

#### 密钥登录与 `-k` 选项

为安全起见，工具**不会**因为 CSV 里配了密钥就自动用密钥登录；是否用密钥必须显式通过 `-k` 选项开启。该选项有三种用法（三态）：

- **不写 **`-k`：纯密码登录。工具会**完全忽略**所有密钥相关配置（CSV 第 5/6 列、`account.key`、`account.key_passphrase`），也不做任何密钥文件预检查；密码逻辑不受影响。
- **仅写 **`-k`**（不带路径）**：使用密钥登录，密钥来源按「CSV 第 5 列 > 配置默认 `account.key`」逐节点解析；口令按「CSV 第 6 列 > 配置默认 `account.key_passphrase`」解析。这与旧版「配了密钥就默认用」的行为一致，只是现在需要你主动加 `-k`。
- `-k /path/to/key`：把指定私钥作为**所有节点的统一私钥**，覆盖每个节点自带的密钥/口令配置。私钥路径走终端当前工作目录（`~` 会展开，相对路径不拼接 `secret_dir`）。若私钥本身加密（有 passphrase），运行时会**交互提示你输入口令**，直接回车表示无口令；个别节点想用自己的密钥就别带路径、改用「仅 `-k`」。

示例：

```bash
# 所有节点用统一私钥 /opt/keys/id_rsa 登录（加密则交互输口令）
python3 sshfleet.py -f nodes.csv -c "df -h" -k /opt/keys/id_rsa

# 每个节点按 CSV/配置默认用各自的密钥
python3 sshfleet.py -f nodes.csv -c "df -h" -k

# 纯密码（忽略一切密钥配置）
python3 sshfleet.py -f nodes.csv -c "df -h"
```

### 配置文件

配置文件路径：`src/config/SSHFleet.yaml`。下面是一份**最小可用配置**（把尖括号里换成你自己的）：

```yaml
account:
  port: 22                      # 默认 SSH 端口，CSV 里不写端口时用这个
  user: root                    # 默认登录用户名
  secret_dir: ~/.MyPW           # 凭据目录：CSV 里写的相对路径（密码/私钥/口令文件）都拼到这里
  password: ~/.MyPW/pw.txt      # 默认密码文件：内容是 Base64 编码后的密码（见 CSV 第 1 步）
```

主要配置项：

| 配置段 | 关键参数 | 解读 |
| --- | --- | --- |
| `account` | port, user, secret\_dir, password, key | CSV 里没填端口/用户名/密码/私钥时用的默认值；`password`/`key` 是「文件路径」，文件内容才是真正的密码/私钥 |
| `account` | key\_passphrase | 默认私钥口令文件（仅当用「加密过的密钥」登录才需要）；内容是该口令的 Base64，可被 CSV 第 6 列按节点覆盖 |
| `execution` | mode, timeout\_\* | 执行权限（direct/sudo）、各种超时时间 |
| `enable` | output\_to\_xlsx, results\_to\_xlsx | 是否把结果导出成 Excel |
| `paths` | logs, files, exe, jsons | 日志、文件、历史记录等存放位置（一般不用改） |

---

## 详细使用示例

### 示例 1：批量执行命令

创建 `nodes.csv` 文件：

```csv
192.168.1.10
192.168.1.11
192.168.1.12
```

执行命令：

```bash
python sshfleet.py -f nodes.csv -c "uptime"
```

### 示例 2：批量执行脚本

创建 `deploy.sh` 脚本：

```bash
#!/bin/bash
echo "开始部署"
mkdir -p /opt/app
echo "部署完成"
```

执行脚本：

```bash
python sshfleet.py -f nodes.csv -s deploy.sh -m sudo
```

### 示例 3：批量上传文件

```bash
python sshfleet.py -f nodes.csv -u ./app.tar.gz -p /opt/
```

### 示例 4：批量下载文件

```bash
# 下载单个文件
python sshfleet.py -f nodes.csv -d /opt/logs/app.log -p ./downloads

# 下载整个目录
python sshfleet.py -f nodes.csv -d /opt/logs/ -p ./downloads
```

下载结果按 IP 建子目录存放：

```
./downloads/
├── 10.0.0.1/
│   └── app.log
├── 10.0.0.2/
│   └── app.log
└── 10.0.0.3/
    └── app.log
```

### 示例 5：非交互模式

```bash
python sshfleet.py -f nodes.csv -c "df -h" --disinteractive
```

### 示例 6：内联CSV（单个节点）

无需创建CSV文件，直接在命令行指定节点信息：

```bash
python sshfleet.py -f "192.168.1.10,22,root,~/.MyPW/pw.txt" -c "uptime"
```

---

## 技术架构

Python 负责参数解析、安全检查、日志整理、结果输出；Go 负责高并发 SSH 执行引擎（命令执行、文件上传、文件下载）。两者通过 HTTP SSE（Server-Sent Events）通信：Python 启动 Go 子进程，Go 启动 HTTP 服务器，Python 发送 HTTP 请求并接收 SSE 流式结果。

### 项目结构

```
sshfleet.py                     # 入口：参数解析、流程编排
src/
├── check/                      # 校验模块
│   ├── arguments.py            # 参数合规性检查
│   ├── dangerous.py            # 危险命令检测
│   └── files.py                # 文件存在性检查
├── command/                    # 命令构建模块
│   └── builder.py              # 最终执行命令构建
├── common/                     # 共享层（跨模块公共工具）
│   ├── constants.py            # 公共常量（成功分类名、颜色常量）
│   ├── format_utils.py         # 结果呈现公共函数（模式/状态行/IP排序）
│   ├── error_handler.py        # 错误打印/退出约定/异常装饰器
│   ├── loader.py               # 配置文件加载（Pydantic 模型校验）
│   └── text_utils.py           # 文本清洗、大小格式化、路径规范化
├── config/                     # 配置文件夹
│   ├── SSHFleet.yaml           # 工具配置（账号、超时、路径等）
│   ├── dangerous_keywords.yaml # 危险命令检测规则
│   └── error_keywords.yaml     # 错误分类关键词
├── gotogo/                     # Go 执行器模块
│   ├── go_to_go.py             # 主执行函数：启动 Go 进程 + HTTP SSE 接收 + Rich 进度条
│   ├── caller.py               # Go 进程调用与 HTTP SSE 通信
│   ├── builder.py              # 请求体构建（命令/上传/下载/密钥登录）
│   ├── parser.py               # SSE 响应解析、base64 解码
│   └── classifier.py           # 错误分类
├── go/                         # Go 引擎二进制目录（放入 SSHFleet-Go 可执行文件，仓库不含预编译）
├── input/                      # 输入交互模块
│   ├── args.py                 # 命令行参数解析
│   ├── csv.py                  # CSV 节点文件读取
│   ├── confirm.py              # 参数信息交互确认
│   └── interaction.py          # 用户交互确认
├── log/                        # 日志模块
│   └── logger.py               # 日志初始化与管理
└── output/                     # 输出处理模块
    ├── terminal.py             # 终端格式化输出
    ├── report.py               # 执行报告生成
    ├── xlsx.py                 # Excel 文件生成
    ├── statistics.py           # 结果统计计算
    └── archive.py              # 资源文件备份与打包
```

### 执行流程

```
参数解析(input) → 校验(check) → 用户确认(input)
  ↓
┌─ 命令/脚本模式 ─→ gotogo 模块启动 Go 子进程，通过 HTTP SSE 实时接收结果（Rich 进度条显示）
├─ 上传模式 ─→ gotogo 模块启动 Go 子进程，通过 HTTP SSE 实时接收结果（Rich 进度条显示）
└─ 下载模式 ─→ gotogo 模块启动 Go 子进程，通过 HTTP SSE 实时接收结果（Rich 进度条显示）
  ↓
结果统计(output) → 终端输出(output) → 生成报告(output) → 资源备份(output) → 创建 latest_history 链接
```

### 历史记录结构

```
historys/
├── SSHFleetTools.log                              # 工具运行日志
└── YYYY-MM-DD_HH-MM-SS_模式_备注/                 # 每次执行独立目录
    ├── SSHFleet_Go.log                            # 执行日志（Go 引擎）
    ├── output.txt                                 # 终端输出（txt，命令/上传模式均写入）
    ├── output.xlsx                                # 终端输出（xlsx，由 output.txt 转换）
    ├── report.txt                                 # 汇总报告
    ├── results.xlsx                               # 结果字典（xlsx）
    └── assets/                                    # 资源备份（按模式条件生成）
        ├── <csv_file>                             # 节点 CSV 文件（始终备份）
        ├── <script_file>                          # 执行脚本（仅脚本模式）
        └── <upload_file>                          # 上传文件（仅上传模式）
```

---

## 常见问题 (FAQ)

### Q1: 连接超时怎么办？

A: 可以通过 `-T` 参数增加连接超时时间：

```bash
python sshfleet.py -f nodes.csv -c "uptime" -T 30
```

### Q2: 如何批量处理多个端口的服务器？

A: 在 CSV 文件中为每台服务器单独指定端口（第 4 列是「密码文件路径」，不是密码本身，详见「CSV 文件格式」第 1 步）：

```csv
192.168.1.10,22,root,~/.MyPW/pw.txt
192.168.1.11,10022,root,~/.MyPW/pw.txt
192.168.1.12,20022,root,~/.MyPW/pw.txt
```

### Q3: 遇到高危命令提示怎么办？

A: 工具会检测危险命令并提示确认。如果确认要执行，输入 `y` 继续；或者使用 `--disinteractive` 参数跳过确认（谨慎使用）。

### Q4: 如何查看历史执行记录？

A: 历史记录保存在 `historys/` 目录下，每次执行创建一个独立目录。

### Q5: Windows 下执行报错？

A: 确保 Go 可执行文件 `SSHFleet_Go.exe` 存在，且未被杀毒软件拦截。

### Q6: 第 4 列到底填什么？为什么不能直接写密码？

A: 第 4 列填的是「密码文件路径」，不是密码本身。因为直接把密码写进 CSV 会以明文暴露，所以约定：把密码做 Base64 编码后存进一个文件，CSV 里只写这个文件的路径。生成方法见「CSV 文件格式」第 1 步。如果你嫌麻烦，也可以不填第 4 列，改在配置文件 `account.password` 里设好默认密码文件，或者运行时让程序交互式问你密码。

### Q7: 怎么用密钥登录（不开密码）？

A: 密钥登录现在需要显式加 `-k` 选项才会启用（不写 `-k` 工具会当成纯密码）。在第 5 列填你的 PEM 私钥文件路径（如 `~/.ssh/id_ed25519`）、第 4 列留空，并加上 `-k` 即可，详见「CSV 文件格式 - 场景 C」。如果私钥本身加密了（有 passphrase），再把口令文件路径填到第 6 列（或用 `-k /path` 统一私钥，运行时会交互问你口令），详见场景 D 与「密钥登录与 `-k` 选项」。

---

## 依赖

Python 3.10+，主要依赖：

- `loguru` - 日志记录
- `pydantic` - 数据模型验证
- `pyyaml` - 配置文件解析
- `rich` - 终端美化和进度条
- `openpyxl` - Excel 文件生成
- `requests` - HTTP 通信（与 Go 进程 SSE 交互）

---

## 仓库

- GitHub: [github.com/GH-HYL/SSHFleet](https://github.com/GH-HYL/SSHFleet.git)
- Gitee: [gitee.com/huang-fugui-123/sshfleet](https://gitee.com/huang-fugui-123/sshfleet)
- 邮箱: <465317918@qq.com>

---

## 许可证

本项目仅供学习和内部使用。

---

> **警告：** 该工具可能存在 BUG，请在测试环境验证后再投入使用。数据无价，操作前请再三思量。