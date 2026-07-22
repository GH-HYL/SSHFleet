# -*- coding: utf-8 -*-
# SSHFleet 终端输出模块

import src.color as color

from src import utils
from src.log import tlog


@utils.error_and_exit_handling_decorator(
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
    print("═" * 60)
    tlog.success("格式化统计结果信息输出到终端成功")
    return
