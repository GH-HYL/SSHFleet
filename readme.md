# SSHexec SSH异步多协程执行的运维工具

SSHexec是一款企业级？SSH批量运维工具，基于Python 3.14开发，集成了命令和脚本执行（多协程，支持数千级并发量）、文件上传下载、安全审计等多项功能，专为大规模服务器运维场景设计，支持Windows和Linux双系统。

## 1. 多模式执行引擎

- 命令模式：执行shell命令
- 脚本模式：执行本地脚本
- 上传模式：上传文件到多个节点
- 下载模式：从多节点批量下载文件
- 打包模式：将最新的历史记录打包

## 2. 智能安全防护

- 危险命令检测（内置50+种危险规则）
- 权限风险预警
- 交互式安全确认机制
- 丰富的高危操作提醒逻辑

## 3. 高级文件传输

- 支持SFTP传输协议上传和下载
- 自动目录创建与权限管理
- 自动检测目标空间大小和可写性
- 大文件分块传输与断点续传

## 4. 专业级日志系统

- 实时执行日志输出
- 留档种类丰富（工具日志、执行日志、结果字典、结果报告、执行记录、过程材料）
- 执行记录分三种输出：终端输出、txt、xlsx
- 多维度分类归纳（按成功/失败类型，失败类型有更具体的分类）
- 自动生成报告
- 日志自动归档
- 耗时统计

## 5. 企业级特性

- 多协程并发控制（支持数千级节点并发）
- 超时熔断机制（连接/命令/传输独立超时）
- 断连自动重试
- 系统终端信号安全处理
- 资源占用监控
- 支持ctrl-c打断运行

# 以上全是废话，开始干货

## 6. 代码架构

主体结构：

```text
项目目录/
├── sshexec.py                    # 主程序
└── src/                          # 源代码文件夹
    ├── go/                       # go语言的执行器文件夹
    │   ├── batch_ssh             # 执行器（linux可执行文件）
    │   ├── batch_ssh.exe         # 执行器（windows可执行文件）
    │   └── go.py                 # 执行器入口
    ├── transfer/                 # 传输模块
    │   ├── transfer_check.py     # 传输检查文件
    │   ├── transfer_utils.py     # 传输工具文件
    │   └── transfer.py           # 传输文件
    ├── config/                   # 配置文件夹
    │   ├── sshexec.ini           # 配置文件
    │   ├── dangerous_class.json  # 危险命令分类正则文件
    │   └── error_class.json      # 错误分类关键词文件
    ├── check.py                  # 检查文件
    ├── color.py                  # 颜色文件
    ├── core.py                   # 核心代码文件
    ├── dicts.py                  # 字典文件
    ├── output.py                 # 打印结果文件
    ├── toml.py                   # 配置读取文件
    └── utils.py                  # 工具文件
```

日志结构：

```txt
historys/
├── sshexec.log                      # 工具日志
└── YYYYMMDD_HHMMSS_执行模式_备注/    # 每次执行独立目录
    ├── msg.log                      # 执行日志  
    ├── output.txt                   # 输出记录（txt版）
    ├── output.xlsx                  # 输出记录（xlsx梳理版）
    ├── report.txt                   # 汇总报告
    ├── results_dict.xlsx            # 结果字典（xlsx版）
    └── assets/                      # 过程材料备份
        ├──ip.csv                    # 节点文件
        ├──my.sh                     # 执行脚本
        └──mypackage                 # 上传文件备份
```

## 7. 详细使用指南

### 基础执行模式

1. 命令模式：

   ```bash
   python3 sshexec.py -f nodes.csv -c "ls -l"
   ```
2. 脚本模式：

   ```bash
   python3 sshexec.py -f nodes.csv -s script.sh
   ```
3. 上传模式：

   ```bash
   python3 sshexec.py -f nodes.csv -u /local/path -p /remote/path
   ```
4. 下载模式：

   ```bash
   python3 sshexec.py -f nodes.csv -d /remote/path -p /local/path
   ```
5. 打包模式：

   ```bash
   python3 sshexec.py -z
   ```

## 8. 参数详解

```text
optional arguments:
  -h, --help          show this help message and exit
  -c command               （命令模式）           远程执行命令
  -s script                （脚本模式）           远程执行脚本
  -u upload                （上传模式）           本地上传文件或目录路径
  -d download              （下载模式）           远程下载文件或目录路径（绝对路径）
  -z                       （打包模式）           打包最新日志到当前路径，注意：打包前会删除当前旧打包文件
  -f csv_file                                    节点信息的 CSV 文件 （ -c 或 -s 或 -u 或 -d 时必填）
  -p path                                        上传文件或目录的路径、下载文件存放的目录路径（ -u 或 -d 时必填）
  -m mode                  [默认: sudo]           c、s模式的执行权限 ：“direct” 用户权限执行；“sudo” root 权限执行
  -t execute_timeout       [默认: 60 或 300 ]     执行超时时间（秒）：c、s模式默认60；u、p模式300
  -T connect_timeout       [默认值: 10]           连接超时时间（秒）
  -n number                [默认：最大]           c、s模式并发执行数，默认使用节点数，可指定并发梳理
  -e env                                         环境变量，默认为空，可在配置文件进行配置 （ -c 或 -s 专用，可选填）
  -k [key]                 [默认:不使用]          使用秘钥登陆：-k 指定私钥路径（不指定路径默认：~/.ssh/id_rsa）
  -r remark                                      备注信息，默认自动生成，用于生成历史记录文件名后缀
  --disinteractive                               取消高危命令告警和配置信息的交互确认
```

## 9. csv文件格式详解

标准格式：IP,端口,用户名,密码

示例：

```txt
   192.168.1.10,10022,root,P@ssw0rd   # csv的四列全部配置，但要注意明文密码存在风险

   192.168.1.10                       # 极简格式，端口号、用户、密码使用默认配置，详见./src/config/sshexec.ini

   192.168.1.10,10022,root,           # 密码为空时，密码默认值也未配置时，可以在执行时，在交互模式下，输入密码回车提交
```

注意：./src/config/sshexec.ini的默认密码为密码的base64编码（避免明文存储密码）

## 10. 安全规则示例（错误分类规则同理，不再赘述）

工具内置的部分安全检测规则（错误分类规则），可酌情增删改，详见./src/dict.py：

```python
DANGEROUS_PATTERNS = [
    {'name': '递归强制删除根目录', 'example': 'rm -rf /', 'regex': r'^\s*rm\s+(-[rf]+|--no-preserve-root|-(?:r\s+f|f\s+r))\s*/\s*(?:#.*)?$', 'risk_level': 'forbidden'},
    {'name': '递归强制删除系统目录', 'example': 'rm -rf /etc', 'regex': r'^\s*rm\s+(-[rf]+|-(?:r\s+f|f\s+r))\s*/(?:etc|usr|var|lib|bin|sbin)(?:/.*)?\s*(?:#.*)?$', 'risk_level': 'forbidden'},
    {'name': '格式化根磁盘', 'example': 'mkfs /dev/sda', 'regex': r'^\s*mkfs\s+/(dev/)?(sd|hd|vd)[a-z]+\s*(?:#.*)?$', 'risk_level': 'forbidden'},
    # ...共50+条规则
]
```

## 11.联系方式

GitHub仓库：[github.com/GH-HYL/Multi-SSHexec](https://github.com/GH-HYL/Multi-SSHexec)

Gitee仓库：[SSHexec-Async SSH异步多协程执行的运维工具: SSHexec-Async SSH异步多协程执行的运维工具，支持数千级目标节点并发执行命令、脚本（shell、python）执行，同时还支持包括上传文件和下载文件的功能](https://gitee.com/huang-fugui-123/sshexec)

技术交流邮箱：465317918@qq.com

# 警告：

该工具可能存在BUG，请在测试环境测试验证后，再投入使用

数据无价，操作前请再三思量
