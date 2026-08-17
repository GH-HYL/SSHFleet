# -*- coding: utf-8 -*-
# SSHFleet 统计结果信息模块

import argparse
from typing import Dict, List
from collections import Counter
from datetime import datetime

from src.common.error_handler import error_and_exit_handling_decorator
from src.log import tlog
from src.common.constants import SUCCESS_CATEGORY_EXECUTE, SUCCESS_CATEGORY_TRANSPORT
from src.common.format_utils import get_mode, sort_ips


@error_and_exit_handling_decorator(
    "results_statistics", "计算统计结果信息失败", isexit=True
)
def results_statistics(
    results: List,
    node_infos: List,
    args: argparse.Namespace,
    global_start_time: datetime,
    global_stop_time: datetime,
) -> Dict:
    """
    功能：
        计算统计结果信息

    参数：
        results: 结果列表
        node_infos: 节点信息列表
        args: 命令行参数
        global_start_time: 全局开始时间
        global_stop_time: 全局结束时间

    返回：
        包含所有统计结果信息的字典
    """

    # 基本统计
    results_total = len(results)
    nodeinofs_total = len(node_infos)

    # 成功/失败统计
    exit_counts = Counter(d.get("exit_bool") for d in results)
    success_counts = exit_counts.get(True, 0)
    fail_counts = exit_counts.get(False, 0)

    # 总数校验
    verify = "通过" if nodeinofs_total == results_total else "异常"

    # 分类统计
    category_counts = Counter(d.get("result_category") for d in results)

    # 根据模式移除成功分类
    if get_mode(args) in ("upload", "download"):
        success_category = SUCCESS_CATEGORY_TRANSPORT
        category_counts.pop(success_category, None)
    else:
        success_category = SUCCESS_CATEGORY_EXECUTE
        category_counts.pop(success_category, None)

    # 按数量正（倒）序排序（reverse逆转）失败分类
    sorted_fail_categories = sorted(
        category_counts.items(), key=lambda x: x[1], reverse=True
    )

    # 按分类分组并收集IP地址
    category_ip_map = {}
    for result in results:
        category = result.get("result_category", "未知")
        ip = result.get("ip", "未知IP")
        if category not in category_ip_map:
            category_ip_map[category] = []
        category_ip_map[category].append(ip)

    # 分离成功分类IP
    if "success_category" not in locals():
        tlog.error("success_category 未定义，未知执行模式")
        success_category = "未知"
    success_ips = category_ip_map.pop(success_category, [])

    # 对每个分类的IP进行数字排序
    for category in category_ip_map:
        category_ip_map[category] = sort_ips(category_ip_map[category])

    # 对成功IP进行数字排序
    sorted_success_ips = sort_ips(success_ips) if success_ips else []

    # 构建统计结果字典
    statistics = {
        "results_total": results_total,
        "nodeinofs_total": nodeinofs_total,
        "verify": verify,
        "success_counts": success_counts,
        "fail_counts": fail_counts,
        "sorted_fail_categories": sorted_fail_categories,
        "success_category": (
            success_category if "success_category" in locals() else None
        ),
        "success_ips_count": len(success_ips),
        "sorted_success_ips": sorted_success_ips,
        "category_ip_map": category_ip_map,
        "global_start_time": global_start_time,
        "global_stop_time": global_stop_time,
        "global_cost_time": round(
            (global_stop_time - global_start_time).total_seconds(), 2
        ),
    }
    tlog.success("计算统计结果信息成功")

    return statistics
