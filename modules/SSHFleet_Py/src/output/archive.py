# -*- coding: utf-8 -*-
# SSHFleet 归档模块 - 保存执行资源文件

import argparse
import os
import shutil
from pathlib import Path
from posixpath import join as posix_join

from src.common.error_handler import error_and_exit_handling_decorator
from src.common.loader import SSHFleetConfig
from src.log import tlog


@error_and_exit_handling_decorator(
    "save_execute_resource_files", "保存执行资源文件失败", isexit=True
)
def save_execute_resource_files(
    args: argparse.Namespace, log_dir: str, config: SSHFleetConfig
) -> None:
    """
    功能：
        保存执行资源文件（脚本、上传文件、配置文件）到指定目录。

    参数：
        args: 命令行参数
        log_dir: 日志目录路径
        config: 配置对象

    返回：
        None
    """
    # 创建资源存放目录
    resources_dir = posix_join(log_dir, config.paths.files.asset)
    os.makedirs(resources_dir, exist_ok=True)

    # 处理 -s 参数（单个文件）
    if hasattr(args, "s") and args.s:
        src_path = Path(args.s)
        if src_path.exists():
            dst_path = Path(resources_dir) / src_path.name
            shutil.copy2(src_path, dst_path)  # 保留元数据复制

    # 处理 -f 参数（单个文件）
    if hasattr(args, "f") and args.f:
        src_path = Path(args.f)
        if src_path.exists():
            dst_path = Path(resources_dir) / src_path.name
            shutil.copy2(src_path, dst_path)
    tlog.success("保存执行资源文件成功")

