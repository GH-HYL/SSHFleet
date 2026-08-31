# -*- coding: utf-8 -*-
# SSHFleet 终端输出模块
# 职责：终端呈现层——统计结果输出、单条结果行格式化、执行进度 UI（rich 对象唯一持有者）

import argparse
import re
import time
from typing import Any, Dict, List

from rich.console import Console
from rich.live import Live
from rich.progress import (
    Progress,
    BarColumn,
    TextColumn,
    TimeElapsedColumn,
    TransferSpeedColumn,
    DownloadColumn,
)
from rich.table import Table
from rich.text import Text

import src.common.constants as color

from src.common.error_handler import error_and_exit_handling_decorator
from src.common.format_utils import format_conn_status, get_action_name, get_mode
from src.log import tlog

console = Console()

# 可配置变量：最大显示节点数
MAX_VISIBLE_NODES = 20


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
    items = "  ".join(f"{code} >> {EXIT_CODE_HINTS[code]}" for code, _ in ordered)
    return f"常见退出码: {items}"


@error_and_exit_handling_decorator(
    "format_statistic_results_to_terminal",
    "格式化统计结果信息输出到终端失败",
    isexit=True,
)
def format_statistic_results_to_terminal(results_statistic: dict, error_keywords: dict) -> None:
    """
    功能：
        格式化统计结果信息输出到终端

    参数：
        results_statistic: 结果统计信息字典
        error_keywords: 错误分类关键词映射（用于区分"已分类"与"兜底原文"）

    返回值：
        None
    """

    print("═" * 60)
    print(f"  总耗时：{results_statistic['global_cost_time']} 秒")
    if results_statistic["verify"] == "通过":
        print(
            f"  {color.COLOR_CYAN}节点总数：{color.COLOR_RESET} {results_statistic['nodeinofs_total']}  {color.COLOR_CYAN}完成总数：{color.COLOR_RESET}{results_statistic['results_total']}"
        )
    else:
        print(
            f"  {color.COLOR_CYAN}节点总数：{color.COLOR_RESET} {results_statistic['nodeinofs_total']}  {color.COLOR_CYAN}完成总数：{color.COLOR_RESET}{results_statistic['results_total']}  {color.COLOR_CYAN}总数校验：{color.COLOR_RESET}{color.COLOR_RED}{results_statistic['verify']}{color.COLOR_RESET}"
        )

    if results_statistic["fail_counts"] > 0:
        print(
            f"  {color.COLOR_GREEN}成功：{color.COLOR_RESET} {results_statistic['success_counts']}   {color.COLOR_RED}失败：{color.COLOR_RESET} {results_statistic['fail_counts']}"
        )
    else:
        print(
            f"  {color.COLOR_GREEN}成功：{color.COLOR_RESET} {results_statistic['success_counts']}"
        )

    if results_statistic["sorted_fail_categories"]:
        # 延迟导入：terminal 被 go_to_go 引用，而 gotogo 包顶层会再回引 terminal，
        # 顶层 import 会形成循环依赖（与 error_handler 的延迟导入同理）
        from src.gotogo.classifier import is_fallback_category

        # 已分类的（关键词命中/动态退出码等）内容简短，合并一行展示；
        # 兜底原文（关键词未命中时把报错原文当分类）一般较长，每个独占一行
        known_parts = []
        fallback_parts = []
        for category, count in results_statistic["sorted_fail_categories"]:
            item = f"{color.COLOR_YELLOW}{category}：{color.COLOR_RESET}{count}"
            if is_fallback_category(category, error_keywords):
                fallback_parts.append(item)
            else:
                known_parts.append(item)

        if known_parts:
            print(
                f'  {color.COLOR_RED}失败分类统计{color.COLOR_RESET} >>>  {"  ".join(known_parts)}'
            )
        else:
            print(f"  {color.COLOR_RED}失败分类统计{color.COLOR_RESET} >>>")
        for item in fallback_parts:
            print(f"    {item}")

        hints = _format_exit_code_hints(results_statistic["sorted_fail_categories"])
        if hints:
            print(f"  {color.COLOR_YELLOW}{hints}{color.COLOR_RESET}")
    print("═" * 60)
    tlog.success("格式化统计结果信息输出到终端成功")
    return


def format_speed(bytes_per_sec: float) -> str:
    """格式化速度显示"""
    if bytes_per_sec >= 1024 * 1024:
        return f"{bytes_per_sec / 1024 / 1024:.1f}MB/s"
    elif bytes_per_sec >= 1024:
        return f"{bytes_per_sec / 1024:.1f}KB/s"
    return f"{bytes_per_sec:.0f}B/s"


class SpeedTracker:
    """聚合速度计算器（滑动窗口）"""

    def __init__(self, window_size: float = 2.0):
        self.window_size = window_size
        self.window: List[tuple] = []  # [(timestamp, bytes)]

    def update(self, total_bytes: int) -> float:
        """更新并返回当前速度（bytes/sec），确保不返回负值"""
        now = time.time()
        self.window.append((now, total_bytes))
        # 移除超过窗口的旧数据
        self.window = [(t, b) for t, b in self.window if now - t <= self.window_size]
        # 计算窗口内总字节差
        if len(self.window) >= 2:
            bytes_delta = self.window[-1][1] - self.window[0][1]
            time_delta = self.window[-1][0] - self.window[0][0]
            if time_delta > 0:
                speed = bytes_delta / time_delta
                return max(speed, 0)  # 确保不返回负速度
        return 0


def format_result_line(result: Dict, args: argparse.Namespace) -> str:
    """
    格式化单条结果

    格式：
        【IP】 连接: 成功/失败 - X.XXXs
        【IP】 执行/上传: 成功/失败 - X.XXXs
        输出内容
        【IP】 分类: 分类名称
        ==================================================
    """
    lines = []
    ip = result.get("ip", "未知IP")
    connect_success = result.get("connect_success", False)
    connect_cost_time = result.get("connect_cost_time", 0)
    exec_cost_time = result.get("exec_cost_time", 0)
    exit_code = result.get("exit_code", -1)
    output = result.get("output", "")
    error = result.get("error")
    result_category = result.get("result_category", "未知")
    action = get_action_name(get_mode(args))

    # 连接状态
    lines.append(f"【{ip}】 {format_conn_status(connect_success, connect_cost_time)}")

    if connect_success:
        # 执行/上传状态
        exec_success = exit_code == 0
        exec_status = "成功" if exec_success else "失败"
        lines.append(f"【{ip}】 {action}: {exec_status} - {exec_cost_time:.3f}s")
        # 输出内容（去除首尾空行）
        if output:
            lines.append(output.strip())
    else:
        # 连接失败，显示错误信息
        error_msg = error if error else "未知错误"
        lines.append(f"【{ip}】 错误: {error_msg}")

    # 分类
    lines.append(f"【{ip}】 分类: {result_category}")
    lines.append("=" * 50)

    return "\n".join(lines)


def record_result_output(result: Dict, args: argparse.Namespace, output_file) -> None:
    """格式化单条结果并落盘/打印（result 与兼容旧格式分支共用）"""
    formatted = format_result_line(result, args)

    # 写入 output.txt（两种模式共用）
    if output_file:
        try:
            output_file.write(formatted + "\n")
            output_file.flush()
        except Exception as e:
            tlog.warning(f"写入 output.txt 失败: {e}")

    # 命令模式：打印到终端
    if not args.u and not args.d:
        console.print(formatted)


def _build_progress(args: argparse.Namespace, total_nodes: int) -> dict:
    """
    构建进度条 UI（上传/下载 与 命令 两种模式）

    Returns:
        dict: 传输模式含 progress_table/total_progress/total_task/
              node_progress/node_task/node_bars/speed_tracker；
              命令模式仅 progress_table/node_progress/node_task
    """
    # 分界线
    separator = "─" * 50

    if args.u or args.d:
        # 统一前缀宽度
        prefix = "    "

        # 进度标签
        progress_label = "下载进度" if args.d else "上传进度"

        # 总字节进度（仅上传模式）
        total_progress = Progress(
            TextColumn(f"{prefix}{progress_label}  "),
            BarColumn(bar_width=40, complete_style="green", finished_style="blue"),
            "[progress.percentage]{task.percentage:>3.0f}%",
            TextColumn("  {task.fields[speed]}"),
            DownloadColumn(),
        )
        total_task = total_progress.add_task("", total=1, speed="0B/s")

        # 节点完成进度
        node_progress = Progress(
            TextColumn(f"{prefix}节点进度  "),
            BarColumn(bar_width=40, complete_style="green", finished_style="blue"),
            "[progress.percentage]{task.percentage:>3.0f}%",
            "[green]{task.completed}/{task.total}",
            TimeElapsedColumn(),
            TextColumn("  [bright_green]Succ:[bright_black]{task.fields[success_nodes]} [bright_red]Fail:[bright_black]{task.fields[fail_nodes]}"),
        )
        node_task = node_progress.add_task("", total=total_nodes, success_nodes=0, fail_nodes=0)

        # 单节点进度（动态增删）
        node_bars = Progress(
            TextColumn(f"{prefix}"),
            BarColumn(bar_width=40, complete_style="cyan", finished_style="blue"),
            "[progress.percentage]{task.percentage:>3.0f}%",
            TransferSpeedColumn(),
            TextColumn("  {task.fields[ip]}  Total:{task.fields[total_files]} Succ:{task.fields[success_files]} Fail:{task.fields[fail_files]}"),
        )

        # 布局
        progress_table = Table.grid()
        progress_table.add_row(total_progress)
        progress_table.add_row(node_progress)
        progress_table.add_row(Text(f"{prefix}{separator}"))
        progress_table.add_row(node_bars)

        return {
            "progress_table": progress_table,
            "total_progress": total_progress,
            "total_task": total_task,
            "node_progress": node_progress,
            "node_task": node_task,
            "node_bars": node_bars,
            "speed_tracker": SpeedTracker(),
        }

    # 命令模式：简单进度条
    node_progress = Progress(
        TextColumn("    执行进度"),
        BarColumn(bar_width=40, complete_style="green", finished_style="blue"),
        TextColumn("{task.fields[percent_display]}"),
        "[green]{task.completed}/{task.total}",
        TimeElapsedColumn(),
        TextColumn("  [bright_green]Succ:[bright_black]{task.fields[success_nodes]} [bright_red]Fail:[bright_black]{task.fields[fail_nodes]}"),
    )
    node_task = node_progress.add_task("", total=total_nodes, percent_display="  0%", success_nodes=0, fail_nodes=0)
    return {
        "progress_table": node_progress,
        "node_progress": node_progress,
        "node_task": node_task,
    }


class ProgressUI:
    """终端进度 UI（rich 对象唯一持有者）

    执行器经窄方法接口驱动呈现，不直接接触 rich 对象；
    Live 生命周期（start/stop）也由本对象统一管理。
    """

    def __init__(self, args: argparse.Namespace, total_nodes: int):
        ui = _build_progress(args, total_nodes)
        self.progress_table = ui["progress_table"]
        self.node_progress = ui["node_progress"]
        self.node_task = ui["node_task"]
        self.is_transfer = bool(getattr(args, "u", None) or getattr(args, "d", None))
        if self.is_transfer:
            self.total_progress = ui["total_progress"]
            self.total_task = ui["total_task"]
            self.node_bars = ui["node_bars"]
            self.speed_tracker = ui["speed_tracker"]
        self._task_by_seq = {}  # seq -> rich task_id（单节点进度条映射）
        self.live = Live(ui["progress_table"], console=console, refresh_per_second=20)

    # ---- 生命周期 ----
    def start(self) -> None:
        self.live.start()

    def stop(self) -> None:
        self.live.stop()

    # ---- 单节点进度条 ----
    def has_node_bar(self, seq) -> bool:
        return seq in self._task_by_seq if self.is_transfer else False

    def add_node_bar(self, seq, ip, total_files, total_bytes) -> None:
        if not self.is_transfer:
            return
        task_id = self.node_bars.add_task("",
            ip=ip,
            total_files=str(total_files),
            success_files="0",
            fail_files="0",
            total=total_bytes or 1,
        )
        self._task_by_seq[seq] = task_id

    def _task_id(self, seq):
        if not self.is_transfer:
            return None
        return self._task_by_seq.get(seq)

    def update_node_total(self, seq, total_bytes) -> None:
        tid = self._task_id(seq)
        if tid is not None:
            self.node_bars.update(tid, total=total_bytes)

    def update_node_files(self, seq, success_files, fail_files) -> None:
        tid = self._task_id(seq)
        if tid is not None:
            self.node_bars.update(tid, success_files=str(success_files), fail_files=str(fail_files or 0))

    def update_node_completed(self, seq, completed) -> None:
        tid = self._task_id(seq)
        if tid is not None:
            self.node_bars.update(tid, completed=completed)

    def remove_node_bar(self, seq) -> None:
        if not self.is_transfer:
            return
        tid = self._task_id(seq)
        if tid is not None:
            self.node_bars.remove_task(tid)
            self._task_by_seq.pop(seq, None)

    def refresh(self) -> None:
        self.live.refresh()

    def active_bar_count(self) -> int:
        """当前活跃单节点进度条数量（排队上限判断用）"""
        return len(self._task_by_seq)

    # ---- 总进度（传输模式） ----
    def update_total(self, total=None, completed=None, speed=None) -> None:
        if self.is_transfer:
            kwargs = {}
            if total is not None:
                kwargs["total"] = total
            if completed is not None:
                kwargs["completed"] = completed
            if speed is not None:
                kwargs["speed"] = speed
            self.total_progress.update(self.total_task, **kwargs)

    def speed_text(self, total_bytes) -> str:
        if not self.is_transfer:
            return ""
        return format_speed(self.speed_tracker.update(total_bytes))

    # ---- 节点完成进度 ----
    def update_node_progress(self, **kwargs) -> None:
        self.node_progress.update(self.node_task, **kwargs)

    # ---- 警告输出（SSE 流不完整等运行期提示） ----
    def warn(self, msg: str) -> None:
        """经 rich 标记渲染警告行（执行器不直接接触 rich）"""
        console.print(f"[yellow]警告: {msg}[/yellow]")
