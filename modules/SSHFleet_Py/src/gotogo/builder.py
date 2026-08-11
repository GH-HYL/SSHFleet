# -*- coding: utf-8 -*-
# 请求体构建模块

import argparse
import os
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
                "key_content": node.get("key_content", ""),
                "key_passphrase": node.get("key_passphrase", ""),
            }
            for i, node in enumerate(nodesinfo)
        ],
    }


def build_upload_request(args: argparse.Namespace, nodesinfo: List[Dict]) -> Dict:
    """构建上传请求体"""
    return {
        # 本地路径转绝对（Go 契约：file_path 必须绝对路径）
        "file_path": os.path.abspath(args.u),
        # args.p 在 upload 模式是远程路径，不转绝对
        "remote_path": args.p,
        "options": {
            "concurrency": args.n,
            "connect_timeout": args.T,
            "exec_timeout": args.t,
            "sudo": args.m == "sudo",
        },
        "nodes": [
            {"seq": i, "ip": node["ip"], "port": node.get("port", 22),
             "user": node["user"], "password": node["password"],
             "key_content": node.get("key_content", ""),
             "key_passphrase": node.get("key_passphrase", "")}
            for i, node in enumerate(nodesinfo)
        ],
    }


def build_download_request(args: argparse.Namespace, nodesinfo: List[Dict]) -> Dict:
    """构建下载请求体"""
    return {
        # args.d（远程路径）已由 check_arguments 强制 / 开头，不动
        "remote_path": args.d,
        # args.p 在 download 模式是本地目录，转绝对（Go 契约：local_path 必须绝对路径）
        "local_path": os.path.abspath(args.p),
        "options": {
            "concurrency": args.n,
            "connect_timeout": args.T,
            "exec_timeout": args.t,
            "sudo": args.m == "sudo",
        },
        "nodes": [
            {"seq": i, "ip": node["ip"], "port": node.get("port", 22),
             "user": node["user"], "password": node["password"],
             "key_content": node.get("key_content", ""),
             "key_passphrase": node.get("key_passphrase", "")}
            for i, node in enumerate(nodesinfo)
        ],
    }
