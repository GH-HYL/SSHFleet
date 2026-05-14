# -*- coding: utf-8 -*-
# SSHexec 命令执行器入口
# 该文件负责与Go程序建立网络连接，接收进度和结果数据，并将其转换为Python格式

import argparse
import json
import os
import socket
import subprocess
import sys
import threading
import time
from tkinter import N
from typing import Dict, List
from threading import Thread
from queue import Queue, Empty
from rich.console import Console
from rich.live import Live
from rich.progress import Progress, BarColumn, TextColumn, TimeElapsedColumn

from src import utils
from src.toml import SSHExecConfig
from src.utils import tlog

# 全局控制台输出对象，用于美化命令行输出
console = Console()


def handle_go_connection(
    conn: socket.socket,
    completion_event: threading.Event,
    node_progress,
    node_task,
    total_nodes: int,
    results: List[Dict],
) -> None:
    """
    功能：
        处理与Go程序建立的网络连接，接收实时进度和结果数据

    参数：
        conn: 网络连接套接字对象
        completion_event: 线程事件，用于标记任务完成状态
        node_progress: 进度条对象，用于显示执行进度
        node_task: 进度条任务ID
        total_nodes: 总共需要执行的节点数量
        results: 存储执行结果的列表

    返回：
        无返回值
    """

    # ✅ 改1：使用字节缓冲区，而不是字符串缓冲区
    buffer = b""
    try:
        while not completion_event.is_set():
            try:
                # ✅ 改2：接收原始字节，不解码
                data = conn.recv(10 * 1024 * 1024)
                if not data:
                    tlog.debug("Monitor Server - Go程序主动关闭了连接")
                    completion_event.set()
                    break

            except socket.timeout:
                continue
            except Exception as e:
                tlog.error(f"Monitor Server - 接收数据时发生异常: {e}")
                completion_event.set()
                break

            # 把新收到的字节追加到缓冲区
            buffer += data

            # ✅ 改3：按字节换行符 b'\n' 切分完整行
            while b"\n" in buffer:
                line_bytes, buffer = buffer.split(b"\n", 1)
                line = line_bytes.decode("utf-8").strip()
                if line:
                    try:
                        msg_data = json.loads(line)
                        msg_type = msg_data.get("type")

                        if msg_type == "progress":
                            completed = msg_data.get("completed", 0)
                            # 计算向下取整的百分比
                            percent_int = int(completed / total_nodes * 100)   # 直接截断小数，等价于 floor
                            percent_display = f"{percent_int:>3}%"             # 格式化为 " 99%" 或 "100%"
                            
                            node_progress.update(
                                node_task,
                                description=f"执行进度 [bright_yellow]已完成: [bright_black]{completed}/{total_nodes}",
                                completed=completed,
                                percent_display=percent_display,
                                # 注意：不要传入 total 参数，保持原总数不变
                            )

                        elif msg_type == "completion":
                            go_results = msg_data.get("results", [])
                            tlog.info(
                                f"Monitor Server - 所有任务执行完成，收到结果数量: {len(go_results)}"
                            )
                            results.extend(go_results)
                            completion_event.set()

                    except Exception as e:
                        tlog.error(
                            f"Monitor Server - 处理消息异常: {e}, 原始消息: {line}"
                        )

    except Exception as e:
        tlog.error(f"Monitor Server - 处理连接时发生异常: {e}")
        completion_event.set()
    finally:
        try:
            conn.close()
            tlog.debug("Monitor Server - 网络连接已关闭")
        except:
            pass

def capture_subprocess_output(process: subprocess.Popen) -> tuple[Thread, Thread]:
    """
    功能：
        捕获Go子进程的标准输出和错误输出，并实时显示在控制台

    参数：
        process: 子进程对象

    返回：
        无返回值
    """

    def read_stream(stream, output_queue):
        """读线程：只管读，扔队列，不阻塞"""
        try:
            while True:
                line = stream.readline()
                if not line:
                    output_queue.put(None)  # 结束信号
                    break
                output_queue.put(line)
        except Exception:
            output_queue.put(None)

    def flush_output(output_queue):
        """输出线程：按时间间隔批量刷，控制重绘频率"""
        FLUSH_INTERVAL = 0.05  # 50ms刷一次，平衡速度和实时性
        last_flush = time.time()
        batch = []

        while True:
            try:
                # 最多等50ms拿一条数据
                line = output_queue.get(timeout=FLUSH_INTERVAL)
                if line is None:
                    # 结束信号，刷完剩余
                    if batch:
                        sys.stdout.write("".join(batch))
                        sys.stdout.flush()
                    break
                batch.append(line)
            except Empty:
                pass

            # 时间到了就刷，或者累积太多也刷
            now = time.time()
            if batch and (now - last_flush >= FLUSH_INTERVAL or len(batch) >= 200):
                sys.stdout.write("".join(batch))
                sys.stdout.flush()
                batch.clear()
                last_flush = now

    stdout_queue = Queue()
    stderr_queue = Queue()

    # 每个流：1个读线程 + 1个输出线程
    stdout_reader = threading.Thread(
        target=read_stream, args=(process.stdout, stdout_queue), daemon=True
    )
    stdout_flusher = threading.Thread(
        target=flush_output, args=(stdout_queue,), daemon=True
    )
    stderr_reader = threading.Thread(
        target=read_stream, args=(process.stderr, stderr_queue), daemon=True
    )
    stderr_flusher = threading.Thread(
        target=flush_output, args=(stderr_queue,), daemon=True
    )

    stdout_reader.start()
    stdout_flusher.start()
    stderr_reader.start()
    stderr_flusher.start()

    return stdout_flusher, stderr_flusher


# @utils.error_and_exit_handling_decorator("go_to_go", "Go程序批量任务执行失败")
def go_to_go(
    args: argparse.Namespace,
    config: SSHExecConfig,
    nodesinfo: List[Dict],
    exec_log_dir: str,
    error_keywords: Dict[str, List[str]],
    transfer_command: str | None = None,
) -> List[Dict]:
    """
    功能：
        主执行函数 - 启动Go程序执行SSH批量任务，通过网络套接字实时监控进度

    参数：
        args: 命令行参数对象
        config: SSH执行配置对象
        nodesinfo: 节点信息列表，包含要执行SSH任务的服务器信息
        exec_log_dir: 执行日志存储目录路径
        error_keywords: 错误关键词字典，用于结果分析

    返回：
        List[Dict]: 执行结果列表，包含每个节点的执行状态和输出
    """
    tlog.info("开始执行Go程序批量任务（网络套接字版本）")

    # 计算总共需要处理的节点数量
    total_nodes = len(nodesinfo)

    # 创建进度条显示组件
    node_progress = Progress(
        TextColumn("    执行进度"),
        BarColumn(bar_width=40, complete_style="green", finished_style="blue"),
        TextColumn("{task.fields[percent_display]}"),      # 使用自定义字段
        "[green]{task.completed}/{task.total}",
        TimeElapsedColumn(),
    )

    # 添加进度条任务
    node_task = node_progress.add_task("", total=total_nodes)

    # 初始化结果存储列表
    results = []

    # 创建TCP服务器套接字，用于与Go程序通信
    server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    # 绑定到本地回环地址和随机可用端口
    server.bind(("127.0.0.1", 0))
    # 只允许一个连接，Go程序执行完成后会断开连接
    server.listen(1)
    # Monitor Server 连接超时时间设置为 10 秒
    server.settimeout(10)
    # 获取系统分配给的端口号
    port = server.getsockname()[1]

    # 构建发送给Go程序的请求数据
    request_data = {
        "args": vars(args),  # 将命名空间对象转换为字典
        "config": config.model_dump(),  # 将配置对象序列化为字典
        "nodesinfo": nodesinfo,
        "monitor_info": f"127.0.0.1:{port}",  # 监控服务器地址信息
        "exec_log_dir": exec_log_dir,
        "final_command": transfer_command if transfer_command else utils.build_final_command(args),  # 构建最终执行的命令
        "error_keywords": error_keywords,
    }

    # 将请求数据序列化为JSON格式
    request_json = json.dumps(request_data, ensure_ascii=False, indent=4)

    # print(request_json)
    # sys.exit(1)

    # 根据操作系统类型选择对应的Go可执行文件路径
    if os.name == "nt":
        exe_path = config.paths.exe.batch_tool_windows
    elif os.name == "posix":
        exe_path = config.paths.exe.batch_tool_linux
        os.chmod(exe_path, 0o755)
    else:
        utils.print_error_informantion_and_exit(
            "gotogo.go_to_go", f"不支持的平台: {os.name}"
        )

    # 检查可执行文件是否存在
    if not os.path.exists(exe_path):
        utils.print_error_informantion_and_exit(
            "gotogo.go_to_go", f"Go可执行文件不存在: {exe_path}"
        )

    tlog.info(f"当前系统：{os.name}；程序路径：{exe_path}")

    try:
        # 启动Go程序作为子进程
        process = subprocess.Popen(
            [exe_path],
            stdin=subprocess.PIPE,  # 标准输入管道，用于发送数据
            stdout=subprocess.PIPE,  # 标准输出管道，用于接收输出
            stderr=subprocess.PIPE,  # 错误输出管道，用于接收错误信息
            text=True,  # 使用文本模式
            encoding="utf-8",  # 使用UTF-8编码
            bufsize=1,  # 行缓冲模式
        )

        # 将请求数据写入Go程序的标准输入
        process.stdin.write(request_json)  # pyright: ignore[reportOptionalMemberAccess]
        # 确保数据被发送
        process.stdin.flush()  # pyright: ignore[reportOptionalMemberAccess]
        # 关闭输入流，表示数据发送完成
        process.stdin.close()  # pyright: ignore[reportOptionalMemberAccess]

        # 启动输出捕获线程，实时显示Go程序的输出
        stdout_thread, stderr_thread = capture_subprocess_output(process)

        # 使用Live组件动态显示进度条
        live = Live(node_progress, console=console, refresh_per_second=20)
        live.start()

        # 等待Go程序连接到监控服务器
        conn, addr = server.accept()
        tlog.success(f"Monitor Server - 启动成功， {addr}")

        # 创建线程事件，用于标记任务完成状态
        completion_event = threading.Event()

        # 创建新线程专门处理与Go程序的网络通信
        monitor_thread = threading.Thread(
            target=handle_go_connection,
            args=(
                conn,
                completion_event,
                node_progress,
                node_task,
                total_nodes,
                results,
            ),
            name="SSHExecMonitorServerThread",
            daemon=True,  # 设置为守护线程，主线程退出时自动结束
        )
        monitor_thread.start()

        # 等待Go程序进程执行完成
        tlog.debug(f"日志内容轮转到文件: {exec_log_dir}/{config.paths.logs.exec}")
        process.wait()
        tlog.info("Go程序 - “执行”进程已退出")

        # 等待连接处理线程完成，最多等待10秒
        monitor_thread.join(timeout=10.0)

        if monitor_thread.is_alive():
            print("Monitor Server - 进度条监控线程超时，强制结束")
            tlog.warning("Monitor Server - 进度条监控线程超时，强制结束")
        else:
            tlog.info("Monitor Server - 进度条监控线程已正常结束")

        # 等待读取线程退出
        stdout_thread.join(timeout=30)
        stderr_thread.join(timeout=30)
        if stdout_thread.is_alive() or stderr_thread.is_alive():
            print("Monitor Server - 捕获输出线程超时，强制结束")
            tlog.warning("Monitor Server - 捕获输出线程超时，强制结束")
        else:
            tlog.info("Monitor Server - 捕获输出线程已正常结束")

        # 停止Live显示
        live.stop()

        # 检查是否成功接收到完成信号
        if not completion_event.is_set():
            print("Monitor Server - 未收到完成信号，可能执行异常")
            tlog.warning("Monitor Server - 未收到完成信号，可能执行异常")

        # 统计执行结果信息
        success_count = sum(
            1 for r in results if r.get("connect_success") and r.get("exit_bool")
        )
        fail_count = len(results) - success_count
        tlog.info(
            f"Monitor Server - 任务执行完成，共收集到 {len(results)} 条结果，执行统计: 成功 {success_count}, 失败 {fail_count}"
        )

        return results

    except socket.timeout:
        # 处理连接超时异常
        error_msg = "Monitor Server - 等待Go程序连接超时（10秒）"
        console.print(error_msg, style="red")
        tlog.error(error_msg)
        raise RuntimeError(error_msg)
    except json.JSONDecodeError as e:
        # 处理JSON解析异常
        error_msg = (
            f"Monitor Server - 解析程序 {os.path.basename(exe_path)} 响应失败: {e}"
        )
        console.print(error_msg, style="red")
        tlog.error(error_msg)
        raise RuntimeError(error_msg)
    except Exception as e:
        # 处理其他异常
        error_msg = (
            f"Monitor Server - {os.path.basename(exe_path)} 执行过程中出现异常: {e}"
        )
        console.print(error_msg, style="red")
        tlog.error(error_msg)
        raise RuntimeError(error_msg)
    finally:
        # 确保资源被正确清理，关闭服务器套接字
        try:
            server.close()
        except:
            pass
