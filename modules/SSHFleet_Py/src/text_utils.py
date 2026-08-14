# -*- coding: utf-8 -*-
# SSHFleet 文本/数值格式化模块
# 集中管理：Excel 字符清洗、文件大小格式化等纯函数


def clean_for_excel(original_text, replace_tabs=False):
    """
    功能：
        移除所有 openpyxl / XML 无法处理的字符，保留常用空白符（\t \n \r）。    
        同时对 ANSI 转义序列进行二次清理（防止被转义后还残留控制码）。
    参数：
        original_text: 原始文本字符串
    返回：
        cleaned_text: 清理后的文本字符串
    """
    import re

    # 类型规范化
    if original_text is None:
        text = ""
    elif isinstance(original_text, bytes):
        text = original_text.decode('utf-8', errors='backslashreplace')
    elif isinstance(original_text, str):
        text = original_text
    else:
        text = str(original_text)

    # 正则：完整的 ANSI 序列
    ANSI_ESCAPE_RE = re.compile(
        r'\x1b\[[0-9;]*[a-zA-Z]'
        r'|\x1b\][^\x07]*(?:\x07|\x1b\\)'
        r'|\x1b[@-_][0-?]*[-/]*[@-~]'
    )

    # 正则：所有非打印/零宽字符（保留 \t \n \r）
    INVISIBLE_CHAR_RE = re.compile(
        r'[\x00-\x08\x0b\x0c\x0e-\x1f\x7f-\x9f'   # C0/C1 控制字符
        r'\ud800-\udfff'                         # 孤立代理项
        r'\u200b-\u200f\u2028-\u202f\u2060-\u206f\ufeff]'  # 零宽/格式字符
    )

    # 1. 移除 ANSI 序列
    text = ANSI_ESCAPE_RE.sub('', text)

    # 2. 移除所有不可见/非法字符（保留 \t \n \r 暂时）
    text = INVISIBLE_CHAR_RE.sub('', text)

    # 3. 统一换行符
    text = text.replace('\r\n', '\n').replace('\r', '\n')

    # 4. 可选：替换制表符为四个空格（避免 WPS 光标异常）
    if replace_tabs:
        text = text.replace('\t', '    ')

    # 5. 防止等号开头的行被 Excel 误认为公式
    lines = text.split('\n')
    for i, line in enumerate(lines):
        if line.lstrip().startswith('='):
            stripped = line.lstrip()
            indent = line[:len(line) - len(stripped)]
            lines[i] = indent + ' ' + stripped   # 在第一个 '=' 前加一个空格
    text = '\n'.join(lines)

    # 6. 移除开头和结尾的空行（维持整洁）
    text = text.strip('\n')

    return text


def format_size(size_bytes: int) -> str:
    """自适应文件大小单位（B/KB/MB/GB/TB/PB），保留2位小数"""
    units = ["B", "KB", "MB", "GB", "TB", "PB"]
    size = float(abs(size_bytes))
    for i, unit in enumerate(units):
        if i == len(units) - 1 or size < 1024:
            return f"{size:.2f} {unit}"
        size /= 1024
    return f"{size:.2f} {unit}"
