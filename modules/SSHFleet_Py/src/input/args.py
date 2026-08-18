# -*- coding: utf-8 -*-
# SSHFleet 参数解析模块

import argparse
import base64
import os
import sys

from src.common.error_handler import error_and_exit_handling_decorator, print_error_information_and_exit
from src.common.text_utils import args_normalize_path
from src.common.loader import SSHFleetConfig


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
        print_error_information_and_exit(
            "validate_password_file",
            f"密码文件不存在：{file_path}"
        )

    # 2. 检查文件是否可读
    try:
        with open(file_path, "r", encoding="utf-8") as f:
            content = f.read().strip()
    except PermissionError:
        print_error_information_and_exit(
            "validate_password_file",
            f"密码文件无法读取：{file_path}"
        )
    except Exception as e:
        print_error_information_and_exit(
            "validate_password_file",
            f"读取密码文件失败：{file_path}\n异常信息：{e}"
        )

    # 3. 检查文件内容是否为空
    if not content:
        print_error_information_and_exit(
            "validate_password_file",
            f"密码文件内容为空：{file_path}"
        )

    # 4. 检查是否为有效的 Base64 编码
    try:
        decoded = base64.b64decode(content)
    except Exception as e:
        print_error_information_and_exit(
            "validate_password_file",
            f"密码文件内容不是有效的 Base64 编码：{file_path}\n异常信息：{e}"
        )

    # 5. 检查解码后是否为空
    if not decoded:
        print_error_information_and_exit(
            "validate_password_file",
            f"密码文件解码后内容为空：{file_path}"
        )

@error_and_exit_handling_decorator("parse_args", "参数解析失败")
def parse_args(config: SSHFleetConfig) -> argparse.Namespace:
    """
    功能：
        参数解析函数

    返回：
        argparse.Namespace: 解析后的参数对象
    """

    parser = argparse.ArgumentParser(
        description="SSHFleet - 基于 Go 后端的批量 SSH 执行和上传工具",
        formatter_class=argparse.RawTextHelpFormatter,
        usage="\npython3 sshfleet.py  ( -c | -s | -u | -d | -z )  ( -f ) ( -p ) [其他可选参数]\n",
        epilog=(
            "\n示例:\n"
            '  命令模式: python3 sshfleet.py -f nodes.csv -c "ls -l"\n'
            "  脚本模式: python3 sshfleet.py -f nodes.csv -s script.sh\n"
            "  上传模式: python3 sshfleet.py -f nodes.csv -u /local/path -p /remote/path\n"
            "  下载模式: python3 sshfleet.py -f nodes.csv -d /remote/path -p /local/path\n"
            "  打包模式: python3 sshfleet.py -z\n"
            "\n上传并发说明:\n"
            "  上传模式下，工具根据配置文件中的文件大小阈值输出建议并发数\n"
            "  输入 y 使用建议值，输入 n 保留原值继续执行\n"
        ),
    )
    # 三列对齐的帮助信息（argparse自动显示第一列选项名，help只写第二列+第三列）
    help_entries = [
        ('-c', 'command', '(命令模式)', '远程在多台服务器上执行一条命令'),
        ('-s', 'script', '(脚本模式)', '远程在多台服务器上执行一个本地脚本'),
        ('-u', 'upload', '(上传模式)', '把本地文件或目录传到服务器'),
        ('-d', 'download', '(下载模式)', '从服务器下载文件或目录到本地'),
        ('-z', None, '(打包模式)', '把最近一次的执行记录打包到当前目录'),
        ('-f', 'csv_file', None, '节点清单：CSV 文件路径，或直接在命令行写一行节点信息 (-c/-s/-u/-d 时必须带)'),
        ('-p', 'path', None, '目标路径：上传到服务器的目录 / 从服务器下载到的本地目录 (-u/-d 时必须带)'),
        ('-m', 'mode', f'[默认: {config.execution.mode or "direct"}]', '执行身份: direct=用登录用户身份, sudo=用 root 身份执行'),
        ('-t', 'timeout', f'[默认: 命令{config.execution.timeout_execute}s/上传{config.execution.timeout_transfer}s]', '单台执行或传输的超时时间 (秒)'),
        ('-T', 'timeout', f'[默认: {config.execution.timeout_connect}]', '连接每台服务器的超时时间 (秒)'),
        ('-n', 'number', '[默认: 同时跑全部节点]', '并发数：同时操作几台服务器 (不填则全部并行)'),
        ('-r', 'remark', None, '给这次任务起个名字，会作为历史记录文件夹的后缀 (不填自动生成)'),
        ('--nobash', None, None, '命令模式专用: 不套一层 bash 环境，直接执行原始命令'),
        ('--disinteractive', None, None, '跳过所有确认提示直接执行 (批量跑脚本时常用)'),
        ('-k', 'KEY_PATH', '(密钥登录)', '不指定=纯密码; 仅 -k=用CSV/配置默认密钥; -k 路径=所有节点统一私钥', '?'),
    ]

    def display_width(s):
        """计算字符串显示宽度（中文字符占2列）"""
        import unicodedata
        w = 0
        for c in s:
            if unicodedata.east_asian_width(c) in ('F', 'W'):
                w += 2
            else:
                w += 1
        return w

    def pad_to_width(s, target_width):
        """按显示宽度填充空格"""
        return s + ' ' * (target_width - display_width(s))

    # 计算第二列最大显示宽度
    col2_width = max(display_width(item[2]) if item[2] else 0 for item in help_entries)

    try:
        for item in help_entries:
            opt, metavar, tag, desc = item[0], item[1], item[2], item[3]
            nargs = item[4] if len(item) > 4 else None
            col2 = tag if tag else ''
            help_str = f'{pad_to_width(col2, col2_width)}  {desc}'
            if nargs == '?':
                parser.add_argument(opt, metavar=metavar, nargs='?', const='no_value', default='', help=help_str)
            elif metavar:
                parser.add_argument(opt, metavar=metavar, help=help_str)
            else:
                parser.add_argument(opt, action='store_true', help=help_str)
    except Exception as e:
        print_error_information_and_exit(
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
    elif args.d and args.t is None:
        args.t = config.execution.timeout_transfer

    # 连接超时默认值（未指定 -T 时使用配置的 timeout_connect）
    if args.T is None:
        args.T = config.execution.timeout_connect

    # 数值参数统一转为 int（argparse 默认返回字符串，check_arguments 按 int 校验）
    for attr in ["t", "T", "n"]:
        val = getattr(args, attr, None)
        if val is not None and not isinstance(val, int):
            try:
                setattr(args, attr, int(val))
            except (ValueError, TypeError):
                pass  # 非法值留给 check_arguments 校验报错

    # 未指定 -m 时使用配置的默认执行模式（参考老代码 default=config.execution.mode）
    if args.m is None:
        args.m = config.execution.mode

    # 未指定 -n 时默认 0（参考老代码 default=0，确认阶段替换为节点数）
    if args.n is None:
        args.n = 0

    # 路径参数规范化
    for path_attr in ["s", "f", "u", "p", "d"]:
        path_value = getattr(args, path_attr, None)
        if path_value:
            # 路径中间不能包含空格,不是路径不能包括空格,不能以空格开头
            if " " in path_value.strip():
                print_error_information_and_exit(
                    "parse_args", "路径参数中间不能包含空格"
                )
            setattr(args, path_attr, args_normalize_path(path_value))

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
        elif args.d:
            # 取d路径的文件名部分作为备注
            args.r = os.path.basename(args.d)

    return args
