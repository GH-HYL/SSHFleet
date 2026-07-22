# -*- coding: utf-8 -*-
# 请求体构建模块

import argparse
from typing import Dict, List, Optional

from src.command.builder import build_final_command


def build_request(
    args: argparse.Namespace,
    nodesinfo: List[Dict],
    transfer_command: Optional[str] = None,
) -> Dict:
    """
    从命令行参数和节点信息构建 Go API 请求体

    Args:
        args: 命令行参数
        nodesinfo: 节点信息列表
        transfer_command: 传输命令（可选，覆盖自动构建的命令）

    Returns:
        dict: 符合 Go API 规范的请求体
    """
    command = transfer_command if transfer_command else build_final_command(args)

    return {
        "command": command,
        "options": {
            "concurrency": args.n,
            "connect_timeout": args.T,
            "exec_timeout": args.t,
        },
        "nodes": [
            {
                "seq": i,
                "ip": node["ip"],
                "port": node.get("port", 22),
                "user": node["user"],
                "password": node["password"],
            }
            for i, node in enumerate(nodesinfo)
        ],
    }


def build_upload_request(args: argparse.Namespace, nodesinfo: List[Dict]) -> Dict:
    """构建上传请求体"""
    return {
        "file_path": args.u,
        "remote_path": args.p,
        "options": {
            "concurrency": args.n,
            "connect_timeout": args.T,
            "exec_timeout": args.t,
            "sudo": args.m == "sudo",
        },
        "nodes": [
            {"seq": i, "ip": node["ip"], "port": node.get("port", 22),
             "user": node["user"], "password": node["password"]}
            for i, node in enumerate(nodesinfo)
        ],
    }
