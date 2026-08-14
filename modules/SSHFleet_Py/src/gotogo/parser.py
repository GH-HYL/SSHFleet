# -*- coding: utf-8 -*-
# SSE 响应解析模块

import base64
from typing import Dict

from src.gotogo.classifier import classify


def parse_result(sse_data: Dict, error_keywords: Dict[str, list[str]], mode: str = "execute") -> Dict:
    """
    解析 SSE 单条结果，补充 Python 端字段

    Args:
        sse_data: Go 返回的单条结果
        error_keywords: 错误分类关键词
        mode: 执行模式，"execute"、"upload" 或 "download"

    Returns:
        dict: 完整的结果字典（兼容 core.results_statistics）
    """
    output = decode_output(sse_data.get("output", ""))
    error = sse_data.get("error")

    return {
        "seq": sse_data.get("seq"),
        "ip": sse_data.get("ip"),
        "port": sse_data.get("port"),
        "user": sse_data.get("user"),
        "connect_success": sse_data.get("connect_success", False),
        "exit_bool": sse_data.get("connect_success", False) and sse_data.get("exit_code") == 0,
        "exit_code": sse_data.get("exit_code"),
        "output": output,
        "connect_cost_time": sse_data.get("connect_cost_time", 0),
        "exec_cost_time": sse_data.get("exec_cost_time", 0),
        "result_category": classify(
            exit_code=sse_data.get("exit_code"),
            error=error,
            output=output,
            error_keywords=error_keywords,
            mode=mode,
        ),
        "error": error,
        # 上传专属字段
        "total_bytes": sse_data.get("total_bytes", 0),
        "total_files": sse_data.get("total_files", 0),
        "success_files": sse_data.get("success_files", 0),
        "failed_files": sse_data.get("failed_files", 0),
    }


def decode_output(encoded: str) -> str:
    """base64 解码输出"""
    if not encoded:
        return ""
    try:
        return base64.b64decode(encoded).decode("utf-8", errors="replace")
    except Exception:
        return ""
