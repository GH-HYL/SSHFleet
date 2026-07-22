# -*- coding: utf-8 -*-
# SSHFleet 核心文件
# 该文件负责定义核心函数和类，包括参数解析、配置加载、任务执行等

import argparse
import base64
import csv
import ipaddress
import os
import re
import shutil

# 系统或第三方模块
import sys
from typing import Dict, List
from collections import Counter
from datetime import datetime
from pathlib import Path
from posixpath import join as posix_join

# 自定义模块
import src.color as color
import src.utils as utils
from src.yaml import SSHFleetConfig
from src.utils import tlog


def validate_password_file(file_path: str) -> None:
    """
    验证密码文件的有效性

    Args:
        file_path: 密码文件路径

    Raises:
        SystemExit: 验证失败时退出程序
    """
    # 1. 检查文件是否存在
    if not os.path.exists(file_path):
        utils.print_error_information_and_exit(
            "validate_password_file",
            f"密码文件不存在：{file_path}"
        )

    # 2. 检查文件是否可读
    try:
        with open(file_path, "r", encoding="utf-8") as f:
            content = f.read().strip()
    except PermissionError:
        utils.print_error_information_and_exit(
            "validate_password_file",
            f"密码文件无法读取：{file_path}"
        )
    except Exception as e:
        utils.print_error_information_and_exit(
            "validate_password_file",
            f"读取密码文件失败：{file_path}\n异常信息：{e}"
        )

    # 3. 检查文件内容是否为空
    if not content:
        utils.print_error_information_and_exit(
            "validate_password_file",
            f"密码文件内容为空：{file_path}"
        )

    # 4. 检查是否为有效的 Base64 编码
    try:
        decoded = base64.b64decode(content)
    except Exception as e:
        utils.print_error_information_and_exit(
            "validate_password_file",
            f"密码文件内容不是有效的 Base64 编码：{file_path}\n异常信息：{e}"
        )

    # 5. 检查解码后是否为空
    if not decoded:
        utils.print_error_information_and_exit(
            "validate_password_file",
            f"密码文件解码后内容为空：{file_path}"
        )


@utils.error_and_exit_handling_decorator("parse_args", "参数解析失败")
def parse_args(config: SSHFleetConfig) -> argparse.Namespace:
    """
    功能：
        参数解析函数

    返回：
        argparse.Namespace: 解析后的参数对象
    """

    parser = argparse.ArgumentParser(
        description="\n基于 Python3.12 的 Paramiko 工具库的批量执行和传输工具\n"
        "\n功能说明:\n"
        "  执行模式: c、s、u、z\n"
        "  模式说明：执行命令、执行脚本、上传文件、打包最新日志\n"
        "\n示例：\n"
        '  命令模式: python3 sshfleet.py -f nodes.csv -c "ls -l"\n'
        "  脚本模式: python3 sshfleet.py -f nodes.csv -s script.sh\n"
        "  上传模式: python3 sshfleet.py -f nodes.csv -u /local/path  -p /remote/path\n"
        "  打包模式: python3 sshfleet.py -z\n",
        formatter_class=argparse.RawTextHelpFormatter,
        usage="\npython3 sshfleet.py  ( -c | -s | -u | -z )  ( -f ) ( -p ) [其他可选参数]\n",
    )
    try:
        parser.add_argument('-c', metavar='command', help='    （命令模式）           远程执行命令')
        parser.add_argument('-s', metavar='script', help='    （脚本模式）           远程执行脚本')
        parser.add_argument('-u', metavar='upload', help='    （上传模式）           本地上传文件或目录路径')
        parser.add_argument('-z', action='store_true', help='    （打包模式）           打包最新日志到当前路径，注意：打包前会删除当前旧打包文件')
        parser.add_argument('-f', metavar='csv_file', help='                           节点信息的 CSV 文件 （ -c 或 -s 或 -u 时必填）')
        parser.add_argument('-p', metavar='path', help='                           上传文件或目录的路径（ -u 时必填）')
        parser.add_argument('-m', metavar='mode', choices=['direct', 'sudo'],default=config.execution.mode,  help=f'     [默认: {config.execution.mode if config.execution.mode else "direct"}]          c、s模式的执行权限 ："direct" 用户权限执行；"sudo" root 权限执行')
        parser.add_argument('-t', metavar='execute_timeout',type=int, help=f'     [默认: {config.execution.timeout_execute if config.execution.timeout_execute  else "60"} 或 {config.execution.timeout_transfer if config.execution.timeout_transfer else "300"} ]    执行超时时间（秒）：c、s模式默认{config.execution.timeout_execute if config.execution.timeout_execute else "60"}；u、p模式{config.execution.timeout_transfer if config.execution.timeout_transfer else "300"}')
        parser.add_argument('-T', metavar='connect_timeout',type=int, default=config.execution.timeout_connect, help=f'     [默认值: {config.execution.timeout_connect if config.execution.timeout_connect else "10"}]          连接超时时间（秒）')
        parser.add_argument('-n', metavar='number', type=int, default=0, help='     [默认：最大]          c、s模式并发执行数，默认使用节点数，可指定并发梳理')
        # parser.add_argument('-k', metavar='key',type=str, nargs='?', const='no_value', default="", help=f'     [默认:不使用]         使用秘钥登陆：-k 指定私钥路径（不指定路径默认：{config.execution.private_key if config.execution.private_key else "~/.ssh/id_rsa"}）')
        parser.add_argument('-r', metavar='remark',type=str, default='', help='                           备注信息，默认自动生成，用于生成历史记录文件名后缀') 
        parser.add_argument('--nobash', action='store_true', help='                           命令模式专用，不使用bash环境执行命令，直接执行原始命令')
        parser.add_argument('--disinteractive', action='store_true', help='                           取消高危命令告警和配置信息的交互确认')
    except Exception as e:
        utils.print_error_information_and_exit(
            "parse_args", f"参数初始化失败\n异常类型：\n{type(e)}\n异常信息：\n{e}"
        )

    # 未提供任何参数，输出帮助信息
    if len(sys.argv) == 1:
        parser.print_help()
        sys.exit(0)

    args = parser.parse_args()

    # 根据模式设置超时时间
    if (args.c or args.s) and args.t is None:
        args.t = config.execution.timeout_execute
    elif args.u and args.t is None:
        args.t = config.execution.timeout_transfer

    # 路径参数规范化
    for path_attr in ["s", "f", "u", "p"]:
        path_value = getattr(args, path_attr, None)
        if path_value:
            # 路径中间不能包含空格,不是路径不能包括空格,不能以空格开头
            if " " in path_value.strip():
                utils.print_error_information_and_exit(
                    "parse_args", "路径参数中间不能包含空格"
                )
            setattr(args, path_attr, utils.args_normalize_path(path_value))

    # # 处理秘钥参数，如果不指定，默认使用家目录下的id_rsa
    # if args.k == "no_value":  # 用户只输入了 -k
    #     default_path = os.path.expanduser(config.execution.private_key)
    #     if os.path.exists(default_path):
    #         args.k = default_path  # 文件存在，使用展开后的路径
    #     else:
    #         args.k = ""  # 文件不存在，置为空

    # 处理备注参数，如果不指定，默认使用空字符串
    if not args.r:  # 用户未输入备注
        if args.c:
            # 取c值的第一个字段内容,如果这个字段长度超过8，则截取前8个字符
            # 截取后的内容只允许包含字母、数字、下划线和短横线
            args.r = (
                args.c.split()[0][:8]
                .replace(" ", "_")
                .replace("/", "_")
                .replace(":", "_")
                .replace("*", "_")
                .replace("?", "_")
                .replace('"', "_")
                .replace("<", "_")
                .replace(">", "_")
                .replace("|", "_")
                .replace("\\", "_")
            )

        elif args.s:
            # 取s路径的文件名部分作为备注
            args.r = os.path.basename(args.s)
        elif args.u:
            # 取u路径的文件名部分作为备注
            args.r = os.path.basename(args.u)

    return args


@utils.error_and_exit_handling_decorator("read_nodes_infos", "读取节点信息失败")
def read_nodes_infos(csv_path: str, config: SSHFleetConfig) -> List[Dict[str, str]]:
    """
    功能：
        从CSV文件中读取节点信息，并进行数据验证和处理

    参数：
        csv_path (str): CSV文件的路径
        config (SSHFleetConfig): 配置对象

    返回：
        List[Dict[str, str]]: 处理后的节点信息列表，每个节点是一个字典，包含ip、port、user和password字段
    """

    # debug=True，跳过IP重复检查
    debug = True

    # 初始化存储字典
    csv_infos = []  # 存储从CSV读取的原始内容
    node_infos = []  # 存储处理后的节点信息
    csv_errors = []  # 存储错误信息

    # 读取CSV文件
    try:
        with open(csv_path, "r", encoding="utf-8") as file:
            reader = csv.reader(file)
            for row in reader:
                # 跳过空行和注释行
                if not row or (row[0].startswith("#") and row[0].strip() != ""):
                    continue
                csv_infos.append(row)
    except Exception as e:
        print(
            f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:read_nodes_infos]{color.COLOR_RESET} 读取文件时发生错误: {e}"
        )
        sys.exit(1)

    try:
        # 第0行0列不是ip格式，移除表头
        ipaddress.ip_address(csv_infos[0][0])  # 尝试解析 IP
    except ValueError:  # 如果不是合法IP，则移除表头
        csv_infos.pop(0)
        print(
            f"{color.COLOR_RED}[INFO]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:read_nodes_infos]{color.COLOR_RESET} 第一行不是IP格式，已移除表头行"
        )

    # 处理端口和用户的默认值及用户输入标志
    port_use_input = False  # 是否使用用户输入的端口值
    port_input_value = None  # 用户输入的端口值
    user_use_input = False  # 是否使用用户输入的用户名值
    user_input_value = None  # 用户输入的用户名值
    password_use_input = False  # 是否使用用户输入的密码值
    password_input_value = None  # 用户输入的密码值

    # 处理每个节点
    for idx, row in enumerate(csv_infos, start=1):
        # 确保行有足够的列
        while len(row) < 4:
            row.append("")

        ip, port, user, password = row[:4]
        errors = []

        # 验证IP地址
        if not ip or ip.strip() == "":
            errors.append("IP必须存在")
        else:
            ip = ip.strip()
            # 简单的IP格式验证
            ip_pattern = r"^(\d{1,3}\.){3}\d{1,3}$"
            if not re.match(ip_pattern, ip):
                errors.append("IP格式不正确")
            # 检查IP是否重复（在当前已处理的节点中）
            if not debug:
                for processed_node in node_infos:
                    if processed_node["ip"] == ip:
                        errors.append("IP地址重复")
                        break
        # 处理端口 - 按照优先级：CSV > config > 用户之前输入的值 > 用户输入新值
        port = port.strip() if port else ""
        if port:  # CSV中有值，最高优先级
            if not port.isdigit() or not (1 <= int(port) <= 65535):
                errors.append("端口必须是1-65535之间的整数，当前值为：" + port)
            else:
                port = int(port)
        elif config.account.port and config.account.port != "None":  # 使用配置的默认值
            port = config.account.port
            # 验证配置的默认端口
            if not str(port).isdigit() or not (1 <= int(port) <= 65535):
                errors.append("配置文件中的默认端口格式错误")
        elif port_use_input:  # 使用之前用户输入的值
            port = port_input_value
        else:  # 请求用户输入新值
            while True:
                try:
                    input_port = input(
                        f"行 {idx} (IP: {ip if ip else '空值'}): 端口为空，请输入端口号: "
                    )
                    if input_port.isdigit() and 1 <= int(input_port) <= 65535:
                        port = input_port
                        # 询问是否将此端口号应用于所有后续端口为空的节点
                        if not port_use_input and idx < len(
                            csv_infos
                        ):  # 检查是否还有后续节点
                            if utils.get_user_confirmation(
                                f"\n{color.COLOR_YELLOW}是否将此端口号应用于所有后续端口为空的节点？{color.COLOR_RESET}",
                                yorn=True,
                            ):
                                port_use_input = True
                                port_input_value = int(port)
                        break
                    else:
                        print("端口必须是1-65535之间的整数")
                except KeyboardInterrupt:
                    print("\n用户取消输入")
                    sys.exit(1)

        # 处理用户名 - 按照优先级：CSV > config > 用户之前输入的值 > 用户输入新值
        user = user.strip() if user else ""
        if user:  # CSV中有值，最高优先级
            pass  # 已经有值，不需要处理
        elif config.account.user and config.account.user != "None":  # 使用配置的默认值
            user = config.account.user
        elif user_use_input:  # 使用之前用户输入的值
            user = user_input_value
        else:  # 请求用户输入新值
            while True:
                try:
                    input_user = input(
                        f"行 {idx} (IP: {ip if ip else '空值'}): 用户名为空，请输入用户名: "
                    )
                    if input_user.strip():
                        user = input_user.strip()
                        # 询问是否将此用户名应用于所有后续用户为空的节点
                        if not user_use_input and idx < len(
                            csv_infos
                        ):  # 检查是否还有后续节点
                            if utils.get_user_confirmation(
                                f"\n{color.COLOR_YELLOW}是否将此用户名应用于所有后续用户为空的节点？{color.COLOR_RESET}",
                                yorn=True,
                            ):
                                user_use_input = True
                                user_input_value = user
                        break
                    else:
                        print("用户名不能为空")
                except KeyboardInterrupt:
                    print("\n用户取消输入")
                    sys.exit(1)

        # 处理密码 - 按照优先级：CSV > config > 用户之前输入的值 > 用户输入新值
        password = password.strip() if password else ""
        if password:  # CSV中有值，最高优先级
            pass  # 已经有值，不需要处理
        elif (
            config.account.password and config.account.password != "None"
        ):  # 使用配置的默认值
            # 验证密码文件有效性
            validate_password_file(config.account.password)
            # 读取密码文件内容并解码base64
            with open(config.account.password, "r", encoding="utf-8") as f:
                password = base64.b64decode(f.read().strip()).decode("utf-8")
        elif password_use_input:  # 使用之前用户输入的值
            password = password_input_value
        else:  # 请求用户输入新值
            # 输出信息时才导入getpass模块，优化不必要的模块导入
            import getpass

            while True:
                try:
                    input_password = getpass.getpass(
                        f"行 {idx} (IP: {ip if ip else '空值'}): 密码为空，请输入密码: "
                    )
                    if input_password:
                        password = input_password
                        # 询问是否将此密码应用于所有后续密码为空的节点
                        if not password_use_input and idx < len(
                            csv_infos
                        ):  # 检查是否还有后续节点
                            if utils.get_user_confirmation(
                                f"\n{color.COLOR_YELLOW}是否将此密码应用于所有后续密码为空的节点？{color.COLOR_RESET}",
                                yorn=True,
                            ):
                                password_use_input = True
                                password_input_value = password
                        break
                    else:
                        print("密码不能为空，请重新输入")
                except KeyboardInterrupt:
                    print("\n用户取消输入")
                    sys.exit(1)

        # 如果有错误，添加到错误列表
        if errors:
            error_msg = "，".join(errors)
            csv_errors.append(f"行 {idx} (IP: {ip if ip else '空值'}): - {error_msg}")
        else:
            # 验证通过，添加到节点列表
            node_infos.append(
                {"ip": ip, "port": port, "user": user, "password": password}
            )

    # 检查是否有错误
    if csv_errors:
        print("发现以下错误:")
        for error in csv_errors:
            print(error)
        sys.exit(1)

    return node_infos


@utils.error_and_exit_handling_decorator("arguments_confirm", "确认执行参数失败")
def arguments_confirm(args, nodes, config=None):
    """
    功能：
        确认执行参数

    参数：
        args: 命令行参数
        nodes: 节点列表
        config: 配置对象（可选，用于上传并发阈值检查）

    返回：
        None
    """

    if args.c:
        # 检测并移除命令的边界符号
        remove_symbol, args.c = utils.remove_command_fist_last_same_symbol(args.c)

    # 未输入并发数，默认使用nodes数量进行并发
    args.n = len(nodes) if args.n == 0 else args.n

    # 上传并发阈值检查
    if args.u and config:
        file_size = _calculate_upload_size(args.u)
        allowed = _check_concurrency_threshold(file_size, config)
        if allowed > 0 and args.n > allowed:
            print(f"{color.COLOR_YELLOW}⚠️ 上传文件总大小 {utils.format_size(file_size)}，建议并发数调整为 {allowed}（当前为 {args.n}）{color.COLOR_RESET}")
            print(f"{color.COLOR_YELLOW}提示：大文件高并发带宽或本地压力过大{color.COLOR_RESET}")
            if utils.get_user_confirmation(
                f"是否将并发数调整为 {allowed}？",
                yorn=True
            ):
                args.n = allowed
            else:
                print(
                    f"{color.COLOR_RED}[WARNING] 用户已拒绝并发执行建议！"
                    f"请确认你充分理解并发操作的风险后再继续！{color.COLOR_RESET}"
                )

    # 非交互模式
    if args.disinteractive:
        print(
            f"{color.COLOR_YELLOW} [非交互模式] 跳过执行参数确认环节，直接执行{color.COLOR_RESET}\n"
        )
        return

    # 构建标题横幅
    title = "           SSHFleet - 执行参数确认           "
    border = "═" * (len(title) + 10)

    print(f"\n{color.COLOR_CYAN}╔{border}╗{color.COLOR_RESET}")
    print(f"{color.COLOR_CYAN}║  {title}  ║{color.COLOR_RESET}")
    print(f"{color.COLOR_CYAN}╚{border}╝{color.COLOR_RESET}\n")

    # 1.构建显示信息的表格
    info_table = [("权限类型", args.m)]

    # 2. 填充参数信息
    if args.c:
        if remove_symbol:
            print(
                f"{color.COLOR_YELLOW}▶ 重要提示: {color.COLOR_RESET}系统检测并移除了命令的边界符号 [ {color.COLOR_RED}{remove_symbol}{color.COLOR_RESET} ] \n"
            )
        info_table.append(("执行模式", "命令模式"))
        info_table.append(("执行命令", args.c))
        info_table.append(("", ""))
    if args.s:
        info_table.append(("执行模式", "脚本模式"))
        info_table.append(("脚本路径", args.s))
        info_table.append(("", ""))
    if args.u:
        info_table.append(("执行模式", "上传模式"))
        info_table.append(("本地路径", args.u))
        info_table.append(("远程路径", args.p))
        info_table.append(("", ""))

    info_table.append(("CSV文件路径", args.f))
    info_table.append(("节点数量", len(nodes)))
    # if args.c or args.s:
    info_table.append(("并发数值", args.n))
    info_table.append(("", ""))

    # 3. 超时设置
    if args.T:
        info_table.append(("连接超时", f"{args.T}s"))
    if args.t:
        if args.c or args.s:
            info_table.append(("执行超时", f"{args.t}s"))
        if args.u:
            info_table.append(("传输超时", f"{args.t}s"))

    # 备注信息
    if args.r:
        info_table.append(("备注信息", args.r.strip()))

    # 打印信息表格
    max_label_len = max(len(label) for label, _ in info_table if label)  # 基础字符长度

    for label, value in info_table:
        if not label and not value:
            print()
        else:
            # 动态计算中英文混合的实际显示宽度差
            chinese_count = sum(1 for c in label if "\u4e00" <= c <= "\u9fff")
            english_count = len(label) - chinese_count
            real_width = english_count + chinese_count * 2

            # 计算需要补偿的空格数（关键调整：减去基础len已包含的1单位宽度）
            padding = max_label_len - len(label) + (real_width - len(label)) - 1

            display_label = label + " " * padding
            print(
                f"{color.COLOR_GREEN}▶ {display_label}-→   {color.COLOR_RESET}{color.COLOR_MAGENTA}{value}{color.COLOR_RESET}"
            )

    # 4. 显示-p参数内容 (如果有)
    if args.u:
        print(
            f"\n{color.COLOR_YELLOW}📁 上传文件/目录内容 (-u 参数):{color.COLOR_RESET}"
        )
        try:
            u_path = Path(args.u)
            if u_path.is_file():
                print(f"└── {u_path.name} (文件)")
            elif u_path.is_dir():
                print(f"└── {u_path.name}/")
                for i, item in enumerate(u_path.iterdir()):
                    prefix = (
                        "    ├──" if i < len(list(u_path.iterdir())) - 1 else "    └──"
                    )
                    if item.is_dir():
                        print(f"{prefix} {item.name}/")
                    else:
                        print(f"{prefix} {item.name}")
        except Exception as e:
            print(
                f"{color.COLOR_YELLOW}[警告] 无法解析路径内容: {e}{color.COLOR_RESET}"
            )
            sys.exit(1)

    # 5. 获取用户确认
    print("\n" + "═" * 60)
    if not utils.get_user_confirmation(
        f"\n{color.COLOR_YELLOW}是否执行上述参数？{color.COLOR_RESET}", yorn=True
    ):
        print(f"{color.COLOR_YELLOW}操作已取消{color.COLOR_RESET}")
        tlog.warning("执行已取消，SSHFleet工具已退出")
        sys.exit(1)
    print(f"SSHFleet工具{color.COLOR_BLUE}开始执行{color.COLOR_RESET}......")
    print(f"{'=' * 50}")


def _calculate_upload_size(file_path: str) -> int:
    """计算上传文件大小：单文件取大小，目录取总大小"""
    if os.path.isfile(file_path):
        return os.path.getsize(file_path)
    total = 0
    for root, dirs, files in os.walk(file_path):
        for f in files:
            fp = os.path.join(root, f)
            if os.path.isfile(fp) and not os.path.islink(fp):
                total += os.path.getsize(fp)
    return total


def _check_concurrency_threshold(file_size: int, config) -> int:
    """根据文件大小返回建议并发数，0 = 不限制。按配置顺序遍历"""
    t = config.upload.concurrency_thresholds
    if file_size < t.small_file:
        return 0
    elif file_size > t.large_file:
        return 1
    else:
        return t.medium_concurrency


@utils.error_and_exit_handling_decorator(
    "save_execute_resource_files", "保存执行资源文件失败", isexit=True
)
def save_execute_resource_files(
    args: argparse.Namespace, log_dir: str, config: SSHFleetConfig
) -> None:
    """
    功能：
        保存执行资源文件（脚本、上传文件、配置文件）到指定目录。

    参数：
        args: 命令行参数
        log_dir: 日志目录路径
        config: 配置对象

    返回：
        None
    """
    # 创建资源存放目录
    resources_dir = posix_join(log_dir, config.paths.files.asset)
    os.makedirs(resources_dir, exist_ok=True)

    # 处理 -s 参数（单个文件）
    if hasattr(args, "s") and args.s:
        src_path = Path(args.s)
        if src_path.exists():
            dst_path = Path(resources_dir) / src_path.name
            shutil.copy2(src_path, dst_path)  # 保留元数据复制

    # 处理 -f 参数（单个文件）
    if hasattr(args, "f") and args.f:
        src_path = Path(args.f)
        if src_path.exists():
            dst_path = Path(resources_dir) / src_path.name
            shutil.copy2(src_path, dst_path)
    tlog.success("保存执行资源文件成功")

@utils.error_and_exit_handling_decorator(
    "results_statistics", "计算统计结果信息失败", isexit=True
)
def results_statistics(
    results: List,
    node_infos: List,
    args: argparse.Namespace,
    global_start_time: datetime,
    global_stop_time: datetime,
) -> Dict:
    """
    功能：
        计算统计结果信息

    参数：
        results: 结果列表
        node_infos: 节点信息列表
        args: 命令行参数
        global_start_time: 全局开始时间
        global_stop_time: 全局结束时间

    返回：
        包含所有统计结果信息的字典
    """

    # 基本统计
    results_total = len(results)
    nodeinofs_total = len(node_infos)

    # 成功/失败统计
    exit_counts = Counter(d.get("exit_bool") for d in results)
    success_counts = exit_counts.get(True, 0)
    fail_counts = exit_counts.get(False, 0)

    # 总数校验
    verify = "通过" if nodeinofs_total == results_total else "异常"
    verify_color = (
        color.COLOR_GREEN if nodeinofs_total == results_total else color.COLOR_RED
    )

    # 分类统计
    category_counts = Counter(d.get("result_category") for d in results)

    # 根据模式移除成功分类
    if args.u:
        success_category = "传输成功"
        category_counts.pop(success_category, None)
    elif args.c or args.s:
        success_category = "执行成功"
        category_counts.pop(success_category, None)

    # 按数量正（倒）序排序（reverse逆转）失败分类
    sorted_fail_categories = sorted(
        category_counts.items(), key=lambda x: x[1], reverse=True
    )

    # 按分类分组并收集IP地址
    category_ip_map = {}
    for result in results:
        category = result.get("result_category", "未知")
        ip = result.get("ip", "未知IP")
        if category not in category_ip_map:
            category_ip_map[category] = []
        category_ip_map[category].append(ip)

    # 分离成功分类IP
    if "success_category" not in locals():
        tlog.error("success_category 未定义，未知执行模式")
        success_category = "未知"
    success_ips = category_ip_map.pop(success_category, [])

    # 对每个分类的IP进行数字排序
    for category in category_ip_map:
        category_ip_map[category] = sorted(
            category_ip_map[category],
            key=lambda ip: [int(part) for part in ip.split(".")],
        )

    # 对成功IP进行数字排序
    sorted_success_ips = (
        sorted(success_ips, key=lambda ip: [int(part) for part in ip.split(".")])
        if success_ips
        else []
    )

    # 构建统计结果字典
    statistics = {
        "results_total": results_total,
        "nodeinofs_total": nodeinofs_total,
        "verify": verify,
        "verify_color": verify_color,
        "success_counts": success_counts,
        "fail_counts": fail_counts,
        "sorted_fail_categories": sorted_fail_categories,
        "success_category": (
            success_category if "success_category" in locals() else None
        ),
        "success_ips_count": len(success_ips),
        "sorted_success_ips": sorted_success_ips,
        "category_ip_map": category_ip_map,
        "global_start_time": global_start_time,
        "global_stop_time": global_stop_time,
        "global_cost_time": round(
            (global_stop_time - global_start_time).total_seconds(), 2
        ),
    }
    tlog.success("计算统计结果信息成功")

    return statistics


@utils.error_and_exit_handling_decorator("zip_latest_history", "打包历史记录失败")
def zip_latest_history(args, config: SSHFleetConfig):
    """
    功能：
        打包最新的历史记录日志文件为 ZIP 或 TAR 格式

    参数：
        args: 命令行参数

    返回：
        None
    """

    history_dir = config.paths.logs.historys
    zip_dir = config.paths.logs.zip

    # -z 参数需要单独使用，与所有其他参数互斥
    if args.z:
        if len(sys.argv) > 2:
            print(
                f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:zip_latest_history]{color.COLOR_RESET} -z 模式不能与其他参数一起使用",
                file=sys.stderr,
            )
            sys.exit(1)

    if not os.path.isdir(history_dir):
        print(
            f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:zip_latest_history]{color.COLOR_RESET} 历史记录目录 '{history_dir}' 不存在",
            file=sys.stderr,
        )
        print("提示: 请先至少一次执行任务以生成历史记录")
        sys.exit(1)

    log_dirs = [
        d for d in os.listdir(history_dir) if os.path.isdir(posix_join(history_dir, d))
    ]
    if not log_dirs:
        print(
            f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:zip_latest_history]{color.COLOR_RESET} 历史记录目录 '{history_dir}' 中没有日志文件夹",
            file=sys.stderr,
        )
        print("提示: 请先至少一次执行任务以生成历史记录")
        sys.exit(1)

    # 按目录名排序，假设目录名格式为 log-YYYYMMDD_HHMMSS
    log_dirs.sort(reverse=True, key=lambda x: x.replace("_", ""))
    latest_log_dir = log_dirs[0]

    # 拼接压缩包名称
    working_dir = os.getcwd()
    package_name = f"log-{latest_log_dir}"

    # 如果配置文件为空或不存在，打包路径存放在工作路径下，过存在，检查是绝对路径还是相对路径，按照路径配置生成
    if zip_dir:
        # 格式化路径，确保使用正斜杠
        default_zip_dir = utils.args_normalize_path(zip_dir)

        # 检查是否是绝对路径
        if os.path.isabs(default_zip_dir):
            zip_dir = default_zip_dir
        else:
            zip_dir = posix_join(working_dir, default_zip_dir)
    else:
        zip_dir = working_dir

    # 检查打包路径是否存在，不存在则创建
    if not os.path.exists(zip_dir):
        utils.get_user_confirmation(
            f"{zip_dir} \n 配置文件的默认打包路径不存在，是否创建？", yorn=True
        )
        os.makedirs(zip_dir)

    zip_path = posix_join(zip_dir, f"{package_name}.zip")
    tar_path = posix_join(zip_dir, f"{package_name}.tar")

    # 筛选已存在的 ZIP 文件并删除
    zip_pattern = re.compile(r"log-\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2}_.*\.zip")
    existing_zip_files = [f for f in os.listdir(zip_dir) if zip_pattern.match(f)]
    for f in existing_zip_files:
        file_path = posix_join(zip_dir, f)
        os.remove(file_path)
        print(f"已清理当前路径下旧打包文件 {f}")

    # 筛选已存在的 TAR 文件并删除
    tar_pattern = re.compile(r"log-\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2}_.*\.tar")
    existing_tar_files = [f for f in os.listdir(zip_dir) if tar_pattern.match(f)]
    for f in existing_tar_files:
        file_path = posix_join(zip_dir, f)
        os.remove(file_path)
        print(f"已清理当前路径下旧打包文件 {f}")

    # 尝试使用 ZIP 打包
    try:
        import zipfile

        with zipfile.ZipFile(zip_path, "w", zipfile.ZIP_STORED) as zipf:
            source_dir = posix_join(history_dir, latest_log_dir)
            for root, _, files in os.walk(source_dir):
                for file in files:
                    file_path = posix_join(root, file)
                    arcname = os.path.relpath(file_path, source_dir)
                    zipf.write(file_path, arcname)
        print(f"ZIP打包成功: {os.path.basename(zip_path)}")
        return
    except Exception as e:
        print(f"ZIP打包失败: {str(e)}", file=sys.stderr)

    # ZIP 打包失败，尝试使用 TAR 打包
    try:
        import tarfile

        source_dir = posix_join(history_dir, latest_log_dir)
        with tarfile.open(tar_path, "w") as tar:
            tar.add(source_dir, arcname=os.path.basename(source_dir))
        print(f"TAR打包成功: {os.path.basename(tar_path)}")
    except Exception as e:
        print(f"TAR打包失败: {str(e)}", file=sys.stderr)
