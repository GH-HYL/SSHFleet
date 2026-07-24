# -*- coding: utf-8 -*-
# SSHFleet 参数解析模块

import argparse
import base64
import os
import sys

import src.utils as utils
from src.yaml import SSHFleetConfig


def validate_password_file(file_path: str) -> None:
    """
    验证密码文件的有效性

    Args:
        file_path: 密码文件路径

    Raises:
        SystemExit: 验证失败时退出程序
    """
    import src.utils as utils

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
        description="SSHFleet - 基于 Go 后端的批量 SSH 执行和上传工具",
        formatter_class=argparse.RawTextHelpFormatter,
        usage="\npython3 sshfleet.py  ( -c | -s | -u | -z )  ( -f ) ( -p ) [其他可选参数]\n",
        epilog=(
            "\n示例:\n"
            '  命令模式: python3 sshfleet.py -f nodes.csv -c "ls -l"\n'
            "  脚本模式: python3 sshfleet.py -f nodes.csv -s script.sh\n"
            "  上传模式: python3 sshfleet.py -f nodes.csv -u /local/path -p /remote/path\n"
            "  打包模式: python3 sshfleet.py -z\n"
            "\n上传并发说明:\n"
            "  未指定 -n 时，工具会根据文件大小自动约束并发数（小文件不限，大文件串行）\n"
            "  显式指定 -n 时，工具信任您的选择，仅提示建议值\n"
        ),
    )
    # 三列对齐的帮助信息（argparse自动显示第一列选项名，help只写第二列+第三列）
    help_entries = [
        ('-c', 'command', '(命令模式)', '远程执行命令'),
        ('-s', 'script', '(脚本模式)', '远程执行脚本'),
        ('-u', 'upload', '(上传模式)', '本地上传文件或目录路径'),
        ('-z', None, '(打包模式)', '打包最新日志到当前路径'),
        ('-f', 'csv_file', None, '节点信息的 CSV 文件或内联CSV文本 (-c / -s / -u 时必填)'),
        ('-p', 'path', None, '上传目标路径 (-u 时必填)'),
        ('-m', 'mode', f'[默认: {config.execution.mode or "direct"}]', '执行权限: direct=用户权限, sudo=root权限'),
        ('-t', 'timeout', f'[默认: 命令{config.execution.timeout_execute}s/上传{config.execution.timeout_transfer}s]', '执行或传输超时 (秒)'),
        ('-T', 'timeout', f'[默认: {config.execution.timeout_connect}]', '连接超时 (秒)'),
        ('-n', 'number', '[默认: 节点数]', '并发数。上传模式下未指定时会根据文件大小自动约束'),
        ('-r', 'remark', None, '备注信息, 用于生成历史记录文件名后缀'),
        ('--nobash', None, None, '命令模式: 不使用 bash 环境, 直接执行原始命令'),
        ('--disinteractive', None, None, '跳过交互确认, 直接执行'),
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
    col2_width = max(display_width(tag) if tag else 0 for _, _, tag, _ in help_entries)

    try:
        for opt, metavar, tag, desc in help_entries:
            col2 = tag if tag else ''
            help_str = f'{pad_to_width(col2, col2_width)}  {desc}'
            if metavar:
                parser.add_argument(opt, metavar=metavar, help=help_str)
            else:
                parser.add_argument(opt, action='store_true', help=help_str)
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
