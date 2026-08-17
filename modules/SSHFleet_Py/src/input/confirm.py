# -*- coding: utf-8 -*-
# SSHFleet 执行参数确认模块

import os
import sys
from pathlib import Path

import src.common.constants as color
from src.common.error_handler import error_and_exit_handling_decorator
from src.common.text_utils import format_size
from src.input.interaction import get_user_confirmation
from src.log import tlog


def _check_upload_concurrency(args, config) -> None:
    """按配置阈值输出上传并发建议，用户确认后应用

    建议规则（见 SSHFleet.yaml 中 upload.concurrency_thresholds）：
    - file_size < small_file: 全并发（0 = 不限制，无需建议）
    - file_size > large_file: 串行（并发=1）
    - 两者之间: 最多 medium_concurrency 并发

    交互规则：
    - 输入 y：使用建议并发数
    - 输入 n：保留原值继续执行（不退出）
    """
    if not args.u or not config:
        return

    file_size = _calculate_upload_size(args.u)
    allowed = _check_concurrency_threshold(file_size, config)

    # 小文件不限制并发，无需建议
    if allowed == 0:
        return

    print(
        f"{color.COLOR_YELLOW}上传文件总大小 {format_size(file_size)}，"
        f"建议并发数为 {allowed}{color.COLOR_RESET}"
    )
    if get_user_confirmation(
        f"是否使用建议并发数 {allowed} ？",
        yorn=True,
        disinteractive=getattr(args, 'disinteractive', False),
    ):
        args.n = allowed
    # 输入 n：保留原值（未指定 -n 时后续默认使用节点数），继续执行


def _build_info_table(args, nodes):
    """构建显示信息的表格数据"""
    info_table = [("权限类型", args.m)]

    # 填充参数信息
    if args.c:
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
    if args.d:
        info_table.append(("执行模式", "下载模式"))
        info_table.append(("远程路径", args.d))
        info_table.append(("本地路径", args.p))
        info_table.append(("", ""))

    info_table.append(("CSV文件路径", args.f))
    info_table.append(("节点数量", len(nodes)))
    info_table.append(("并发数值", args.n))
    info_table.append(("", ""))

    # 超时设置
    if args.T:
        info_table.append(("连接超时", f"{args.T}s"))
    if args.t:
        if args.c or args.s:
            info_table.append(("执行超时", f"{args.t}s"))
        if args.u:
            info_table.append(("传输超时", f"{args.t}s"))
        if args.d:
            info_table.append(("传输超时", f"{args.t}s"))

    # 备注信息
    if args.r:
        info_table.append(("备注信息", args.r.strip()))

    return info_table


def _print_info_table(info_table) -> None:
    """打印信息表格，处理CJK字符宽度"""
    max_label_len = max(len(label) for label, _ in info_table if label)

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
                f"{color.COLOR_BRIGHT_CYAN}▶ {display_label}-→   {color.COLOR_RESET}{color.COLOR_BRIGHT_ORANGE}{value}{color.COLOR_RESET}"
            )


def _show_upload_content(u_path) -> None:
    """显示上传文件/目录内容"""
    print(
        f"\n{color.COLOR_YELLOW}📁 上传文件/目录内容 (-u 参数):{color.COLOR_RESET}"
    )
    try:
        u_path = Path(u_path)
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


@error_and_exit_handling_decorator("arguments_confirm", "确认执行参数失败")
def arguments_confirm(args, nodes, config=None, remove_symbol=None):
    """
    功能：
        确认执行参数

    参数：
        args: 命令行参数
        nodes: 节点列表
        config: 配置对象（可选，用于上传并发阈值检查）
        remove_symbol: 命令边界符号移除提示（可选，由调用方在命令构建阶段处理并传入）

    返回：
        None
    """

    # 上传并发建议（y 用建议值，n 保留原值继续执行，不退出）
    _check_upload_concurrency(args, config)

    # 未输入并发数，默认使用nodes数量进行并发
    args.n = len(nodes) if args.n == 0 else args.n

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
    info_table = _build_info_table(args, nodes)

    # 如果有命令边界符号移除提示
    if args.c and remove_symbol:
        print(
            f"{color.COLOR_YELLOW}▶ 重要提示: {color.COLOR_RESET}系统检测并移除了命令的边界符号 [ {color.COLOR_RED}{remove_symbol}{color.COLOR_RESET} ] \n"
        )

    # 2. 打印信息表格
    _print_info_table(info_table)

    # 3. 显示上传内容
    if args.u:
        _show_upload_content(args.u)

    # 4. 获取用户确认
    print("\n" + "═" * 60)
    if not get_user_confirmation(
        f"\n{color.COLOR_BRIGHT_YELLOW}是否执行上述参数？{color.COLOR_RESET}",
        yorn=True,
        disinteractive=getattr(args, 'disinteractive', False),
    ):
        print(f"{color.COLOR_YELLOW}操作已取消{color.COLOR_RESET}")
        tlog.warning("执行已取消，SSHFleet工具已退出")
        sys.exit(1)
    print(f"SSHFleet工具{color.COLOR_BLUE}开始执行{color.COLOR_RESET}......")
    print(f"{'=' * 50}")
