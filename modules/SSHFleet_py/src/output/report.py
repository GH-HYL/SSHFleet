# -*- coding: utf-8 -*-
# SSHFleet 报告输出模块

import argparse
import sys
from posixpath import join as posix_join

from src import utils
from src.yaml import SSHFleetConfig
from src.log import tlog


@utils.error_and_exit_handling_decorator(
    "format_statistic_results_to_report",
    "格式化统计结果信息输出到报告文件失败",
    isexit=True,
)
def format_statistic_results_to_report(
    results_statistic: dict,
    log_dir: str,
    args: argparse.Namespace,
    config: SSHFleetConfig,
) -> None:
    """
    功能：
        格式化统计结果信息输出到报告文件

    参数：
        results_statistic: 结果统计信息字典
        log_dir: 日志目录路径
        args: 命令行参数
        config: 配置对象

    返回值：
        None
    """
    report_file = posix_join(log_dir, config.paths.files.report)

    # 格式化执行命令内容
    args_set = sys.argv[1:]
    args_content = " ".join(args_set) if args_set else ""
    command = f"python3 {sys.argv[0]} {args_content}"

    with open(report_file, "w", encoding="utf-8") as f:
        f.write(
            "=============================执行结果统计报告=============================\n"
        )
        f.write(f'执行开始时间： {results_statistic["global_start_time"]}\n')
        f.write(f'执行结束时间： {results_statistic["global_stop_time"]}\n')
        f.write(f'执行耗时： {results_statistic["global_cost_time"]}  秒\n')
        f.write(f"\n【执行命令】 \n  {command}\n")
        f.write("\n【执行参数】\n")
        if args.c:
            f.write("  执行模式： 命令模式\n")
            f.write(f"  执行命令： {args.c}\n")
        if args.s:
            f.write("  执行模式： 脚本模式\n")
            f.write(f"  脚本路径： {args.s}\n")
        if args.u:
            f.write("  执行模式： 上传模式\n")
            f.write(f"  本地路径： {args.u}\n")
            f.write(f"  远程路径： {args.p}\n")

        f.write(f"  CSV文件路径： {args.f}\n")
        f.write(f'  节点数量： {results_statistic["nodeinofs_total"]}\n')
        if args.c or args.s:
            f.write(f"  并发数值： {args.n}\n")
        if args.T:
            f.write(f"  连接超时： {args.T}s\n")
        if args.t:
            if args.c or args.s:
                f.write(f"  执行超时： {args.t}s\n")
            if args.u:
                f.write(f"  传输超时： {args.t}s\n")

        f.write("\n【结果统计】\n")
        f.write(f"  总耗时： {results_statistic['global_cost_time']}  秒\n")
        f.write(
            f"  节点总数: {results_statistic['nodeinofs_total']}  完成总数：{results_statistic['results_total']}  总数校验：{results_statistic['verify']}\n"
        )
        f.write(
            f"  成功: {results_statistic['success_counts']}    失败: {results_statistic['fail_counts']}\n"
        )

        if results_statistic["sorted_fail_categories"]:
            f.write(
                f'  失败分类统计 -→  {"  ".join(f"{k}：{v}" for k, v in results_statistic["sorted_fail_categories"])}\n'
            )

        f.write("\n【IP清单统计】\n")

        # 先输出失败分类（按IP数量升序排列）
        sorted_fail_items = sorted(
            results_statistic["category_ip_map"].items(), key=lambda x: len(x[1])
        )
        for category, ips in sorted_fail_items:
            f.write(f"\n{category}（{len(ips)}）：\n")
            for ip in ips:
                f.write(f"{ip}\n")

        # 最后输出成功分类
        if results_statistic["sorted_success_ips"]:
            f.write(
                f'\n{results_statistic["success_category"]}（{results_statistic["success_ips_count"]}）：\n'
            )
            for ip in results_statistic["sorted_success_ips"]:
                f.write(f"{ip}\n")
    tlog.success("格式化统计结果信息输出到报告文件成功")
