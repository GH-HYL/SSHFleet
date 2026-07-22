# -*- coding: utf-8 -*-
# SSHFleet 工具文件
# 该文件负责定义通用的工具函数，包括参数解析、配置加载、日志初始化等

import argparse
import base64

# 系统或第三方模块
import os
import re
import shlex
import sys

from datetime import datetime
from posixpath import join as posix_join

from loguru import logger

# 自定义模块
import src.color as color
from src.yaml import SSHFleetConfig

# 向后兼容：从日志模块导入tlog和迁移的函数
from src.log import tlog
from src.log import init_tool_logger, init_execution_logger, create_exec_log_dir, create_latest_log_symlink


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

def get_user_confirmation(prompt, yorn=False):
    """
    功能：
        获取用户确认的通用函数

    参数：
        prompt: 确认提示信息
        yorn: 是否为 yes/no 确认，默认 False

    返回：
        confirm: 用户确认结果，True 或 False
    """

    try:
        if yorn:
            confirm = (
                input(f"{prompt} {color.COLOR_RED}[Y/n]{color.COLOR_RESET}: ")
                .strip()
                .lower()
                or "y"
            )
        else:
            confirm = (
                input(f"{prompt} {color.COLOR_RED}[y/N]{color.COLOR_RESET}: ")
                .strip()
                .lower()
                or "n"
            )
        return confirm == "y"
    except KeyboardInterrupt:
        print(f"\n{color.COLOR_YELLOW}操作已取消{color.COLOR_RESET}")
        sys.exit(1)
    except EOFError:  # 处理管道输入等情况
        print(f"\n{color.COLOR_YELLOW}输入结束，操作已取消{color.COLOR_RESET}")
        sys.exit(1)



def print_error_information_and_exit(
    func_name: str, error_str: str, isexit: bool = True
):
    """
    功能：
        打印错误信息并退出程序

    参数：
        func_name: 函数名
        error_str: 错误信息
        exit: 是否退出程序（默认退出）

    返回：
        None
    """
    print(
        f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:{func_name}]{color.COLOR_RESET} {error_str}",
        file=sys.stderr,
    )
    if isexit:
        sys.exit(1)



def args_normalize_path(path):
    """
    功能：
        纯字符串层面的路径规范化处理（不涉及实际文件系统）

    规则：
      - 统一分隔符为 `/`
      - 处理 `.` 和 `..`（仅字符串层面）
      - 不强制添加或删除结尾 `/`
      - 保留原始路径类型（绝对/相对）
      - Windows 盘符转为 `C:/` 格式

    参数：
        path: 待处理的路径字符串

    返回：
        处理后的路径字符串
    """

    path = str(path).strip()

    # 统一转换分隔符（\ → /）
    path = path.replace("\\", "/")

    # 记录是否以 / 结尾
    ends_with_slash = path.endswith("/")

    # 处理 Windows 盘符路径（如 C:\ → C:/）
    if re.match(r"^[A-Za-z]:", path):
        drive = path[0].upper()
        rest = path[2:].lstrip("/")
        path = f"{drive}:/{rest}" if rest else f"{drive}:/"
        return path  # Windows 路径不处理结尾 /

    # 使用 normpath 处理 . 和 ..（但会去掉结尾 /）
    path = os.path.normpath(path).replace("\\", "/")

    # 还原用户输入的结尾 /
    if ends_with_slash and path != "/":
        path += "/"

    return path



# 报错退出装饰器函数
def error_and_exit_handling_decorator(
    func_name: str, error_str: str, isexit: bool = True
):
    """
    功能：
        装饰器函数，用于处理函数执行时的异常

    参数：
        func_name: 函数名
        error_str: 错误信息
        isexit: 是否退出程序（默认退出）

    返回：
        装饰器函数
    """

    def decorator(func):
        def wrapper(*args, **kwargs):
            try:
                result = func(*args, **kwargs)
                return result
            except Exception as e:
                tlog.error(
                    f"{func_name}，{error_str}\n异常类型：\n{type(e)}\n异常信息：\n{e}"
                )
                print_error_information_and_exit(
                    f"{func_name}",
                    f"{error_str}\n异常类型：{type(e)}\n异常信息：\n{e}",
                    isexit,
                )

        return wrapper

    return decorator







def format_size(size_bytes: int) -> str:
    """自适应文件大小单位（B/KB/MB/GB/TB/PB），保留2位小数"""
    units = ["B", "KB", "MB", "GB", "TB", "PB"]
    size = float(abs(size_bytes))
    for i, unit in enumerate(units):
        if i == len(units) - 1 or size < 1024:
            return f"{size:.2f} {unit}"
        size /= 1024
    return f"{size:.2f} {unit}"


# 向后兼容：已迁移到 src.command.builder 模块
from src.command.builder import build_final_command, remove_command_fist_last_same_symbol
