# -*- coding: utf-8 -*-
# SSHFleet 命令构建模块
# 负责命令字符串的拼接、解析与规范化处理

import argparse
import base64
import shlex

from src.log import tlog


def remove_command_fist_last_same_symbol(cmd_str):
    """
    功能：
        去除 command 命令 首尾相同的特殊符号

    参数：
        cmd_str: 命令字符串

    返回：
        removed_symbol: 被移除的特殊符号
        cmd_str: 处理后的命令字符串
    """

    # 特殊符号黑名单，以下符号不移除
    forbidden_chars = r"^$*+?.()[]{}|\/"

    # 判断并处理 , 命令大于一个字符、首尾相同、首尾不是字母或数字、首尾不在特殊符号黑名单中
    if (
        len(cmd_str) > 1
        and cmd_str[0] == cmd_str[-1]
        and not cmd_str[0].isalnum()
        and cmd_str[0] not in forbidden_chars
    ):

        removed_symbol = cmd_str[0]  # 记录被移除的符号
        cmd_str = cmd_str[1:-1]  # 实际移除操作
        return removed_symbol, cmd_str
    else:
        return None, cmd_str


def build_final_command(args: argparse.Namespace) -> str:
    """
    根据参数构建命令字符串

    --nobash 模式：直接返回用户输入的原始命令，不做任何预处理
    默认模式：添加环境变量、sudo 权限，用登录 shell（bash -lc / bash -l）包裹

    Args:
        args: 参数字典

    Returns:
        str: 完整命令字符串
    """

    # --nobash 模式：原样传递给 Go，不做任何处理
    if args.nobash and args.c:
        tlog.info(f"--nobash 模式，原样传递命令: {args.c}")
        return args.c

    # 初始化组件,设置输出编码方式（C.UTF-8是POSIX标准，所有Linux发行版内置支持）
    components = ["LC_ALL=en_US.UTF-8 LANG=en_US.UTF-8;"]

    # sudo
    if args.m == "sudo":
        components.append("sudo")

    # 命令主体
    if args.c:
        safe_command = shlex.quote(args.c)
        components.append(f"bash -lc {safe_command}")

    elif args.s:
        # 脚本解释器选择
        interpreter = "python3" if args.s.endswith(".py") else "bash -l"

        # 读取脚本内容并正确使用heredoc
        with open(args.s, "r", encoding="utf-8") as f:
            script_content = f.read().strip()

        # 编码为base64
        encoded_content = base64.b64encode(script_content.encode("utf-8")).decode(
            "utf-8"
        )

        # 构建命令：使用printf输出base64字符串，然后解码并执行
        # base64编码字符集安全，不包含引号，因此使用单引号包裹
        # printf '%s' 意思是原样输出字符串，不进行转义，确保内容完整传递
        command = f"printf '%s' '{encoded_content}' | base64 -d | {'sudo ' if args.m == 'sudo' else ''}{interpreter}"

        components.append(command)

    else:
        from src.utils import print_error_information_and_exit
        tlog.error("参数出现严重异常，args.c 和 args.s 不能同时为空")
        print_error_information_and_exit(
            "add_env_sudo_to_commands",
            "参数出现严重异常，args.c 和 args.s 不能同时为空",
        )

    # 组合完整命令
    final_command = " ".join(filter(None, components))

    tlog.success(f"完整命令拼接完成: {final_command}")
    return final_command.strip()
