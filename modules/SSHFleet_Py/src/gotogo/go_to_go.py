# -*- coding: utf-8 -*-
# Go 批量任务执行主入口

import argparse
import os
import threading
from dataclasses import dataclass, field
from typing import Any, Dict, List

from src.gotogo import builder, caller, parser
from src.common.loader import SSHFleetConfig
from src.log import tlog
from src.output.terminal import (
    MAX_VISIBLE_NODES,
    ProgressUI,
    record_result_output,
)


@dataclass
class SseSession:
    """一次 SSE 接收循环的共享上下文（进度状态 + 结果集；呈现经 ProgressUI 门面驱动）"""

    # 上传/下载模式状态
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
    # 呈现门面（rich 对象唯一持有者；执行器只经窄接口驱动）
    ui: Any = None


def _shutdown_go(process, port, process_key, go_dead, health_stop, health_thread, ui, output_file) -> None:
    """Go 进程与资源的统一收尾（正常退出与 Ctrl+C 中断共用）"""
    # 停止健康检查线程
    health_stop.set()
    health_thread.join(timeout=2)
    ui.stop()
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
    session.ui.update_total(total=session.global_total_bytes)
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

    if not session.ui.has_node_bar(seq):
        # 排队：满 N 个则等待
        if session.ui.active_bar_count() >= MAX_VISIBLE_NODES:
            return True
        # 初始化节点进度条（经呈现门面，不直接接触 rich 对象）
        session.ui.add_node_bar(
            seq,
            ip=sse_data.get("ip", "?"),
            total_files=sse_data.get("total_files", "?"),
            total_bytes=total_bytes or session.node_total_bytes.get(seq, 1),
        )
        session.node_approximate[seq] = 0

    if total_bytes is not None:
        # 首次：更新 total
        session.ui.update_node_total(seq, total_bytes)

    # 更新 success_files/failed_files
    if success_files is not None:
        session.ui.update_node_files(seq, success_files, failed_files or 0)

    # 补偿：用精确值替换近似值
    if uploaded > 0:
        old = session.node_approximate.get(seq, 0)
        delta = uploaded - old
        session.total_uploaded += delta
        session.node_approximate[seq] = uploaded

    # 更新进度条
    session.ui.update_total(completed=session.total_uploaded, speed=session.ui.speed_text(session.total_uploaded))
    if session.ui.has_node_bar(seq):
        session.ui.update_node_completed(seq, uploaded)
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
        session.ui.update_total(completed=session.total_uploaded, speed=session.ui.speed_text(session.total_uploaded))

        # 移除节点进度条
        if session.ui.has_node_bar(seq):
            # 成功节点直接 100%
            if sse_data.get("exit_code") == 0:
                session.ui.update_node_completed(seq, session.node_total_bytes.get(seq, old_approx))
                # 小文件传输太快，progress 与 result 几乎同时到达，
                # 在删除前强制刷新一次，确保 100% 完成态被渲染出来
                session.ui.refresh()
            session.ui.remove_node_bar(seq)
        for d in [session.node_approximate, session.node_total_bytes]:
            d.pop(seq, None)

        session.completed_nodes += 1
        # 统计上传成功/失败节点
        if sse_data.get("exit_code") == 0:
            session.upload_success_nodes += 1
        else:
            session.upload_fail_nodes += 1
        session.ui.update_node_progress(
            completed=session.completed_nodes,
            success_nodes=session.upload_success_nodes,
            fail_nodes=session.upload_fail_nodes,
        )

    # 解析结果（兼容旧逻辑）
    result = parser.parse_result(sse_data, error_keywords, mode=exec_mode)
    session.results.append(result)

    # 格式化输出 + 落盘/打印（呈现层统一实现）
    record_result_output(result, args, session.output_file)

    if not args.u and not args.d:
        # 统计成功/失败节点
        if result.get("exit_code") == 0:
            session.success_nodes += 1
        else:
            session.fail_nodes += 1

        # 更新进度
        completed = len(session.results)
        percent_int = int(completed / total_nodes * 100)
        session.ui.update_node_progress(
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
        session.ui.update_node_progress(completed=done_total)
    # total 一致性校验（P4）：仅不一致时警告，不中断
    if done_total != len(session.results):
        warn_msg = f"SSE 流可能不完整: 收到 {len(session.results)} 条结果, 预期 {done_total} 条"
        tlog.warning(warn_msg)
        session.ui.warn(warn_msg)
    return False


def go_to_go(
    args: argparse.Namespace,
    config: SSHFleetConfig,
    nodesinfo: List[Dict],
    exec_log_dir: str,
    error_keywords: Dict[str, List[str]],
) -> List[Dict]:
    """
    主执行函数 - 启动 Go 程序执行 SSH 批量任务，通过 HTTP SSE 接收结果

    Args:
        args: 命令行参数
        config: SSH 配置
        nodesinfo: 节点信息列表
        exec_log_dir: 执行日志目录
        error_keywords: 错误分类关键词

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
        request_body = builder.build_request(args, nodesinfo)
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
        err_detail = f"，stderr: {stderr}" if stderr else ""
        raise RuntimeError(
            f"Go 服务启动超时（10 秒未就绪）{err_detail}"
            f"，请检查 {exe_path} 是否存在、可执行，或查看日志目录下的错误输出"
        )

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

    # 6. 创建进度 UI（上传/下载 与 命令 两种模式；rich 对象唯一持有者）
    ui = ProgressUI(args, total_nodes)

    # 7. 发送请求并接收 SSE 流
    total_timeout = (args.T + args.t) * 1.5

    # 打开 output.txt 文件（两种模式均写入）
    output_file_path = os.path.join(exec_log_dir, config.paths.files.output)
    output_file = None
    try:
        output_file = open(output_file_path, "w", encoding="utf-8")
    except Exception as e:
        tlog.warning(f"无法创建 output.txt 文件: {e}")

    # SSE 会话上下文（进度状态 + 结果集；呈现统一经 ProgressUI 门面驱动）
    session = SseSession(
        output_file=output_file,
        ui=ui,
    )
    ui.start()

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
        # 统一收尾：停止线程/UI、关闭文件、通知并回收 Go 进程
        _shutdown_go(process, port, process_key, go_dead, health_stop, health_thread, ui, output_file)

    # 10. 统计结果
    success_count = sum(1 for r in session.results if r.get("connect_success") and r.get("exit_bool"))
    fail_count = len(session.results) - success_count
    tlog.info(f"任务执行完成，共 {len(session.results)} 条结果: 成功 {success_count}, 失败 {fail_count}")

    return session.results
