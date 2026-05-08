# SSHexec

基于 Python + Go 混合架构的 SSH 批量运维工具，支持命令/脚本执行与文件传输，面向大规模服务器运维场景。

## 功能概览

| 能力 | 说明 |
|------|------|
| 命令执行 | 通过 Go 协程引擎多协程并发，支持数千节点，实时进度条显示 |
| 脚本执行 | 远程执行本地 shell/python 脚本，base64 编码传输 |
| 文件上传 | SFTP 协议，目录自动打包压缩，大文件流式传输 |
| 文件下载 | SFTP 协议，支持下载远程文件/目录到本地 |
| 安全防护 | 危险命令正则规则，风险等级提示，交互式确认 |
| 错误分类 | SSH/网络错误自动归类 |
| 日志归档 | 每次执行独立目录，含终端输出(txt/xlsx)、执行日志、汇总报告、资源备份 |

## 快速开始

```bash
# 命令模式
python3 sshexec.py -f nodes.csv -c "ls -l"

# 脚本模式
python3 sshexec.py -f nodes.csv -s script.sh

# 上传模式
python3 sshexec.py -f nodes.csv -u /local/path -p /remote/path/

# 下载模式
python3 sshexec.py -f nodes.csv -d /remote/path -p /local/path/

# 打包最新历史记录
python3 sshexec.py -z
```

## 参数说明

```
python3 sshexec.py  ( -c | -s | -u | -d | -z )  ( -f ) ( -p ) [可选参数]

必填参数（五选一）：
  -c command     命令模式，远程执行命令
  -s script      脚本模式，远程执行本地脚本（.sh/.py）
  -u upload      上传模式，本地文件或目录路径
  -d download    下载模式，远程文件或目录路径
  -z             打包模式，打包最新历史记录

条件必填：
  -f csv_file    节点 CSV 文件（-c/-s/-u/-d 时必填）
  -p path        上传目标路径/下载存放路径（-u/-d 时必填）

可选参数：
  -m mode        执行权限 [默认: sudo]：direct（用户权限）/ sudo（root 权限）
  -t timeout     执行/传输超时秒数 [默认: 命令60 / 传输300]
  -T timeout     连接超时秒数 [默认: 10]
  -n number      并发数 [默认: 节点数]
  -k [key]       密钥登录，指定私钥路径（不指定默认 ~/.ssh/id_rsa）
  -r remark      备注信息，用于历史记录文件名后缀
  --nobash       命令模式专用，不使用 bash 环境执行命令
  --disinteractive  取消高危命令告警和参数确认的交互提示
```

## CSV 文件格式

```csv
# 完整格式：IP,端口,用户名,密码
192.168.1.10,10022,root,P@ssw0rd

# 极简格式：端口/用户/密码使用配置文件默认值
192.168.1.10

# 密码为空时，运行时交互输入
192.168.1.10,10022,root,
```

默认值在 `src/config/sshexec.ini` 中配置，密码字段为 base64 编码。

## 架构

Python 负责参数解析、安全检查、日志整理、结果输出；Go 负责高并发 SSH 执行引擎。两者通过 stdin/stdout + TCP socket 通信。

```
sshexec.py                     # 入口：参数解析、流程编排
src/
├── core.py                    # 参数解析、节点读取、统计计算
├── gotogo.py                  # Go 执行器调度：启动子进程 + TCP 监控进度 + Rich 进度条显示
├── check.py                   # 参数校验、危险命令检测、脚本内容检查
├── output.py                  # 终端输出、xlsx 报告生成
├── utils.py                   # 工具函数、日志初始化、错误分类、装饰器
├── toml.py                    # 配置文件加载（Pydantic 模型校验）
├── color.py                   # 终端颜色常量
├── transfer/
│   ├── transfer.py            # 传输主流程：上传/下载、进度条、结果收集
│   ├── transfer_check.py      # 传输预检：路径存在性、磁盘空间、写权限
│   └── transfer_utils.py      # SSH 连接创建、文件打包、中断处理
├── config/
│   ├── sshexec.ini            # 工具配置（账号、超时、路径等）
│   ├── dangerous_keywords.json # 危险命令检测规则
│   └── error_keywords.json    # 错误分类关键词
└── go/
    ├── batch_ssh              # Go 执行器（Linux）
    └── batch_ssh.exe          # Go 执行器（Windows）
```

## 执行流程

```
参数解析 → 安全检查 → 节点读取 → 用户确认
  ↓
┌─ 命令/脚本模式 ─→ gotogo.py 启动 Go 子进程，通过 TCP socket 实时接收进度和结果（Rich 进度条显示）
├─ 上传模式 ─→ transfer.py 逐节点 SFTP 传输，带预检和进度条
└─ 下载模式 ─→ transfer.py 逐节点 SFTP 下载，带预检和进度条
  ↓
结果统计 → 终端输出 → 生成报告(xlsx/txt) → 资源备份 → 创建 latest_history 链接
```

## 历史记录结构

```
historys/
├── sshexec.log                              # 工具运行日志
└── YYYY-MM-DD_HH-MM-SS_模式_备注/           # 每次执行独立目录
    ├── msg.log                              # 执行日志
    ├── output.txt                           # 终端输出（txt）
    ├── output.xlsx                          # 终端输出（xlsx）
    ├── report.txt                           # 汇总报告
    ├── results.xlsx                         # 结果字典（xlsx）
    └── assets/                              # 资源备份
        ├── ip.csv                           # 节点文件
        ├── script.sh                        # 执行脚本
        └── upload_dir/                      # 上传文件
```

## 配置文件

配置文件路径：`src/config/sshexec.ini`，主要配置项：

| 配置段 | 关键参数 | 说明 |
|--------|----------|------|
| `[account]` | port, user, password | CSV 缺省值，密码为 base64 编码 |
| `[execution]` | mode, timeout_* | 执行模式、超时时间 |
| `[enable]` | output_to_xlsx, results_to_xlsx | 输出格式开关 |
| `[paths]` | logs, files, exe, jsons | 各类文件路径 |

## 依赖

Python 3.12+，主要依赖：loguru、pydantic、toml、rich、fabric、openpyxl

## 仓库

- GitHub: [github.com/GH-HYL/Multi-SSHexec](https://github.com/GH-HYL/Multi-SSHexec)
- Gitee: [gitee.com/huang-fugui-123/sshexec](https://gitee.com/huang-fugui-123/sshexec)
- 邮箱: 465317918@qq.com

> **警告：** 该工具可能存在 BUG，请在测试环境验证后再投入使用。数据无价，操作前请再三思量。
