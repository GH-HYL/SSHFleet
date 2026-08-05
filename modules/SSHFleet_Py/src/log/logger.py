# -*- coding: utf-8 -*-
# SSHFleet 日志模块
# 该文件负责定义日志相关的函数和变量

import os
import sys
from datetime import datetime
from posixpath import join as posix_join

from loguru import logger

# 自定义模块
import src.color as color
from src.yaml import SSHFleetConfig

# 初始化全局logger变量
tlog = logger.bind(logger_type="tool")
elog = logger.bind(logger_type="exec")


def init_tool_logger(log_dir: str, config: SSHFleetConfig):
    global tlog

    os.makedirs(log_dir, exist_ok=True)

    # 移除默认handler
    logger.remove()

    # 添加tool日志handler
    tlog.add(
        os.path.join(log_dir, config.paths.logs.tool),
        rotation="50 MB",
        level="DEBUG",
        encoding="utf-8",
        format="{time:YYYY-MM-DD HH:mm:ss.SSS} - [{level: ^7}] - {message}",
        filter=lambda record: record["extra"].get("logger_type") == "tool",
    )

    return tlog


def create_exec_log_dir(args, config) -> str:
    """
    功能：
        创建日志目录

    参数：
        args: 命令行参数
        config: 配置对象

    返回：
        日志目录路径
    """
    from src.utils import print_error_information_and_exit

    try:
        # 使用可读的日期时间格式，而不是时间戳
        timestamp = datetime.now().strftime("%Y-%m-%d_%H-%M-%S")

        if args.c:
            file_name = "command"
        elif args.s:
            file_name = "script"
        elif args.u:
            file_name = "upload"
        else:
            file_name = "unknown"  # 添加默认值避免未定义

        # 最后拼接成的大概路径样子是 history/2025-08-20_12-12-12_command/
        log_dir = posix_join(config.paths.logs.historys, f"{timestamp}_{file_name}")

        if args.r:
            log_dir = log_dir + f"_{args.r.strip()}"

        # 生成日志目录（创建完整路径）
        os.makedirs(log_dir, exist_ok=True)

        return log_dir
    except Exception as e:
        tlog.error(
            f"create_exec_log_dir，创建日志目录失败\n异常类型：\n{type(e)}\n异常信息：\n{e}"
        )
        print_error_information_and_exit(
            "create_exec_log_dir",
            f"创建日志目录失败\n异常类型：{type(e)}\n异常信息：\n{e}",
        )


def init_execution_logger(log_dir: str, log_exec: str):
    global elog

    from src.utils import print_error_information_and_exit

    try:
        os.makedirs(log_dir, exist_ok=True)

        # 添加execution日志handler
        elog.add(
            os.path.join(log_dir, log_exec),
            level="DEBUG",
            enqueue=True,  # 启用线程安全队列
            encoding="utf-8",
            format="{time:YYYY-MM-DD HH:mm:ss.SSS} - [{level: ^7}] - {message}",
            filter=lambda record: record["extra"].get("logger_type") == "exec",
        )
        return elog
    except Exception as e:
        tlog.error(
            f"init_execution_logger，初始化执行日志记录器失败\n异常类型：\n{type(e)}\n异常信息：\n{e}"
        )
        print_error_information_and_exit(
            "init_execution_logger",
            f"初始化执行日志记录器失败\n异常类型：{type(e)}\n异常信息：\n{e}",
        )


def create_latest_log_symlink(config: SSHFleetConfig):
    """
    功能：
        创建最新日志符号链接

    参数：
        None

    返回：
        None
    """

    from src.utils import print_error_information_and_exit

    try:
        # 检查一下当前系统环境
        if os.name != "posix":
            tlog.warning("当前系统环境不是POSIX兼容系统，无法创建符号链接")
            return

        if not os.path.isdir(config.paths.logs.historys):
            print("错误: 历史记录目录 (historys) 不存在")
            tlog.error("历史记录目录 (historys) 不存在")
            return

        # 获取historys目录下所有子目录，并按创建时间倒序排序
        log_dirs = []
        for entry in os.scandir(config.paths.logs.historys):
            if entry.is_dir():
                log_dirs.append(entry)
        # 按创建时间排序，最新在前
        log_dirs.sort(key=lambda x: x.stat().st_ctime, reverse=True)

        if not log_dirs:
            print(
                f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:create_latest_log_symlink]{color.COLOR_RESET} 历史记录目录 '{config.paths.logs.historys}' 中没有日志文件夹",
                file=sys.stderr,
            )
            print("提示: 请先至少一次执行任务以生成历史记录")
            tlog.error("历史记录目录 (historys) 中没有日志文件夹")
            return

        latest_log_dir = posix_join(config.paths.logs.historys, log_dirs[0].name)
        latest_link = "latest_history"

        if os.path.islink(latest_link):
            os.remove(latest_link)
        elif os.path.exists(latest_link):
            print(f"警告: 已存在同名文件 {latest_link}，无法创建符号链接")
            tlog.warning(f"已存在同名文件 {latest_link}，无法创建符号链接")
            return
        os.symlink(latest_log_dir, latest_link)
        tlog.success(f"创建最新日志符号链接函数执行成功，指向路径: {latest_log_dir}")
    except OSError as e:
        print(f"创建符号链接失败: {str(e)}")
        tlog.error(
            f"创建最新日志符号链接函数执行失败\n异常类型：\n{type(e)}\n异常信息：\n{e}"
        )
        if e.errno == 1:
            print("提示: 请尝试使用管理员/root权限运行")
            tlog.error("创建最新日志符号链接函数执行失败，权限不足")
        print_error_information_and_exit(
            "create_latest_log_symlink",
            f"创建最新日志符号链接函数执行失败\n异常类型：{type(e)}\n异常信息：\n{e}",
            True,
        )
    except Exception as e:
        tlog.error(
            f"create_latest_log_symlink，创建最新日志符号链接失败\n异常类型：\n{type(e)}\n异常信息：\n{e}"
        )
        print_error_information_and_exit(
            "create_latest_log_symlink",
            f"创建最新日志符号链接失败\n异常类型：{type(e)}\n异常信息：\n{e}",
            True,
        )
