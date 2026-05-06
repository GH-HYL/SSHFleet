# -*- coding: utf-8 -*-
# SSHexec 检查文件
# 该文件负责检查文件是否存在、是否可写等操作

# 系统或第三方模块
import os
from posixpath import join as posix_join
from typing import Dict, Any
from fabric import Connection
from invoke.exceptions import UnexpectedExit

# 自定义模块
import src.transfer.transfer_utils as transfer_utils
from src.utils import elog


def check_path_exists(conn: Connection, path: str) -> bool:
    """检查路径是否存在"""
    try:
        conn.run(f"test -e {path}", hide=True, encoding="utf-8")
        elog.info(f"路径存在性检查 - 路径存在: {path}")
        return True
    except UnexpectedExit as e:
        if "WARNING: Your password has expired." in str(e):
            elog.error(f"密码过期\n{str(e)}")
            raise Exception("密码过期")
        else:
            elog.warning(f"路径存在性检查 - 路径不存在: {path}")
        return False


def check_disk_space(conn: Connection, current_path: str, original_size: int) -> int:
    """
    功能：
        检查远程路径所在磁盘的剩余空间是否充足

    参数：
        conn: Connection对象 - SSH连接对象
        current_path: str - 要检查的路径
        original_size: int - 原始文件大小（字节）
        use_sudo: bool - 是否使用sudo权限执行命令

    返回：
        disk_free - 剩余磁盘空间（KB）

    异常：
        抛出Exception当无法获取磁盘空间或空间不足时
    """
    # 构造命令获取指定路径的剩余空间(KB)
    df_command = f"df -Pk {current_path} | tail -n 1 | awk '{{print $4}}'"
    # 声明disk_free
    disk_free = 0

    try:
        # 获取磁盘剩余空间
        disk_free_result = conn.run(df_command, hide=True, encoding="utf-8")
        elog.info(
            f"剩余空间获取 - 命令获取路径 {current_path} 的磁盘空间信息成功，命令返回结果: {disk_free_result.stdout.strip()}"
        )
    except UnexpectedExit as e:
        elog.error(
            f"剩余空间获取 - 无法获取路径 {current_path} 的磁盘空间信息: {str(e)}"
        )
        raise Exception("剩余空间获取 - 获取磁盘信息失败")

    try:

        disk_free = int(disk_free_result.stdout.strip())
    except ValueError as e:
        elog.error(
            f"剩余空间获取 - 磁盘空间信息转换失败: {disk_free_result.stdout.strip()}\n异常类型：\n{type(e)}\n异常信息：\n{e}"
        )

    # 计算所需最小空间(原始大小的2倍，转换为KB)
    required_space = original_size / 512  # 等价于 original_size*2/1024

    # 检查磁盘空间是否充足
    if disk_free:
        if disk_free < required_space:
            error_msg = f"剩余空间获取 - 路径 {current_path} 磁盘空间不足, 需要 {required_space}KB, 剩余 {disk_free}KB"
            elog.error(error_msg)
            raise Exception("磁盘空间不足")
        else:
            elog.info(
                f"剩余空间获取 - 路径 {current_path} 磁盘空间充足, 剩余空间: {transfer_utils.format_size(disk_free*1000)}"
            )
    else:
        elog.warning(
            f"剩余空间获取 - 无法获取路径 {current_path} 的磁盘空间信息，跳过磁盘空间检查限制"
        )

    return disk_free


def check_single_path_writable(
    conn: Connection, current_path: str, use_sudo: bool
) -> None:
    """
    功能：
        检查单个路径是否可写

    参数：
        conn: Connection对象 - SSH连接对象
        current_path: str - 要检查的路径
        use_sudo: bool - 是否使用sudo权限执行命令

    返回：
        None
    """

    # 检查可写性（创建测试文件）
    import random
    import string

    random_str = "".join(random.choices(string.ascii_lowercase + string.digits, k=12))
    test_file = posix_join(current_path, f".write_test_{random_str}")

    # 检查路径是否可写
    try:
        if use_sudo:
            conn.sudo(f"touch {test_file}", hide=True, encoding="utf-8")
            conn.sudo(f"rm -f {test_file}", hide=True, encoding="utf-8")
        else:
            conn.run(f"touch {test_file}", hide=True, encoding="utf-8")
        # conn.run(f"test -w {test_file}", hide=True, encoding="utf-8")
        elog.info(f"可写性检查 - 路径可写: {current_path}")
    except UnexpectedExit as e:
        elog.error(
            f"可写性检查 - 目标路径{test_file}不可写\n异常类型：\n{type(e)}\n异常信息：\n{e}"
        )
        raise Exception("目标不可写")


def perform_pre_checks(
    conn: Connection,
    remote_path: str,
    original_size: int,
    use_sudo: bool,
    result: Dict[str, Any],
) -> None:
    """
    功能：
        执行所有预检查，包括路径是否存在、磁盘空间是否充足、路径是否可写

    参数：
        conn: Connection对象 - SSH连接对象
        remote_path: str - 要检查的远程路径
        original_size: int - 原始文件大小（字节）
        use_sudo: bool - 是否使用sudo权限执行命令
        result: Dict[str, Any] - 存储检查结果的字典

    返回：
        None
    """

    current_path = remote_path.rstrip("/")
    i = 0

    while True:
        elog.debug(f"路径存在性检查 - 当前路径: {current_path}")
        # 检查当前路径是否存在
        if check_path_exists(conn, current_path):
            # 检查远程路径磁盘可写性
            check_single_path_writable(conn, current_path, use_sudo)
            # 检查远程路径磁盘空间
            result["disk_free"] = check_disk_space(conn, current_path, original_size)
            return

        if current_path == "/":
            i += 1
            if i > 3:
                elog.error(
                    f"路径存在性检查 - 获取路径失败，{i}次查找 / 路径均失败，请检查执行命令环境是否正常"
                )
                raise Exception("获取路径失败")
            else:
                continue
        else:
            elog.debug("路径存在性检查 - 返回上一级路径")
            current_path = os.path.dirname(current_path)


def check_unix_os(conn: Connection) -> bool:
    """
    检查远程操作系统是否为Unix-like
    Args:
        conn: 已建立的连接
    Returns:
        bool: 是否为Unix-like操作系统
    """
    # 首选检查 uname
    try:
        os_result = conn.run("uname -s", hide=True, encoding="utf-8")
        os_result_str = os_result.stdout.strip().lower() if os_result.stdout else ""
        elog.info(f"uname -s 返回信息：{os_result_str}")
        if "linux" in os_result_str or "bsd" in os_result_str:
            elog.info("Unix操作系统检查通过")
            return True
        else:
            elog.warning("Unix操作系统检查未通过，停止本次传输任务")
            raise Exception("非Unix系统")
    except UnexpectedExit as e:
        elog.info(f"uname -s 返回信息：{e}")
        elog.error("uname -s 命令执行异常")
        raise Exception(str(e))
