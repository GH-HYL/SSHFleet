# -*- coding: utf-8 -*-
# SSHFleet 传输路由模块
# 负责决定上传走"命令模式"还是"SFTP模式"，并执行对应逻辑

import os
from typing import Dict, List

from src import utils
from src.utils import tlog


def route_upload(
    args,
    config,
    nodesinfos: List[Dict],
    exec_log_dir: str,
    error_keywords: Dict,
) -> List[Dict]:
    """
    上传路由：根据文件类型选择执行方式

    纯文本文件 → 命令模式（通过Go SSH执行base64脚本）
    二进制文件 → SFTP模式（通过fabric传输）

    Args:
        args: 命令行参数
        config: 配置对象
        nodesinfos: 节点信息列表
        exec_log_dir: 执行日志目录
        error_keywords: 错误分类关键词

    Returns:
        执行结果列表
    """
    from src.transfer.transfer_precheck import transfer_precheck
    from src.gotogo import go_to_go

    tlog.info("开始传输预检查")

    # 预检查：判断是否纯文本
    transfer_command = transfer_precheck(args.u, args.p)

    if transfer_command:
        # 纯文本 → 命令模式
        tlog.info("上传目标是纯文本文件，使用命令模式")
        print("提示：检测到上传目标是纯文本文件，使用批量上传命令")
        results = go_to_go(args, config, nodesinfos, exec_log_dir, error_keywords, transfer_command)
        tlog.success("命令模式上传完成")
    else:
        # 二进制 → SFTP模式
        tlog.info("上传目标包含二进制文件，使用SFTP模式")
        utils.init_execution_logger(exec_log_dir, config.paths.logs.exec)
        tlog.success("初始化执行日志记录器成功")

        import src.transfer.transfer as transfer
        results = transfer.execute_transfer(args, nodesinfos, error_keywords)
        tlog.success("SFTP模式上传完成")

    return results


def route_download(
    args,
    config,
    nodesinfos: List[Dict],
    exec_log_dir: str,
    error_keywords: Dict,
) -> List[Dict]:
    """
    下载路由：下载固定走SFTP模式

    Args:
        args: 命令行参数
        config: 配置对象
        nodesinfos: 节点信息列表
        exec_log_dir: 执行日志目录
        error_keywords: 错误分类关键词

    Returns:
        执行结果列表
    """
    tlog.info("开始下载任务，使用SFTP模式")
    utils.init_execution_logger(exec_log_dir, config.paths.logs.exec)
    tlog.success("初始化执行日志记录器成功")

    import src.transfer.transfer as transfer
    results = transfer.execute_transfer(args, nodesinfos, error_keywords)
    tlog.success("SFTP模式下载完成")

    return results
