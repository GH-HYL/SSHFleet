# -*- coding: utf-8 -*-
# SSHFleet 参数检查模块

import os
import re

from src.check.files import check_script_file
import src.utils as utils


@utils.error_and_exit_handling_decorator("check_arguments", "参数合规性检查失败")
def check_arguments(args):
    """检查命令行参数的有效性"""

    # 检查互斥执行模式参数
    lock_args = [args.c, args.s, args.u, args.z]
    lock_args_count = sum(1 for arg in lock_args if arg)
    if lock_args_count != 1:
        utils.print_error_information_and_exit(
            "check_arguments", " 执行模式参数：-c、-s、-u、-z 互斥，只能指定一个"
        )

    # 检查 -p 参数
    if args.p:
        if args.u:
            if not args.p.startswith("/"):
                utils.print_error_information_and_exit(
                    "check_arguments",
                    f" 上传模式：-p 参数指定的上传目录必须是绝对路径，当前值：{args.p}",
                )
            if not args.p.endswith("/"):
                utils.print_error_information_and_exit(
                    "check_arguments",
                    f" 上传模式：-u 是目录的情况下 -p 参数指定的必须是目录（以/结尾），-p 当前值：{args.p}",
                )

        elif args.c or args.s:
            # -p 不能与 -c 或 -s 同时使用
            utils.print_error_information_and_exit(
                "check_arguments",
                " -p 参数不能搭配 -c 或 -s 使用，请单独使用 -u 参数指定上传文件或目录后再使用 -c 或 -s",
            )

    # 检查 -u 参数
    if args.u:
        if not args.p:
            utils.print_error_information_and_exit(
                "check_arguments", " -u 参数必须搭配 -p 参数使用"
            )
        if not os.path.exists(args.u):
            utils.print_error_information_and_exit(
                "check_arguments", f" -u 参数指定的上传文件或目录不存在：{args.u}"
            )
        if os.path.islink(args.u):
            utils.print_error_information_and_exit(
                "check_arguments",
                f" -u 参数指定的上传文件或目录是符号链接，不能上传：{args.u}",
            )
        if os.path.isdir(args.u):
            # 递归检查整个目录树
            has_real_file = False
            for root, dirs, files in os.walk(args.u):
                for file in files:
                    file_path = os.path.join(root, file)
                    if os.path.isfile(file_path) and not os.path.islink(file_path):
                        has_real_file = True
                        break  # 找到一个就退出内层循环
                if has_real_file:
                    break  # 找到一个就退出外层循环

            if not has_real_file:
                utils.print_error_information_and_exit(
                    "check_arguments",
                    f"-u 参数指定的上传目录及其所有子目录中都没有真正的文件（只有符号链接）：{args.u}",
                )

    # 检查 -m 参数
    if args.m:
        if args.m not in ["direct", "sudo"]:
            utils.print_error_information_and_exit(
                "check_arguments",
                f" -m 参数必须是 'direct' 或 'sudo'，当前值：{args.m}",
            )

    # 检查 -c 参数
    if args.c:
        if not args.c.strip():
            utils.print_error_information_and_exit(
                "check_arguments", " -c 参数不能为空，请提供要执行的命令"
            )

        first_word = re.findall(r"^[^a-zA-Z]*([a-zA-Z]+)", args.c.strip())
        if first_word and first_word[0] in ["for", "while", "until", "if", "case"]:
            utils.print_error_information_and_exit(
                "check_arguments", " -c 参数不兼容执行循环的命令，请使用脚本模式执行"
            )

    # 检查 -s 参数
    if args.s:
        if os.path.isdir(args.s):
            utils.print_error_information_and_exit(
                "check_arguments", f" -s 参数指定的路径是目录，不是脚本文件：{args.s}"
            )
        script_path = args.s
        if os.path.getsize(script_path) == 0:
            utils.print_error_information_and_exit(
                "check_arguments", f" -s 参数指定的脚本文件为空：{script_path}"
            )
        if not script_path.endswith((".sh", ".py")):
            utils.print_error_information_and_exit(
                "check_arguments",
                f" -s 参数指定的脚本文件扩展名必须是 .sh 或 .py，当前值：{script_path}",
            )
        check_script_file(script_path)

    # 检查 -f 参数
    if args.f:
        if not os.path.exists(args.f):
            # 文件不存在，询问是否作为内联CSV文本传入
            if utils.get_user_confirmation(
                f"[-f] 指定的内容 '{args.f}' 不是有效的文件路径，是否将其作为CSV文本传入",
                disinteractive=getattr(args, 'disinteractive', False),
            ):
                args.f_is_inline = True
            else:
                utils.print_error_information_and_exit(
                    "check_arguments", f" -f 参数指定的文件不存在：{args.f}"
                )
        else:
            # 文件存在，执行原有检查
            args.f_is_inline = False
            if os.path.getsize(args.f) == 0:
                utils.print_error_information_and_exit(
                    "check_arguments", f" -f 参数指定的 CSV 文件为空：{args.f}"
                )
            # 不能是二进制文件
            with open(args.f, "rb") as f:
                if b"\x00" in f.read(1024):
                    utils.print_error_information_and_exit(
                        "check_arguments", f" 错误: {args.f} 是二进制文件"
                    )

    # 检查 -n 参数
    if args.n:
        if not isinstance(args.n, int) or args.n <= 0:
            utils.print_error_information_and_exit(
                "check_arguments",
                f" -n 参数格式错误，并发连接数必须是正整数，当前值：{args.n}",
            )

    # # 检查 -k 参数
    # if args.k:
    #     if not os.path.isfile(args.k):
    #         utils.print_error_information_and_exit(
    #             "check_arguments", f" -k 指向的秘钥文件不存在，请检查路径：{args.k}"
    #         )

    # 检查 --disinteractive 参数
    if args.disinteractive and args.z:
        utils.print_error_information_and_exit(
            "check_arguments", " --disinteractive 参数不能与 -z 一起使用"
        )

    # 检查 -t 参数, 必须是 int 类型
    if args.t:
        if not isinstance(args.t, int) or args.t <= 0:
            utils.print_error_information_and_exit(
                "check_arguments",
                f" -t 参数格式错误，命令或传输超时时间必须是正整数，当前值：{args.t}",
            )

    if args.T:
        if not isinstance(args.T, int) or args.T <= 0:
            utils.print_error_information_and_exit(
                "check_arguments",
                f" -T 参数格式错误，连接超时时间必须是正整数，当前值：{args.T}",
            )
