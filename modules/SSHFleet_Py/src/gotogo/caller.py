# -*- coding: utf-8 -*-
# Go 进程调用与 HTTP 通信模块

import json
import os
import secrets
import socket
import string
import subprocess
import time
from typing import Dict, Generator, Optional

import requests

from src.common.loader import SSHFleetConfig
from src.log import tlog


def generate_process_key() -> str:
    """生成随机进程key"""
    return ''.join(secrets.choice(string.ascii_letters + string.digits) for _ in range(32))


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


def start_go_process(exe_path: str, port: int, log_path: str) -> tuple:
    """启动 Go 进程，通过环境变量传递配置

    Returns:
        tuple: (process, process_key)
    """
    process_key = generate_process_key()

    env = os.environ.copy()
    env["SSH_FLEET_KEY"] = process_key
    env["SSH_FLEET_PORT"] = str(port)
    env["SSH_FLEET_LOG_PATH"] = log_path

    tlog.info(f"环境变量: SSH_FLEET_KEY={process_key}, SSH_FLEET_PORT={port}, SSH_FLEET_LOG_PATH={log_path}")

    process = subprocess.Popen(
        [exe_path],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env=env,
    )
    tlog.info(f"Go 进程已启动，PID: {process.pid}，端口: {port}")
    return process, process_key


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


def check_health(port: int, timeout: float = 3.0) -> bool:
    """检查 Go 进程健康状态"""
    try:
        resp = requests.get(
            f"http://127.0.0.1:{port}/api/v1/health",
            timeout=timeout,
        )
        return resp.status_code == 200
    except requests.RequestException:
        return False


def call_go(
    request_body: Dict,
    port: int,
    process_key: str,
    timeout: float = 120.0,
    url_path: str = "/api/v1/execute",
) -> Generator[Dict, None, None]:
    """
    发送 HTTP 请求并接收 SSE 流式响应

    Args:
        request_body: 请求体
        port: Go 服务端口
        process_key: 进程认证key
        timeout: 单条结果最大等待时间（秒），每次收到结果后重置
        url_path: API 端点路径

    Yields:
        dict: 单条执行结果
    """
    url = f"http://127.0.0.1:{port}{url_path}"
    headers = {
        "X-SSH-Fleet-Key": process_key,
        "Content-Type": "application/json",
    }

    try:
        response = requests.post(
            url,
            headers=headers,
            json=request_body,
            stream=True,
            timeout=(5, None),
        )
        if response.status_code >= 400:
            try:
                err_body = response.json()
                err_msg = err_body.get("error", {}).get("message", response.text)
                err_code = err_body.get("error", {}).get("code", "")
            except Exception:
                err_msg = response.text
                err_code = ""
            error_detail = f"[{err_code}] {err_msg}" if err_code else err_msg
            tlog.error(f"Go 返回错误: {error_detail}")
            print(f"\033[91m[ERROR]\033[0m Go 返回错误: {error_detail}")
            raise RuntimeError(error_detail)
    except requests.RequestException as e:
        tlog.error(f"HTTP 连接失败: {e}")
        print(f"\033[91m[ERROR]\033[0m HTTP 连接失败: {e}")
        raise RuntimeError(f"HTTP 连接失败: {e}")

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
            tlog.info(f"SSE 完成标记: total={data['total']}")
            # break 退出循环，让循环尾部的 response.close() 执行（return 会跳过关闭）
            break

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


def shutdown_go_server(port: int, process_key: str) -> bool:
    """
    通知 Go 服务器关闭

    Args:
        port: Go 服务端口
        process_key: 进程认证key

    Returns:
        bool: 是否成功发送关闭信号
    """
    url = f"http://127.0.0.1:{port}/api/v1/shutdown"
    headers = {"X-SSH-Fleet-Key": process_key}
    try:
        response = requests.post(url, headers=headers, timeout=5)
        if response.status_code == 200:
            tlog.info("已发送 Go 服务器关闭信号")
            return True
        else:
            tlog.warning(f"关闭信号响应异常: {response.status_code}")
            return False
    except requests.RequestException as e:
        tlog.warning(f"发送关闭信号失败: {e}")
        return False
