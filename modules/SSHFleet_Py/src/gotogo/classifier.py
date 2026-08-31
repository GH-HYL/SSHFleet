# -*- coding: utf-8 -*-
# 错误分类模块
#
# 设计原则（ADR-0003：退出码语义统一——只属于命令执行）：
#   exit_code 只描述"命令执行结果"，三态语义：
#   - 0     → 命令全部成功（execute=命令本身，upload/download=预处理+传输全部成功）；
#   - 非 0  → 有命令失败（execute=命令本身，upload/download=预处理命令失败），
#             退出码是权威信号，直接分类，不再匹配关键词；
#   - None  → 未执行任何命令（连接失败/传输失败/超时/中断），
#             靠 error/output 关键词匹配推断。
#
#   upload/download 的传输阶段（SFTP 读写）不是命令执行，失败时 Go 不设置退出码
#   （nil），由 error 报文关键词匹配分类；部分成功（failed>0 且 success>0）优先分类。
#   关键词未命中时不再返回笼统的"错误未分类"，而是把 error 报文原文作为分类，
#   保留具体失败内容；仅 error 与 output 均为空时才兜底"错误未分类"。

from src.common.constants import (
    SUCCESS_CATEGORY_EXECUTE,
    SUCCESS_CATEGORY_TRANSPORT,
    PARTIAL_SUCCESS_CATEGORY,
)

# 关键词匹配的哨兵值：未命中时不再直接返回它，
# 而是返回报错原文（见 classify 中 error/output 分支）
UNCLASSIFIED = "错误未分类"

# 兜底原文作为分类时的最大长度，超出部分截断，避免长报文撑爆统计/报表展示
FALLBACK_MAX_LEN = 200


def classify(
    exit_code: int | None,
    error: str | None,
    output: str,
    error_keywords: dict[str, list[str]],
    mode: str = "execute",
    success_files: int = 0,
    failed_files: int = 0,
) -> str:
    """
    根据响应体字段进行错误分类

    Args:
        exit_code: 命令退出码，None 表示未执行命令（连接失败/传输失败/超时/中断等）
        error: Go 层面原始错误信息
        output: 命令输出内容（已解码）
        error_keywords: 分类关键词映射
        mode: 执行模式，"execute"、"upload" 或 "download"
        success_files: 成功传输文件数（仅 upload/download 模式使用）
        failed_files: 失败传输文件数（仅 upload/download 模式使用）

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

    # 无退出码：命令未执行，问题在连接/传输等环节，用关键词匹配推断
    # 传输模式部分成功优先：有成功有失败是结构性状态，先于失败原因展示
    if mode in ("upload", "download") and failed_files > 0 and success_files > 0:
        return PARTIAL_SUCCESS_CATEGORY

    # 关键词未命中时，把报错原文直接作为分类（保留具体失败内容），
    # 不再返回笼统的"错误未分类"；仅 error 与 output 均为空时才兜底。
    if error:
        result = _match(error, error_keywords)
        if result != UNCLASSIFIED:
            return result
        return error.strip() or UNCLASSIFIED

    if output:
        result = _match(output, error_keywords)
        if result != UNCLASSIFIED:
            return result
        return _truncate(output.strip()) or UNCLASSIFIED

    return UNCLASSIFIED


def _truncate(text: str) -> str:
    """超长兜底原文截断，保留语义开头，避免长文本撑爆统计/报表展示"""
    if len(text) <= FALLBACK_MAX_LEN:
        return text
    return text[:FALLBACK_MAX_LEN] + "…"


def _match(text: str, error_keywords: dict) -> str:
    """关键词匹配，返回第一个命中的分类（按分类定义顺序，自上而下）"""
    text_lower = text.lower()
    for category, keywords in error_keywords.items():
        for kw in keywords:
            if _keyword_hit(kw.lower(), text_lower):
                return category
    return UNCLASSIFIED


def _keyword_hit(keyword: str, text: str) -> bool:
    """
    单条关键词的命中判定（通配符语义见 error_keywords.yaml 文件头部）

    支持的通配符只有星号 *，含义是"任意长度的内容（可以为空）"。
    判定规则：把关键词按 * 切成片段，要求片段在文本中按顺序出现即可，
    不要求从头开始、到尾结束，片段之间也不要求相邻。
    不含 * 时退化为普通子串包含匹配。

    Args:
        keyword: 已转小写的关键词
        text: 已转小写的待匹配文本

    Returns:
        bool: 是否命中
    """
    if "*" not in keyword:
        return keyword in text

    pos = 0
    for part in keyword.split("*"):
        if not part:
            continue
        idx = text.find(part, pos)
        if idx < 0:
            return False
        pos = idx + len(part)
    return True


def is_fallback_category(category: str, error_keywords: dict) -> bool:
    """
    判断分类名是否为"关键词未命中时的兜底原文"（区别于规范分类名）

    兜底原文 = classify 中 error/output 分支关键词未命中时返回的原始报文，
    内容不定且一般较长，终端展示时需要特殊处理（独占一行）。

    规范分类名包含：关键词表分类、动态生成类（执行失败(退出码N)）、
    结构性分类（部分成功）、成功分类与完全无信息的兜底（错误未分类）。

    Args:
        category: 分类名
        error_keywords: 分类关键词映射

    Returns:
        bool: 是兜底原文返回 True
    """
    if not category or category == UNCLASSIFIED:
        return False
    if category in error_keywords:
        return False
    if category in (SUCCESS_CATEGORY_EXECUTE, SUCCESS_CATEGORY_TRANSPORT):
        return False
    if category == PARTIAL_SUCCESS_CATEGORY:
        return False
    if category.startswith("执行失败(退出码"):
        return False
    return True
