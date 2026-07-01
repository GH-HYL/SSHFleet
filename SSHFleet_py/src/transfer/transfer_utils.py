# -*- coding: utf-8 -*-
# SSHFleet 传输工具文件
# 该文件负责处理文件传输操作的工具函数，包括创建SSH连接、格式化文件大小等

# 系统或第三方模块
import os
import signal
import tarfile
import tempfile
from fabric import Connection, Config
from paramiko.config import SSHConfig
from typing import Dict, Any
from typing import Tuple, Optional

# 自定义模块
import src.utils as utils
from src import color
from src.utils import elog

# 全局中断标志
transfer_interrupted = False


def create_ssh_connection(node_info: Dict[str, Any], args: Any) -> Connection:
    """
    功能：
        创建SSH连接对象 - 适配Fabric 3.x版本

    参数：
        node_info: 节点信息字典，包含ip/port/user/password等
        args: 命令行参数对象

    返回：
        fabric.Connection对象
    """

    # 准备连接参数 - 只保留必要的参数
    connect_kwargs = {
        "timeout": getattr(args, "T", 5),  # 建立 TCP 网络连接的超时时间
        "auth_timeout": getattr(args, "T", 5),  # SSH 认证过程的超时时间
        "banner_timeout": getattr(
            args, "T", 5
        ),  # 接收 SSH 欢迎横幅（Banner）的超时时间
    }

    # 添加认证信息
    if args.k:
        # 密钥认证
        connect_kwargs.update(
            {
                "key_filename": args.k,
                "look_for_keys": True,
                "allow_agent": True,
            }
        )
    else:
        # 密码认证
        connect_kwargs.update(
            {
                "password": node_info["password"],
                "look_for_keys": False,
                "allow_agent": False,
            }
        )

    # 创建连接对象
    try:
        conn = Connection(
            host=node_info["ip"],
            port=node_info["port"],
            user=node_info["user"],
            connect_kwargs=connect_kwargs,
            config=Config(ssh_config=SSHConfig()),
        )

        elog.info(
            f"创建SSH连接成功: {node_info['ip']}:{node_info['port']} 账号：{node_info['user']}"
        )
        return conn
    except Exception as e:
        elog.error(
            f"创建SSH连接失败 {node_info['ip']}:{node_info['port']} 账号：{node_info['user']}\n异常类型：\n{type(e)}\n异常信息：\n{e}"
        )
        raise


def format_size(size_bytes: int) -> str:
    """
    功能：
        自适应文件大小单位（B/KB/MB/GB/TB/PB），保留2位小数

    参数：
        size_bytes: 文件大小（单位：字节）

    返回：
        格式化后的文件大小字符串
    """
    units = ["B", "KB", "MB", "GB", "TB", "PB"]
    size = float(abs(size_bytes))

    for i, unit in enumerate(units):
        # 如果是最后一个单位，或者当前单位足够表示
        if i == len(units) - 1 or size < 1024:
            return f"{size:.2f} {unit}"
        size /= 1024

    return f"{size:.2f} {unit}"


def calculate_tar_files(path: str) -> Tuple[int, int, Optional[str]]:
    """
    功能：
        计算文件或目录的原始大小和压缩后大小

    参数：
        path: 文件或目录路径

    返回：
        一个元组，包含原始大小（单位：字节）、压缩后大小（单位：字节）、临时压缩包路径（如果有）
    """
    try:
        path = os.path.abspath(os.path.normpath(path))
        if os.path.isfile(path):
            original_size = os.path.getsize(path)
            elog.info(
                f"返回文件计算大小数值: 路径{path}原始大小{format_size(original_size)}，单个文件跳过压缩"
            )
            return original_size, original_size, None
        elif os.path.isdir(path):
            # 计算原始大小
            original_total = 0
            for root, _, files in os.walk(path):
                for file in files:
                    file_path = os.path.join(root, file)
                    try:
                        # 跳过符号链接
                        if not os.path.islink(file_path):
                            original_total += os.path.getsize(file_path)
                    except OSError:
                        elog.warning(f"计算文件大小时，跳过无权限文件: {file_path}")
                        continue

            # # 创建临时压缩文件估算压缩后大小
            # temp_tar_path = None
            # # 默认使用原始大小
            # compressed_size = original_total

            try:
                temp_tar_path = os.path.join(
                    tempfile.gettempdir(), f"size_estimate_{os.urandom(4).hex()}.tar.gz"
                )
                with tarfile.open(temp_tar_path, "w:gz") as tar:
                    # 修改：保留完整的目录结构，包括顶层目录
                    tar.add(
                        path,
                        arcname=os.path.basename(path),
                        filter=lambda info: None if os.path.islink(info.name) else info,
                    )
                compressed_size = os.path.getsize(temp_tar_path)
                # 计算压缩率
                compression_ratio = (
                    compressed_size / original_total if original_total > 0 else 0
                )

            except Exception as e:
                elog.warning(f"压缩失败\n异常类型：\n{type(e)}\n异常信息：\n{e}")
                raise Exception("传输前压缩失败")

            elog.info(
                f"返回文件计算大小数值，原始：{format_size(original_total)}, 压缩：{format_size(compressed_size)}，压缩率：{compression_ratio:.2%}， 临时压缩路径：{temp_tar_path}"
            )
            return original_total, compressed_size, temp_tar_path
    except Exception as e:
        elog.error(f"计算大小失败返回全0和None\n异常类型：\n{type(e)}\n异常信息：\n{e}")
        return 0, 0, None

    elog.error("计算大小执行未正常结束，返回全0和None")
    return 0, 0, None


def chmod_chown_remote_files(conn, path, user, use_sudo):
    """
    统一修改权限和所有者
    """

    sudo_str = "sudo" if use_sudo else ""
    chown_command = f"{sudo_str} chown -R {user} {path}"
    if conn.run(chown_command, hide=True).ok:
        elog.info(f"修改所有者成功 {user}:{path} ")
    else:
        elog.error(f"修改所有者失败 {user}:{path}")

    return


def create_dir(conn, path, use_sudo):
    """
    功能：
        创建目录

    参数：
        conn: asyncssh连接对象
        dir: 目录路径
        use_sudo: 是否使用sudo权限

    返回：
        bool: 创建目录是否成功
    """

    mkdir_command = f"mkdir -p {path}"
    try:
        if use_sudo:
            mkdir_result = conn.sudo(mkdir_command, hide=True)
        else:
            mkdir_result = conn.run(mkdir_command, hide=True)
        if mkdir_result.ok:
            elog.info(f"创建目录成功 {path}")
            return True
    except Exception as e:
        elog.error(f"创建目录失败 {path}\n异常类型：\n{type(e)}\n异常信息：\n{e}")
        raise Exception("创建目录失败")


def initialize_result_dict(
    node_info: Dict[str, Any], args: Any, upload: bool = False, download: bool = False
) -> Dict[str, Any]:
    """初始化结果字典"""
    result = {
        "ip": node_info.get("ip"),
        "connect_bool": None,
        "exit_bool": None,
        "result_category": None,
        "thread_cost_time": 0.000,
        "disk_free": None,
        "upload_local": args.u if upload else None,
        "upload_remote": args.p if upload else None,
        "download_loacl": args.p if download else None,
        "download_remote": args.d if download else None,
        "thread_start_time": None,
        "thread_end_time": None,
        "port": node_info.get("port"),
        "user": node_info.get("user"),
        "password": node_info.get("password"),
        "connect_timeout": args.T,
        "transfer_timeout": args.t,
    }
    if upload:
        del result["download_loacl"]
        del result["download_remote"]
    elif download:
        del result["disk_free"]
        del result["upload_local"]
        del result["upload_remote"]
    else:
        utils.print_error_information_and_exit(
            "initialize_result_dict",
            f"参数错误,请联系开发者检查args.u（upload）和.args.d（download）值的有效性，当前值：{args.u}{upload},{args.d}{download}",
        )

    return result


# def handle_interrupt(signum=None, frame=None):
#     """全局中断信号处理器"""
#     global transfer_interrupted
#     if not transfer_interrupted:
#         transfer_interrupted = True
#         # 关键一步：立即屏蔽后续所有Ctrl+C
#         signal.signal(signal.SIGINT, signal.SIG_IGN)
#         elog.info("收到中断信号...")
#         print(f"{color.COLOR_RED}收到中断信号，正在停止任务...{color.COLOR_RESET}")


def handle_interrupt(signum=None, frame=None):
    global transfer_interrupted
    if not transfer_interrupted:
        transfer_interrupted = True
        signal.signal(signal.SIGINT, signal.SIG_IGN)  # 屏蔽后续
        elog.info("收到中断信号，正在优雅停止...")
        print(
            f"{color.COLOR_RED}收到中断信号，正在停止任务（再次按Ctrl+C强制退出）...{color.COLOR_RESET}"
        )
    else:
        # 第二次 Ctrl+C → 强制退出
        print(f"{color.COLOR_RED}强制退出！{color.COLOR_RESET}")
        os._exit(1)  # 强制终止，不走清理流程
