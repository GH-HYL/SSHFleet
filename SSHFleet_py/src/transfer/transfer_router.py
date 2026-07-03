# -*- coding: utf-8 -*-
# SSHFleet 传输路由模块
# 负责决定上传走"命令模式"还是"SFTP模式"，并执行对应逻辑

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
    上传路由：统一走 Go API /upload

    Args:
        args: 命令行参数
        config: 配置对象
        nodesinfos: 节点信息列表
        exec_log_dir: 执行日志目录
        error_keywords: 错误分类关键词

    Returns:
        执行结果列表
    """
    # 旧入口注释备用（Go 自己递归目录，不需要 tar 压缩和 Fabric SFTP）
    # from src.transfer.transfer_precheck import transfer_precheck
    # from src.gotogo import go_to_go
    # transfer_command = transfer_precheck(args.u, args.p)
    # if transfer_command:
    #     results = go_to_go(args, config, nodesinfos, exec_log_dir, error_keywords, transfer_command)
    # else:
    #     import src.transfer.transfer as transfer
    #     results = transfer.execute_transfer(args, nodesinfos, error_keywords)

    # 新入口：统一走 Go API 上传
    from src.gotogo.go_to_go import go_to_go
    return go_to_go(args, config, nodesinfos, exec_log_dir, error_keywords)


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
