# -*- coding: utf-8 -*-
# Go 批量任务执行主入口

import argparse
import os
from typing import Dict, List, Optional

from rich.console import Console
from rich.live import Live
from rich.progress import Progress, BarColumn, TextColumn, TimeElapsedColumn

from src.gotogo import builder, caller, parser
from src.yaml import SSHFleetConfig
from src.utils import tlog

console = Console()


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
    else:
        request_body = builder.build_request(args, nodesinfo, transfer_command)
        url_path = "/api/v1/execute"
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

    # 5. 创建进度条
    progress_text = "上传进度" if args.u else "执行进度"
    node_progress = Progress(
        TextColumn(f"    {progress_text}"),
        BarColumn(bar_width=40, complete_style="green", finished_style="blue"),
        TextColumn("{task.fields[percent_display]}"),
        "[green]{task.completed}/{task.total}",
        TimeElapsedColumn(),
    )
    node_task = node_progress.add_task("", total=total_nodes, percent_display="  0%")

    # 6. 发送请求并接收 SSE 流
    results = []
    total_timeout = (args.T + args.t) * 1.5
    live = Live(node_progress, console=console, refresh_per_second=20)
    live.start()

    # 打开 output.txt 文件用于写入
    output_file_path = os.path.join(exec_log_dir, config.paths.files.output)
    output_file = None
    try:
        output_file = open(output_file_path, "w", encoding="utf-8")
    except Exception as e:
        tlog.warning(f"无法创建 output.txt 文件: {e}")

    try:
        for sse_data in caller.call_go(request_body, port, process_key, timeout=total_timeout, url_path=url_path):
            result = parser.parse_result(sse_data, error_keywords)
            results.append(result)

            # 格式化输出
            formatted = _format_result(result, args)

            # 写入文件
            if output_file:
                try:
                    output_file.write(formatted + "\n")
                    output_file.flush()
                except Exception as e:
                    tlog.warning(f"写入 output.txt 失败: {e}")

            # 打印到终端
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
        live.stop()
        if output_file:
            output_file.close()

    # 7. 通知 Go 服务器关闭
    caller.shutdown_go_server(port, process_key)

    # 8. 等待 Go 进程退出
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
