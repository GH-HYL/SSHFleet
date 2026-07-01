# -*- coding: utf-8 -*-
# Go 批量任务执行主入口

import argparse
from typing import Dict, List, Optional

from rich.console import Console
from rich.live import Live
from rich.progress import Progress, BarColumn, TextColumn, TimeElapsedColumn

from src.gotogo import builder, caller, parser
from src.toml import SSHFleetConfig
from src.utils import tlog

console = Console()


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
    request_body = builder.build_request(args, nodesinfo, transfer_command)
    tlog.info(f"请求体构建完成，共 {total_nodes} 个节点")

    # 2. 获取 Go 可执行文件路径
    exe_path = caller.get_exe_path(config)
    tlog.info(f"Go 程序路径: {exe_path}")

    # 3. 检查端口可用性并启动 Go 进程
    port = caller.find_available_port()
    process = caller.start_go_process(exe_path, port, exec_log_dir)

    # 4. 等待 Go 服务就绪
    if not caller.wait_for_server(port, timeout=10.0):
        stderr = caller.collect_stderr(process)
        process.kill()
        process.wait()
        tlog.error(f"Go 服务启动超时，stderr: {stderr}")
        raise RuntimeError(f"Go 服务启动超时，stderr: {stderr}")

    # 5. 创建进度条
    node_progress = Progress(
        TextColumn("    执行进度"),
        BarColumn(bar_width=40, complete_style="green", finished_style="blue"),
        TextColumn("{task.fields[percent_display]}"),
        "[green]{task.completed}/{task.total}",
        TimeElapsedColumn(),
    )
    node_task = node_progress.add_task("", total=total_nodes)

    # 6. 发送请求并接收 SSE 流
    results = []
    total_timeout = (args.T + args.t) * 1.5
    live = Live(node_progress, console=console, refresh_per_second=20)
    live.start()

    try:
        for sse_data in caller.call_go(request_body, port, timeout=total_timeout):
            result = parser.parse_result(sse_data, error_keywords)
            results.append(result)

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
        live.stop()

    # 7. 等待 Go 进程退出
    try:
        process.wait(timeout=30)
    except Exception:
        process.kill()
        process.wait()
    tlog.info("Go 进程已退出")

    # 8. 统计结果
    success_count = sum(1 for r in results if r.get("connect_success") and r.get("exit_bool"))
    fail_count = len(results) - success_count
    tlog.info(f"任务执行完成，共 {len(results)} 条结果: 成功 {success_count}, 失败 {fail_count}")

    return results
