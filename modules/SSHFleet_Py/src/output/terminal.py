# -*- coding: utf-8 -*-
# SSHFleet 终端输出模块

import re

import src.common.constants as color

from src.common.error_handler import error_and_exit_handling_decorator
from src.log import tlog


# 常见退出码含义字典（Unix 通用语义，按需自行增删）
EXIT_CODE_HINTS = {
    1: "一般性错误",
    2: "命令用法错误",
    126: "命令不可执行(权限不足)",
    127: "命令未找到",
    130: "被中断(SIGINT/Ctrl+C)",
    137: "被强制杀死(SIGKILL)",
    143: "被终止(SIGTERM)",
    255: "命令执行失败",
}


def _format_exit_code_hints(sorted_fail_categories) -> str:
    """从失败分类中提取本次出现的退出码，翻译为常见退出码提示

    只列 EXIT_CODE_HINTS 字典中命中的码，按出现台数降序，同一码只出现一次。
    无命中时返回空串（调用方不输出该行）。
    """
    seen = {}
    for category, count in sorted_fail_categories:
        m = re.search(r"退出码(\d+)", category)
        if not m:
            continue
        code = int(m.group(1))
        if code in EXIT_CODE_HINTS and code not in seen:
            seen[code] = count

    if not seen:
        return ""

    ordered = sorted(seen.items(), key=lambda x: x[1], reverse=True)
    items = "  ".join(f"{code}={EXIT_CODE_HINTS[code]}" for code, _ in ordered)
    return f"常见退出码: {items}"


@error_and_exit_handling_decorator(
    "format_statistic_results_to_terminal",
    "格式化统计结果信息输出到终端失败",
    isexit=True,
)
def format_statistic_results_to_terminal(results_statistic: dict) -> None:
    """
    功能：
        格式化统计结果信息输出到终端

    参数：
        results_statistic: 结果统计信息字典

    返回值：
        None
    """

    print("═" * 60)
    print(f"  总耗时： {results_statistic['global_cost_time']}  秒")
    if results_statistic["verify"] == "通过":
        print(
            f"  {color.COLOR_CYAN}节点总数:{color.COLOR_RESET} {results_statistic['nodeinofs_total']}  {color.COLOR_CYAN}完成总数：{color.COLOR_RESET}{results_statistic['results_total']}"
        )
    else:
        print(
            f"  {color.COLOR_CYAN}节点总数:{color.COLOR_RESET} {results_statistic['nodeinofs_total']}  {color.COLOR_CYAN}完成总数：{color.COLOR_RESET}{results_statistic['results_total']}  {color.COLOR_CYAN}总数校验：{color.COLOR_RESET}{color.COLOR_RED}{results_statistic['verify']}{color.COLOR_RESET}"
        )

    if results_statistic["fail_counts"] > 0:
        print(
            f"  {color.COLOR_GREEN}成功:{color.COLOR_RESET} {results_statistic['success_counts']}   {color.COLOR_RED}失败:{color.COLOR_RESET} {results_statistic['fail_counts']}"
        )
    else:
        print(
            f"  {color.COLOR_GREEN}成功:{color.COLOR_RESET} {results_statistic['success_counts']}"
        )

    if results_statistic["sorted_fail_categories"]:
        print(
            f'  {color.COLOR_RED}失败分类统计{color.COLOR_RESET} >>>  {"  ".join(f"{color.COLOR_YELLOW}{k}：{color.COLOR_RESET}{v}" for k, v in results_statistic["sorted_fail_categories"])}'
        )
        hints = _format_exit_code_hints(results_statistic["sorted_fail_categories"])
        if hints:
            print(f"  {color.COLOR_YELLOW}{hints}{color.COLOR_RESET}")
    print("═" * 60)
    tlog.success("格式化统计结果信息输出到终端成功")
    return
