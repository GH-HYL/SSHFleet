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
        - 命令模式（args.c）和脚本模式（args.s）都通过 base64 通道传递，
          base64 字符集安全无 shell 特殊字符，inner_command 完全裸写不带引号，
          外层 shlex.quote 只需在最外层做一次单引号包裹，杜绝 '\"'\"' 引号转义套娃。

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
        # 编码为 base64，通过 stdin 通道交给 sudo bash 执行
        # 关键设计：base64 字符集 [A-Za-z0-9+/=] 与 %s 都不含 shell 特殊字符，
        # 因此 inner_command 中完全不带引号；外层 shlex.quote 只有一层 quote，不再嵌套 '\"'\"' 转义
        encoded_cmd = base64.b64encode(args.c.encode("utf-8")).decode("ascii")
        if args.m == "sudo":
            inner_command = (
                f"{env_prefix} printf %s {encoded_cmd} | base64 -d | sudo bash"
            )
        else:
            inner_command = (
                f"{env_prefix} printf %s {encoded_cmd} | base64 -d | bash"
            )

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
        # base64 字符集安全（[A-Za-z0-9+/=]），不含引号，%s 也不需要 quote，
        # 这样 inner_command 全程裸写无引号，外层 shlex.quote 只有一层
        # sudo 模式直接提升解释器权限执行脚本（| sudo bash / | sudo python3）
        sudo_prefix = "sudo " if args.m == "sudo" else ""
        inner_command = (
            f"{env_prefix} printf %s {encoded_content} | base64 -d | "
            f"{sudo_prefix}{interpreter}"
        )

    else:
        from src.common.error_handler import print_error_information_and_exit
        tlog.error("参数出现严重异常，args.c 和 args.s 不能同时为空")
        print_error_information_and_exit(
            "add_env_sudo_to_commands",
            "参数出现严重异常，args.c 和 args.s 不能同时为空",
        )

    # 统一以登录 shell 包裹，保证命令/脚本均在完整环境变量下执行
    final_command = f"bash -lc {shlex.quote(inner_command)}"

    # 日志交代清楚：原始内容 -> 处理方式 -> 最终命令
    # （命令/脚本均经 base64 编码传递，日志里只显示编码串，需把原始内容一并打印）
    if args.c:
        log_detail = (
            f"  原始命令: {args.c}\n"
            f"  处理方式: 已通过 base64 编码后经管道传递，避免引号转义嵌套\n"
            f"  最终命令: {final_command}"
        )
    elif args.s:
        log_detail = (
            f"  原始脚本: {args.s}（内容已通过 base64 编码后经管道传递）\n"
            f"  最终命令: {final_command}"
        )
    else:
        log_detail = f"  最终命令: {final_command}"

    tlog.success(f"完整命令拼接完成\n{log_detail}")
    return final_command.strip()
