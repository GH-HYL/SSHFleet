# -*- coding: utf-8 -*-
# 错误分类模块


def classify(
    connect_success: bool,
    exit_code: int,
    error: str | None,
    output: str,
    error_keywords: dict[str, list[str]],
    mode: str = "execute",
) -> str:
    """
    根据响应体字段进行错误分类

    Args:
        connect_success: 连接是否成功
        exit_code: 命令退出码
        error: Go 层面原始错误信息
        output: 命令输出内容（已解码）
        error_keywords: 分类关键词映射
        mode: 执行模式，"execute" 或 "upload"

    Returns:
        str: 分类名称
    """
    if not connect_success:
        return _match(error or "", error_keywords)

    if exit_code == 0:
        return "传输成功" if mode == "upload" else "执行成功"

    if error:
        result = _match(error, error_keywords)
        if result != "错误未分类":
            return result

    if output:
        result = _match(output, error_keywords)
        if result != "错误未分类":
            return result

    return f"退出码={exit_code}"


def _match(text: str, error_keywords: dict) -> str:
    """关键词匹配，返回第一个命中的分类"""
    text_lower = text.lower()
    for category, keywords in error_keywords.items():
        for kw in keywords:
            if kw.lower() in text_lower:
                return category
    return "错误未分类"
