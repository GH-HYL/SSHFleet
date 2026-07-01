# -*- coding: utf-8 -*-
# Go 进程调用与 HTTP 通信模块

import json
import os
import socket
import subprocess
import time
from typing import Dict, Generator, Optional

import requests

from src.yaml import SSHFleetConfig
from src.utils import tlog


def find_available_port() -> int:
    """找到一个可用端口"""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        port = s.getsockname()[1]
    tlog.info(f"分配可用端口: {port}")
    return port


def get_exe_path(config: SSHFleetConfig) -> str:
    """根据操作系统获取 Go 可执行文件路径"""
    if os.name == "nt":
        exe_path = config.paths.exe.batch_tool_windows
    elif os.name == "posix":
        exe_path = config.paths.exe.batch_tool_linux
        os.chmod(exe_path, 0o755)
    else:
        raise RuntimeError(f"不支持的平台: {os.name}")

    if not os.path.exists(exe_path):
        raise FileNotFoundError(f"Go 可执行文件不存在: {exe_path}")

    return exe_path


def start_go_process(exe_path: str, port: int, log_path: str = "") -> subprocess.Popen:
    """启动 Go 进程"""
    cmd = [exe_path, "--port", str(port)]
    if log_path:
        cmd.extend(["--log-path", log_path])

    process = subprocess.Popen(
        cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    tlog.info(f"Go 进程已启动，PID: {process.pid}，端口: {port}")
    return process


def wait_for_server(port: int, timeout: float = 10.0) -> bool:
    """等待 HTTP 服务就绪"""
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=1):
                return True
        except (ConnectionRefusedError, OSError):
            time.sleep(0.1)
    return False


def call_go(
    request_body: Dict,
    port: int,
    timeout: float = 120.0,
) -> Generator[Dict, None, None]:
    """
    发送 HTTP 请求并接收 SSE 流式响应

    Args:
        request_body: 请求体
        port: Go 服务端口
        timeout: 单条结果最大等待时间（秒），每次收到结果后重置

    Yields:
        dict: 单条执行结果
    """
    url = f"http://127.0.0.1:{port}/api/v1/execute"

    try:
        response = requests.post(
            url,
            json=request_body,
            stream=True,
            timeout=(5, None),  # 连接超时5秒，读取无超时（手动控制）
        )
        response.raise_for_status()
    except requests.RequestException as e:
        tlog.error(f"HTTP 请求失败: {e}")
        raise RuntimeError(f"HTTP 请求失败: {e}")

    last_recv_time = time.time()

    for line in response.iter_lines():
        # 动态超时检查：距离上次收到数据超过 timeout 则退出
        if time.time() - last_recv_time > timeout:
            msg = f"接收数据超时（{timeout}秒无新数据），已收到的结果将正常处理"
            tlog.error(msg)
            print(f"\n[red]警告: {msg}[/red]")
            break

        if not line:
            continue
        if isinstance(line, bytes):
            line = line.decode("utf-8")
        if not line.startswith("data: "):
            continue

        last_recv_time = time.time()
        try:
            data = json.loads(line[6:])
        except json.JSONDecodeError as e:
            tlog.warning(f"SSE 数据解析失败: {e}")
            continue
        if data.get("type") == "done":
            tlog.info(f"SSE 完成标记: total={data['total']}, success={data['success']}, failed={data['failed']}")
            return

        yield data

    response.close()


def collect_stderr(process: subprocess.Popen) -> str:
    """收集 Go 进程的 stderr 输出"""
    try:
        stderr = process.stderr
        if stderr:
            return stderr.read().decode("utf-8", errors="replace")
    except Exception:
        pass
    return ""


def shutdown_go_server(port: int) -> bool:
    """
    通知 Go 服务器关闭

    Args:
        port: Go 服务端口

    Returns:
        bool: 是否成功发送关闭信号
    """
    url = f"http://127.0.0.1:{port}/api/v1/shutdown"
    try:
        response = requests.post(url, timeout=5)
        if response.status_code == 200:
            tlog.info("已发送 Go 服务器关闭信号")
            return True
        else:
            tlog.warning(f"关闭信号响应异常: {response.status_code}")
            return False
    except requests.RequestException as e:
        tlog.warning(f"发送关闭信号失败: {e}")
        return False
