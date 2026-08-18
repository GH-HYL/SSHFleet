# -*- coding: utf-8 -*-
# Go 批量任务执行主入口

import argparse
import os
import threading
import time
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional

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
from src.common.format_utils import format_conn_status, get_action_name, get_mode
from src.common.loader import SSHFleetConfig
from src.log import tlog

console = Console()

# 可配置变量：最大显示节点数
MAX_VISIBLE_NODES = 20


@dataclass
class SseSession:
    """一次 SSE 接收循环的共享上下文（进度状态 + UI 引用 + 结果集）"""

    # 上传/下载模式状态
    active_bars: Dict = field(default_factory=dict)         # {seq: task_id}
    node_approximate: Dict = field(default_factory=dict)    # {seq: uploaded_bytes}
    node_total_bytes: Dict = field(default_factory=dict)    # {seq: total_bytes}
    completed_nodes: int = 0
    upload_success_nodes: int = 0
    upload_fail_nodes: int = 0
    total_uploaded: int = 0
    global_total_bytes: int = 0
    # 命令模式状态
    success_nodes: int = 0
    fail_nodes: int = 0
    # 结果与输出
    results: List = field(default_factory=list)
    output_file: Any = None
    # UI 引用（传输模式才有 total_progress/total_task/node_bars/speed_tracker）
    progress_table: Any = None
    node_progress: Any = None
    node_task: Any = None
    total_progress: Any = None
    total_task: Any = None
    node_bars: Any = None
    speed_tracker: Any = None
    live: Any = None


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


def _record_result(result: Dict, args: argparse.Namespace, output_file) -> None:
    """格式化单条结果并落盘/打印（result 与兼容旧格式分支共用）"""
    formatted = _format_result(result, args)

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


def _shutdown_go(process, port, process_key, go_dead, health_stop, health_thread, live, output_file) -> None:
    """Go 进程与资源的统一收尾（正常退出与 Ctrl+C 中断共用）"""
    # 停止健康检查线程
    health_stop.set()
    health_thread.join(timeout=2)
    live.stop()
    # 关闭 output.txt
    if output_file:
        output_file.close()

    # 通知 Go 服务器关闭（即使 Ctrl+C 中断也要收尾，避免残留孤儿进程）
    if not go_dead.is_set():
        caller.shutdown_go_server(port, process_key)

    # 等待 Go 进程退出
    try:
        process.wait(timeout=10)
    except Exception:
        process.kill()
        process.wait()
    tlog.info("Go 进程已退出")


def _handle_init(session: SseSession, sse_data: Dict, total_nodes: int) -> bool:
    """处理 init 消息：初始化传输全局总量（仅上传模式调用）"""
    total_nodes_init = sse_data.get("total_nodes", total_nodes)
    total_bytes_per_node = sse_data.get("total_bytes_per_node", 0)
    session.global_total_bytes = total_nodes_init * total_bytes_per_node
    session.total_progress.update(session.total_task, total=session.global_total_bytes)
    return True


def _handle_progress(session: SseSession, sse_data: Dict) -> bool:
    """处理 progress 消息：更新上传/下载进度条（排队满则跳过本条）"""
    seq = sse_data["seq"]
    uploaded = sse_data.get("uploaded_bytes", 0) or sse_data.get("downloaded_bytes", 0)
    total_bytes = sse_data.get("total_bytes")
    success_files = sse_data.get("success_files")
    failed_files = sse_data.get("failed_files")

    # 排队时也要记录 total_bytes
    if total_bytes is not None:
        session.node_total_bytes[seq] = total_bytes

    if seq not in session.active_bars:
        # 排队：满 N 个则等待
        if len(session.active_bars) >= MAX_VISIBLE_NODES:
            return True
        # 初始化节点进度条
        task_id = session.node_bars.add_task("",
            ip=sse_data.get("ip", "?"),
            total_files=str(sse_data.get("total_files", "?")),
            success_files="0",
            fail_files="0",
            total=total_bytes or session.node_total_bytes.get(seq, 1),
        )
        session.active_bars[seq] = task_id
        session.node_approximate[seq] = 0

    if total_bytes is not None:
        # 首次：更新 total
        session.node_bars.update(session.active_bars[seq], total=total_bytes)

    # 更新 success_files/failed_files
    if success_files is not None:
        session.node_bars.update(session.active_bars[seq],
            success_files=str(success_files),
            fail_files=str(failed_files or 0))

    # 补偿：用精确值替换近似值
    if uploaded > 0:
        old = session.node_approximate.get(seq, 0)
        delta = uploaded - old
        session.total_uploaded += delta
        session.node_approximate[seq] = uploaded

    # 更新进度条
    speed = session.speed_tracker.update(session.total_uploaded)
    session.total_progress.update(session.total_task,
        completed=session.total_uploaded,
        speed=_format_speed(speed))
    if seq in session.active_bars:
        session.node_bars.update(session.active_bars[seq], completed=uploaded)
    return True


def _handle_result(session: SseSession, sse_data: Dict, args, error_keywords: Dict, exec_mode: str, total_nodes: int) -> bool:
    """处理 result 消息：上传进度校正 + 结果解析记录 + 命令模式进度"""
    seq = sse_data.get("seq")

    if (args.u or args.d) and seq is not None:
        # 用 result 的精确值校正（只增不减，防止进度回退）
        old_approx = session.node_approximate.get(seq, 0)
        exact_bytes = sse_data.get("total_bytes", old_approx)
        if exact_bytes > old_approx:
            session.total_uploaded += (exact_bytes - old_approx)

        # 更新总进度
        speed = session.speed_tracker.update(session.total_uploaded)
        session.total_progress.update(session.total_task,
            completed=session.total_uploaded,
            speed=_format_speed(speed))

        # 移除节点进度条
        if seq in session.active_bars:
            # 成功节点直接 100%
            if sse_data.get("exit_code") == 0:
                session.node_bars.update(session.active_bars[seq],
                    completed=session.node_total_bytes.get(seq, old_approx))
                # 小文件传输太快，progress 与 result 几乎同时到达，
                # 在删除前强制刷新一次，确保 100% 完成态被渲染出来
                session.live.refresh()
            session.node_bars.remove_task(session.active_bars[seq])
            del session.active_bars[seq]
        for d in [session.node_approximate, session.node_total_bytes]:
            d.pop(seq, None)

        session.completed_nodes += 1
        # 统计上传成功/失败节点
        if sse_data.get("exit_code") == 0:
            session.upload_success_nodes += 1
        else:
            session.upload_fail_nodes += 1
        session.node_progress.update(session.node_task,
            completed=session.completed_nodes,
            success_nodes=session.upload_success_nodes,
            fail_nodes=session.upload_fail_nodes)

    # 解析结果（兼容旧逻辑）
    result = parser.parse_result(sse_data, error_keywords, mode=exec_mode)
    session.results.append(result)

    # 格式化输出 + 落盘/打印（result 与兼容分支共用）
    _record_result(result, args, session.output_file)

    if not args.u and not args.d:
        # 统计成功/失败节点
        if result.get("exit_code") == 0:
            session.success_nodes += 1
        else:
            session.fail_nodes += 1

        # 更新进度
        completed = len(session.results)
        percent_int = int(completed / total_nodes * 100)
        session.node_progress.update(
            session.node_task,
            description=f"执行进度 [bright_yellow]已完成: [bright_black]{completed}/{total_nodes}",
            completed=completed,
            percent_display=f"{percent_int:>3}%",
            success_nodes=session.success_nodes,
            fail_nodes=session.fail_nodes,
        )
    return True


def _handle_done(session: SseSession, sse_data: Dict, args, total_nodes: int) -> bool:
    """处理 done 消息：完成标记 + 一致性校验，结束循环（返回 False）"""
    done_total = sse_data.get("total", total_nodes)
    if not args.u and not args.d:
        # 命令模式：更新进度
        session.node_progress.update(session.node_task, completed=done_total)
    # total 一致性校验（P4）：仅不一致时警告，不中断
    if done_total != len(session.results):
        warn_msg = f"SSE 流可能不完整: 收到 {len(session.results)} 条结果, 预期 {done_total} 条"
        tlog.warning(warn_msg)
        console.print(f"[yellow]警告: {warn_msg}[/yellow]")
    return False


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
    elif args.d:
        request_body = builder.build_download_request(args, nodesinfo)
        url_path = "/api/v1/download"
        exec_mode = "download"
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

    # 6. 创建进度条（上传/下载 与 命令 两种模式）
    ui = _build_progress(args, total_nodes)
    progress_table = ui["progress_table"]
    node_progress = ui["node_progress"]
    node_task = ui["node_task"]
    if args.u or args.d:
        total_progress = ui["total_progress"]
        total_task = ui["total_task"]
        node_bars = ui["node_bars"]
        speed_tracker = ui["speed_tracker"]

    # 7. 发送请求并接收 SSE 流
    total_timeout = (args.T + args.t) * 1.5

    # 打开 output.txt 文件（两种模式均写入）
    output_file_path = os.path.join(exec_log_dir, config.paths.files.output)
    output_file = None
    try:
        output_file = open(output_file_path, "w", encoding="utf-8")
    except Exception as e:
        tlog.warning(f"无法创建 output.txt 文件: {e}")

    # SSE 会话上下文（进度状态 + UI 引用 + 结果集；传输模式才注入 total/node_bars/speed_tracker）
    session = SseSession(
        output_file=output_file,
        progress_table=progress_table,
        node_progress=node_progress,
        node_task=node_task,
        total_progress=total_progress if (args.u or args.d) else None,
        total_task=total_task if (args.u or args.d) else None,
        node_bars=node_bars if (args.u or args.d) else None,
        speed_tracker=speed_tracker if (args.u or args.d) else None,
    )
    live = Live(progress_table, console=console, refresh_per_second=20)
    session.live = live
    live.start()

    try:
        for sse_data in caller.call_go(request_body, port, process_key, timeout=total_timeout, url_path=url_path):
            # 检查 Go 进程是否存活
            if go_dead.is_set():
                tlog.error("Go 进程已崩溃，终止接收")
                break

            # 按消息类型分派（Go 端仅 4 种 type：init/progress/result/done）
            msg_type = sse_data.get("type")
            if msg_type == "init":
                if args.u:
                    if not _handle_init(session, sse_data, total_nodes):
                        break
                else:
                    tlog.warning("非上传模式收到 init 消息，已忽略")
            elif msg_type == "progress":
                if args.u or args.d:
                    if not _handle_progress(session, sse_data):
                        break
                else:
                    tlog.warning("命令模式收到 progress 消息，已忽略")
            elif msg_type == "result":
                if not _handle_result(session, sse_data, args, error_keywords, exec_mode, total_nodes):
                    break
            elif msg_type == "done":
                if not _handle_done(session, sse_data, args, total_nodes):
                    break
            else:
                # 未知消息类型：忽略并告警（ADR-0002-B；Go 端当前仅 4 种 type）
                tlog.warning(f"收到未知 SSE 消息类型: {msg_type!r}，已忽略")
                continue

    finally:
        # 统一收尾：停止线程/live、关闭文件、通知并回收 Go 进程
        _shutdown_go(process, port, process_key, go_dead, health_stop, health_thread, live, output_file)

    # 10. 统计结果
    success_count = sum(1 for r in session.results if r.get("connect_success") and r.get("exit_bool"))
    fail_count = len(session.results) - success_count
    tlog.info(f"任务执行完成，共 {len(session.results)} 条结果: 成功 {success_count}, 失败 {fail_count}")

    return session.results
