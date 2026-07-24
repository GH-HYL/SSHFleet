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


| 能力     | 说明                                                                 |
| -------- | -------------------------------------------------------------------- |
| 命令执行 | 通过 Go 协程引擎多协程并发，支持数千节点，实时进度条显示             |
| 脚本执行 | 远程执行本地 shell/python 脚本，base64 编码传输                      |
| 文件上传 | Go 引擎 SFTP 上传，大文件流式传输                                    |
| 文件下载 | SFTP 协议，支持下载远程文件/目录到本地（当前版本暂未开放）           |
| 安全防护 | 危险命令正则规则，风险等级提示，交互式确认                           |
| 错误分类 | SSH/网络错误自动归类                                                 |
| 日志归档 | 每次执行独立目录，含终端输出(txt/xlsx)、执行日志、汇总报告、资源备份 |

---

## 安装说明

### 前置要求

- Python 3.10+
- Go 程序已预编译（包含在 `src/go/` 目录下）
- 支持的操作系统：Windows / Linux

### 安装步骤

1. **克隆或下载项目**

```bash
git clone https://github.com/GH-HYL/Multi-SSHFleet.git
cd sshfleet_py
```

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

```bash
# 命令模式
python3 sshfleet.py -f nodes.csv -c "ls -l"

# 脚本模式
python3 sshfleet.py -f nodes.csv -s script.sh

# 上传模式
python3 sshfleet.py -f nodes.csv -u /local/path -p /remote/path/

# 内联CSV（单个节点，无需编辑CSV文件）
python3 sshfleet.py -f "192.168.1.10,22,root,~/.MyPW/pw.txt" -c "ls"

# 非交互模式（跳过所有确认提示）
python3 sshfleet.py -f nodes.csv -c "df -h" --disinteractive

# 打包最新历史记录
python3 sshfleet.py -z
```

---

## 使用指南

### 参数说明

```
python3 sshfleet.py  ( -c | -s | -u | -z )  ( -f ) ( -p ) [可选参数]
```

#### 必填参数（四选一）


| 参数          | 说明                                                       |
| ------------- | ---------------------------------------------------------- |
| `-c command`  | 命令模式，远程执行命令                                     |
| `-s script`   | 脚本模式，远程执行本地脚本（.sh/.py）                      |
| `-u upload`   | 上传模式，本地文件或目录路径                               |
| `-z`          | 打包模式，打包最新历史记录（打包前会删除当前旧打包文件）   |

#### 条件必填参数


| 参数          | 说明                                                        |
| ------------- | ----------------------------------------------------------- |
| `-f csv_file` | 节点 CSV 文件或内联CSV文本（-c/-s/-u 时必填）              |
| `-p path`     | 上传目标路径（-u 时必填）                                   |

#### 可选参数


| 参数               | 说明                                                                 |
| ------------------ | -------------------------------------------------------------------- |
| `-m mode`          | 执行权限\[默认: 配置文件决定]：direct（用户权限）/ sudo（root 权限） |
| `-t timeout`       | 执行/传输超时秒数\[默认: 命令60 / 传输300]                           |
| `-T timeout`       | 连接超时秒数\[默认: 10]                                              |
| `-n number`        | 并发数\[默认: 节点数]                                                |
| `-r remark`        | 备注信息，用于历史记录文件名后缀                                     |
| `--nobash`         | 命令模式专用，不使用 bash 环境执行命令                               |
| `--disinteractive` | 取消高危命令告警和配置信息确认的交互提示                             |

### CSV 文件格式

```csv
# 完整格式：IP,端口,用户名,密码文件路径
192.168.1.10,10022,root,~/.MyPW/pw.txt

# 极简格式：端口/用户/密码使用配置文件默认值
192.168.1.10

# 密码列为空时，使用配置文件的默认密码文件
192.168.1.10,10022,root,
```

密码字段为 Base64 编码的密码文件路径，支持三种格式：
- `~/.MyPW/pw.txt` — 展开 HOME 目录
- `/home/user/pw.txt` — 绝对路径
- `./pw.txt` — 相对路径，与 `password_dir` 配置拼接

### 配置文件

配置文件路径：`src/config/SSHFleet.yaml`，主要配置项：


| 配置段      | 关键参数                            | 说明                                   |
| ----------- | ----------------------------------- | -------------------------------------- |
| `account`   | port, user, password\_dir, password | CSV 缺省值，密码为 base64 编码文件路径 |
| `execution` | mode, timeout\_\*                   | 执行模式、超时时间                     |
| `enable`    | output\_to\_xlsx, results\_to\_xlsx | 输出格式开关                           |
| `paths`     | logs, files, exe, jsons             | 各类文件路径                           |

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

### 示例 4：非交互模式

```bash
python sshfleet.py -f nodes.csv -c "df -h" --disinteractive
```

### 示例 5：内联CSV（单个节点）

无需创建CSV文件，直接在命令行指定节点信息：

```bash
python sshfleet.py -f "192.168.1.10,22,root,~/.MyPW/pw.txt" -c "uptime"
```

---

## 技术架构

Python 负责参数解析、安全检查、日志整理、结果输出；Go 负责高并发 SSH 执行引擎（命令执行、文件上传）。两者通过 HTTP SSE（Server-Sent Events）通信：Python 启动 Go 子进程，Go 启动 HTTP 服务器，Python 发送 HTTP 请求并接收 SSE 流式结果。

### 项目结构

```
sshfleet.py                     # 入口：参数解析、流程编排
src/
├── input/                      # 输入处理模块
│   ├── args.py                 #   命令行参数解析
│   ├── csv.py                  #   CSV 节点文件读取
│   └── confirm.py              #   参数信息交互确认
├── check/                      # 校验模块
│   ├── arguments.py            #   参数合规性检查
│   ├── dangerous.py            #   危险命令检测
│   └── files.py                #   文件存在性检查
├── command/                    # 命令构建模块
│   └── builder.py              #   最终执行命令构建
├── gotogo/                     # Go 执行器模块
│   ├── go_to_go.py             #   主执行函数：启动 Go 进程 + HTTP SSE 接收 + Rich 进度条
│   ├── caller.py               #   Go 进程调用与 HTTP SSE 通信
│   ├── builder.py              #   请求体构建（命令/上传）
│   ├── parser.py               #   SSE 响应解析、base64 解码
│   └── classifier.py           #   错误分类
├── output/                     # 输出处理模块
│   ├── terminal.py             #   终端格式化输出
│   ├── report.py               #   执行报告生成
│   ├── xlsx.py                 #   Excel 文件生成
│   ├── statistics.py           #   结果统计计算
│   └── archive.py              #   资源文件备份与打包
├── log/                        # 日志模块
│   └── logger.py               #   日志初始化与管理
├── utils.py                    # 工具函数、装饰器
├── yaml.py                     # 配置文件加载（Pydantic 模型校验）
├── color.py                    # 终端颜色常量
└── config/
    ├── SSHFleet.yaml           # 工具配置（账号、超时、路径等）
    ├── dangerous_keywords.json # 危险命令检测规则
    └── error_keywords.json     # 错误分类关键词
```

### 执行流程

```
参数解析(input) → 校验(check) → 用户确认(input)
  ↓
┌─ 命令/脚本模式 ─→ gotogo 模块启动 Go 子进程，通过 HTTP SSE 实时接收结果（Rich 进度条显示）
└─ 上传模式 ─→ gotogo 模块启动 Go 子进程，通过 HTTP SSE 实时接收结果（Rich 进度条显示）
  ↓
结果统计(output) → 终端输出(output) → 生成报告(output) → 资源备份(output) → 创建 latest_history 链接
```

### 历史记录结构

```
historys/
├── SSHFleetTools.log                              # 工具运行日志
└── YYYY-MM-DD_HH-MM-SS_模式_备注/                # 每次执行独立目录
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

A: 在 CSV 文件中直接指定每个节点的端口：

```csv
192.168.1.10,22,root,密码
192.168.1.11,10022,root,密码
192.168.1.12,20022,root,密码
```

### Q3: 遇到高危命令提示怎么办？

A: 工具会检测危险命令并提示确认。如果确认要执行，输入 `y` 继续；或者使用 `--disinteractive` 参数跳过确认（谨慎使用）。

### Q4: 如何查看历史执行记录？

A: 历史记录保存在 `historys/` 目录下，每次执行创建一个独立目录。也可以使用 `-z` 打包最新记录：

```bash
python sshfleet.py -z
```

### Q5: Windows 下执行报错？

A: 确保 Go 可执行文件 `SSHFleet_Go.exe` 存在，且未被杀毒软件拦截。

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

- GitHub: [github.com/GH-HYL/Multi-SSHFleet](https://github.com/GH-HYL/Multi-SSHFleet)
- Gitee: [gitee.com/huang-fugui-123/sshfleet](https://gitee.com/huang-fugui-123/sshfleet)
- 邮箱: <465317918@qq.com>

---

## 许可证

本项目仅供学习和内部使用。

---

> **警告：** 该工具可能存在 BUG，请在测试环境验证后再投入使用。数据无价，操作前请再三思量。
