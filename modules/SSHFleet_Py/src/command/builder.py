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
    默认模式：语言变量、sudo 权限与命令统一收进登录 shell（bash -lc）内执行

    Args:
        args: 参数字典

    Returns:
        str: 完整命令字符串
    """

    # --nobash 模式：原样传递给 Go，不做任何处理
    if args.nobash and args.c:
        tlog.info(f"--nobash 模式，原样传递命令: {args.c}")
        return args.c

    # 语言变量前缀：收进 login shell 内部，export 确保对内部所有命令及子进程生效
    # （C.UTF-8是POSIX标准，所有Linux发行版内置支持）
    env_prefix = "export LC_ALL=en_US.UTF-8 LANG=en_US.UTF-8;"

    # 命令主体：语言变量、sudo 权限与命令统一收进登录 shell（bash -lc）内部，
    # bash -lc 在完整环境里定位 sudo 并执行，无任何组件裸露在极简 PATH 下
    if args.c:
        # 命令模式：sudo 模式由 sudo 提升 bash -c 的权限执行原始命令
        if args.m == "sudo":
            inner_command = f"{env_prefix} sudo bash -c {shlex.quote(args.c)}"
        else:
            inner_command = f"{env_prefix} {args.c}"

    elif args.s:
        # 脚本解释器选择（内层解释器）
        interpreter = "python3" if args.s.endswith(".py") else "bash"

        # 读取脚本内容并正确使用heredoc
        with open(args.s, "r", encoding="utf-8") as f:
            script_content = f.read().strip()

        # 编码为base64
        encoded_content = base64.b64encode(script_content.encode("utf-8")).decode(
            "utf-8"
        )

        # 内层命令：printf输出base64字符串，解码后交给解释器执行
        # base64编码字符集安全，不包含引号，因此使用单引号包裹
        # printf '%s' 意思是原样输出字符串，不进行转义，确保内容完整传递
        # sudo 模式直接提升解释器权限执行脚本（| sudo bash / | sudo python3）
        sudo_prefix = "sudo " if args.m == "sudo" else ""
        inner_command = (
            f"{env_prefix} printf '%s' '{encoded_content}' | base64 -d | "
            f"{sudo_prefix}{interpreter}"
        )

    else:
        from src.utils import print_error_information_and_exit
        tlog.error("参数出现严重异常，args.c 和 args.s 不能同时为空")
        print_error_information_and_exit(
            "add_env_sudo_to_commands",
            "参数出现严重异常，args.c 和 args.s 不能同时为空",
        )

    # 统一以登录 shell 包裹，保证命令/脚本均在完整环境变量下执行
    final_command = f"bash -lc {shlex.quote(inner_command)}"

    tlog.success(f"完整命令拼接完成: {final_command}")
    return final_command.strip()
