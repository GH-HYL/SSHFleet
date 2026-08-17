# -*- coding: utf-8 -*-
# 错误分类模块
#
# 设计原则：
#   退出码是命令执行结果的权威信号。
#   - 有退出码 → 命令/传输流程正常执行到了产生结果，直接按退出码分类；
#   - 无退出码 → 命令未执行，问题在连接/会话/预检等环节，才用关键词匹配推断。

from src.common.constants import SUCCESS_CATEGORY_EXECUTE, SUCCESS_CATEGORY_TRANSPORT


def classify(
    exit_code: int | None,
    error: str | None,
    output: str,
    error_keywords: dict[str, list[str]],
    mode: str = "execute",
) -> str:
    """
    根据响应体字段进行错误分类

    Args:
        exit_code: 命令退出码，None 表示未执行（连接失败/超时/中断等）
        error: Go 层面原始错误信息
        output: 命令输出内容（已解码）
        error_keywords: 分类关键词映射
        mode: 执行模式，"execute"、"upload" 或 "download"

    Returns:
        str: 分类名称
    """
    # 有退出码：按退出码分类，不再对输出做关键词匹配
    if exit_code is not None:
        if exit_code == 0:
            if mode in ("upload", "download"):
                return SUCCESS_CATEGORY_TRANSPORT
            return SUCCESS_CATEGORY_EXECUTE
        return f"执行失败(退出码{exit_code})"

    # 无退出码：命令未执行，问题在连接等环节，用关键词匹配推断
    if error:
        result = _match(error, error_keywords)
        if result != "错误未分类":
            return result

    if output:
        result = _match(output, error_keywords)
        if result != "错误未分类":
            return result

    return "错误未分类"


def _match(text: str, error_keywords: dict) -> str:
    """关键词匹配，返回第一个命中的分类"""
    text_lower = text.lower()
    for category, keywords in error_keywords.items():
        for kw in keywords:
            if kw.lower() in text_lower:
                return category
    return "错误未分类"
