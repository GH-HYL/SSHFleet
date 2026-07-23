# -*- coding: utf-8 -*-
# Go 批量任务执行主入口

import argparse
import os
import threading
import time
from typing import Dict, List, Optional

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

from src.gotogo import builder, caller, parser
from src.yaml import SSHFleetConfig
from src.log import tlog

console = Console()

# 可配置变量：最大显示节点数
MAX_VISIBLE_NODES = 20


def _format_speed(bytes_per_sec: float) -> str:
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


def _format_result(result: Dict, args: argparse.Namespace = None) -> str:
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
    action = "上传" if args and args.u else "执行"

    # 连接状态
    conn_status = "成功" if connect_success else "失败"
    lines.append(f"【{ip}】 连接: {conn_status} - {connect_cost_time:.3f}s")

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


def go_to_go(
    args: argparse.Namespace,
    config: SSHFleetConfig,
    nodesinfo: List[Dict],
    exec_log_dir: str,
    error_keywords: Dict[str, List[str]],
    transfer_command: Optional[str] = None,
) -> List[Dict]:
    """
    主执行函数 - 启动 Go 程序执行 SSH 批量任务，通过 HTTP SSE 接收结果

    Args:
        args: 命令行参数
        config: SSH 配置
        nodesinfo: 节点信息列表
        exec_log_dir: 执行日志目录
        error_keywords: 错误分类关键词
        transfer_command: 传输命令（可选）

    Returns:
        List[Dict]: 执行结果列表
    """
    total_nodes = len(nodesinfo)

    # 1. 构建请求体
    if args.u:
        request_body = builder.build_upload_request(args, nodesinfo)
        url_path = "/api/v1/upload"
        exec_mode = "upload"
    else:
        request_body = builder.build_request(args, nodesinfo, transfer_command)
        url_path = "/api/v1/execute"
        exec_mode = "execute"
    tlog.info(f"请求体构建完成，共 {total_nodes} 个节点")

    # 2. 获取 Go 可执行文件路径
    exe_path = caller.get_exe_path(config)
    tlog.info(f"Go 程序路径: {exe_path}")

    # 3. 检查端口可用性并启动 Go 进程
    port = caller.find_available_port()
    process, process_key = caller.start_go_process(exe_path, port, exec_log_dir)

    # 4. 等待 Go 服务就绪
    if not caller.wait_for_server(port, timeout=10.0):
        stderr = caller.collect_stderr(process)
        process.kill()
        process.wait()
        tlog.error(f"Go 服务启动超时，stderr: {stderr}")
        raise RuntimeError(f"Go 服务启动超时，stderr: {stderr}")

    # 5. 启动健康检查线程
    health_stop = threading.Event()
    go_dead = threading.Event()

    def health_checker():
        while not health_stop.is_set():
            if not caller.check_health(port):
                tlog.error("Go 进程健康检查失败，进程可能已崩溃")
                go_dead.set()
                break
            health_stop.wait(5)

    health_thread = threading.Thread(target=health_checker, daemon=True)
    health_thread.start()

    # 分界线
    separator = "─" * 50

    # 6. 创建进度条（仅上传模式使用双进度条）
    if args.u:
        # 统一前缀宽度
        prefix = "    "

        # 总字节进度
        total_progress = Progress(
            TextColumn(f"{prefix}上传进度  "),
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

        # 速度追踪器
        speed_tracker = SpeedTracker()
    else:
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
        progress_table = node_progress

    # 7. 发送请求并接收 SSE 流
    results = []
    total_timeout = (args.T + args.t) * 1.5

    # 上传模式的状态
    active_bars = {}  # {seq: task_id}
    node_approximate = {}  # {seq: uploaded_bytes}
    node_total_bytes = {}  # {seq: total_bytes}
    completed_nodes = 0
    upload_success_nodes = 0
    upload_fail_nodes = 0
    total_uploaded = 0
    global_total_bytes = 0

    # 命令模式的状态
    success_nodes = 0
    fail_nodes = 0

    # 打开 output.txt 文件（两种模式均写入）
    output_file_path = os.path.join(exec_log_dir, config.paths.files.output)
    output_file = None
    try:
        output_file = open(output_file_path, "w", encoding="utf-8")
    except Exception as e:
        tlog.warning(f"无法创建 output.txt 文件: {e}")

    live = Live(progress_table, console=console, refresh_per_second=20)
    live.start()

    try:
        for sse_data in caller.call_go(request_body, port, process_key, timeout=total_timeout, url_path=url_path):
            # 检查 Go 进程是否存活
            if go_dead.is_set():
                tlog.error("Go 进程已崩溃，终止接收")
                break

            # 判断消息类型
            msg_type = sse_data.get("type")

            if msg_type == "init" and args.u:
                # 初始化全局总量
                total_nodes_init = sse_data.get("total_nodes", total_nodes)
                total_bytes_per_node = sse_data.get("total_bytes_per_node", 0)
                global_total_bytes = total_nodes_init * total_bytes_per_node
                total_progress.update(total_task, total=global_total_bytes)

            elif msg_type == "progress" and args.u:
                seq = sse_data["seq"]
                uploaded = sse_data.get("uploaded_bytes", 0)
                total_bytes = sse_data.get("total_bytes")
                success_files = sse_data.get("success_files")
                failed_files = sse_data.get("failed_files")

                # 排队时也要记录 total_bytes
                if total_bytes is not None:
                    node_total_bytes[seq] = total_bytes

                if seq not in active_bars:
                    # 排队：满 N 个则等待
                    if len(active_bars) >= MAX_VISIBLE_NODES:
                        continue
                    # 初始化节点进度条
                    task_id = node_bars.add_task("",
                        ip=sse_data.get("ip", "?"),
                        total_files=str(sse_data.get("total_files", "?")),
                        success_files="0",
                        fail_files="0",
                        total=total_bytes or node_total_bytes.get(seq, 1),
                    )
                    active_bars[seq] = task_id
                    node_approximate[seq] = 0

                if total_bytes is not None:
                    # 首次：更新 total
                    node_bars.update(active_bars[seq], total=total_bytes)

                # 更新 success_files/failed_files
                if success_files is not None:
                    node_bars.update(active_bars[seq],
                        success_files=str(success_files),
                        fail_files=str(failed_files or 0))

                # 补偿：用精确值替换近似值
                if uploaded > 0:
                    old = node_approximate.get(seq, 0)
                    delta = uploaded - old
                    total_uploaded += delta
                    node_approximate[seq] = uploaded

                # 更新进度条
                speed = speed_tracker.update(total_uploaded)
                total_progress.update(total_task,
                    completed=total_uploaded,
                    speed=_format_speed(speed))
                if seq in active_bars:
                    node_bars.update(active_bars[seq], completed=uploaded)

            elif msg_type == "result":
                seq = sse_data.get("seq")

                if args.u and seq is not None:
                    # 用 result 的精确值校正（只增不减，防止进度回退）
                    old_approx = node_approximate.get(seq, 0)
                    exact_bytes = sse_data.get("total_bytes", old_approx)
                    if exact_bytes > old_approx:
                        total_uploaded += (exact_bytes - old_approx)

                    # 更新总进度
                    speed = speed_tracker.update(total_uploaded)
                    total_progress.update(total_task,
                        completed=total_uploaded,
                        speed=_format_speed(speed))

                    # 移除节点进度条
                    if seq in active_bars:
                        # 成功节点直接 100%
                        if sse_data.get("exit_code") == 0:
                            node_bars.update(active_bars[seq],
                                completed=node_total_bytes.get(seq, old_approx))
                        node_bars.remove_task(active_bars[seq])
                        del active_bars[seq]
                    for d in [node_approximate, node_total_bytes]:
                        d.pop(seq, None)

                    completed_nodes += 1
                    # 统计上传成功/失败节点
                    if sse_data.get("exit_code") == 0:
                        upload_success_nodes += 1
                    else:
                        upload_fail_nodes += 1
                    node_progress.update(node_task,
                        completed=completed_nodes,
                        success_nodes=upload_success_nodes,
                        fail_nodes=upload_fail_nodes)

                # 解析结果（兼容旧逻辑）
                result = parser.parse_result(sse_data, error_keywords, mode=exec_mode)
                results.append(result)

                # 格式化输出（两种模式共用）
                formatted = _format_result(result, args)

                # 写入 output.txt（两种模式共用）
                if output_file:
                    try:
                        output_file.write(formatted + "\n")
                        output_file.flush()
                    except Exception as e:
                        tlog.warning(f"写入 output.txt 失败: {e}")

                if not args.u:
                    # 命令模式：打印到终端
                    console.print(formatted)

                    # 统计成功/失败节点
                    if result.get("exit_code") == 0:
                        success_nodes += 1
                    else:
                        fail_nodes += 1

                    # 更新进度
                    completed = len(results)
                    percent_int = int(completed / total_nodes * 100)
                    node_progress.update(
                        node_task,
                        description=f"执行进度 [bright_yellow]已完成: [bright_black]{completed}/{total_nodes}",
                        completed=completed,
                        percent_display=f"{percent_int:>3}%",
                        success_nodes=success_nodes,
                        fail_nodes=fail_nodes,
                    )

            elif msg_type == "done":
                # 处理完成标记
                if not args.u:
                    # 命令模式：更新进度
                    done_total = sse_data.get("total", total_nodes)
                    done_success = sse_data.get("success", 0)
                    done_failed = sse_data.get("failed", 0)
                    node_progress.update(node_task, completed=done_total)
                break

            else:
                # 其他消息（兼容旧的 result 格式）
                result = parser.parse_result(sse_data, error_keywords, mode=exec_mode)
                results.append(result)

                # 格式化输出（两种模式共用）
                formatted = _format_result(result, args)

                # 写入 output.txt（两种模式共用）
                if output_file:
                    try:
                        output_file.write(formatted + "\n")
                        output_file.flush()
                    except Exception as e:
                        tlog.warning(f"写入 output.txt 失败: {e}")

                if not args.u:
                    # 命令模式：打印到终端
                    console.print(formatted)

                    # 更新进度
                    completed = len(results)
                    percent_int = int(completed / total_nodes * 100)
                    node_progress.update(
                        node_task,
                        description=f"执行进度 [bright_yellow]已完成: [bright_black]{completed}/{total_nodes}",
                        completed=completed,
                        percent_display=f"{percent_int:>3}%",
                    )

    finally:
        # 停止健康检查线程
        health_stop.set()
        health_thread.join(timeout=2)
        live.stop()
        # 关闭 output.txt
        if output_file:
            output_file.close()

    # 8. 通知 Go 服务器关闭
    if not go_dead.is_set():
        caller.shutdown_go_server(port, process_key)

    # 9. 等待 Go 进程退出
    try:
        process.wait(timeout=10)
    except Exception:
        process.kill()
        process.wait()
    tlog.info("Go 进程已退出")

    # 10. 统计结果
    success_count = sum(1 for r in results if r.get("connect_success") and r.get("exit_bool"))
    fail_count = len(results) - success_count
    tlog.info(f"任务执行完成，共 {len(results)} 条结果: 成功 {success_count}, 失败 {fail_count}")

    return results
