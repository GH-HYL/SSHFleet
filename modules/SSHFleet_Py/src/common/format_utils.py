# -*- coding: utf-8 -*-
# SSHFleet 结果呈现公共工具模块
# 单条结果的状态行格式、动作名、IP 排序等在多个输出端（终端/Excel/统计）复用的纯函数。
# 放置于共享层 src/common：被执行层（gotogo）与呈现层（output）共同引用，避免跨层倒挂。


def get_mode(args) -> str:
    """
    从命令行参数推导执行模式

    Returns:
        str: "upload" | "download" | "execute"
    """
    if getattr(args, "u", None):
        return "upload"
    if getattr(args, "d", None):
        return "download"
    return "execute"


def get_action_name(mode: str) -> str:
    """
    模式 → 动作中文名（状态行用）

    Args:
        mode: "upload" | "download" | "execute"

    Returns:
        str: "上传" | "下载" | "执行"
    """
    return {"execute": "执行", "upload": "上传", "download": "下载"}.get(mode, "执行")


def format_conn_status(connect_success: bool, connect_cost_time: float) -> str:
    """状态行：连接状态（"连接: 成功 - X.XXXs"，终端与 Excel 共用）"""
    status = "成功" if connect_success else "失败"
    return f"连接: {status} - {connect_cost_time:.3f}s"


def sort_ips(ips) -> list:
    """IP 数字排序（10.0.0.2 排在 10.0.0.10 前），统计与报告共用"""
    return sorted(ips, key=lambda ip: [int(part) for part in ip.split(".")])
