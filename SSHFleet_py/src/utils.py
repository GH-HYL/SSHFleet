# -*- coding: utf-8 -*-
# SSHFleet 工具文件
# 该文件负责定义通用的工具函数，包括参数解析、配置加载、日志初始化等

import argparse

# 系统或第三方模块
import os
import re
import shlex
import sys
from typing import Dict, List
from datetime import datetime
from posixpath import join as posix_join

from loguru import logger

# 自定义模块
import src.color as color
from src.yaml import SSHFleetConfig

# 初始化全局logger变量
tlog = logger.bind(logger_type="tool")
elog = logger.bind(logger_type="exec")


class JumpOut(Exception):
    """自定义异常，用于跳出try，中断执行"""

    pass

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


    import re

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


def remove_command_fist_last_same_symbol(cmd_str):
    """
    功能：
        去除 command 命令 首尾相同的特殊符号

    参数：
        cmd_str: 命令字符串

    返回：
        removed_symbol: 被移除的特殊符号
        cmd_str: 处理后的命令字符串
    """

    # 特殊符号黑名单，以下符号不移除
    forbidden_chars = r"^$*+?.()[]{}|\/"

    # 判断并处理 , 命令大于一个字符、首尾相同、首尾不是字母或数字、首尾不在特殊符号黑名单中
    if (
        len(cmd_str) > 1
        and cmd_str[0] == cmd_str[-1]
        and not cmd_str[0].isalnum()
        and cmd_str[0] not in forbidden_chars
    ):

        removed_symbol = cmd_str[0]  # 记录被移除的符号
        cmd_str = cmd_str[1:-1]  # 实际移除操作
        return removed_symbol, cmd_str
    else:
        return None, cmd_str


def error_classify(
    ip: str, error_text: str, error_keywords: Dict[str, List[str]]
) -> str:
    """
    功能：
        根据错误文本内容进行分类

    参数：
        ip: 设备IP
        error_text: 错误文本内容
        error_keywords: 错误分类字典，用于根据错误文本分类错误

    返回：
        错误类型分类
    """

    # 转换为小写进行匹配
    error_lower = error_text.lower()
    elog.info(f"{ip}：错误类型原始文本: {error_text}")
    # 注意：这里的关键词是英文，字典关键词全为小写
    for category, keywords in error_keywords.items():
        # 获取单个中文的多个关键词
        for keyword in keywords:
            # 检查关键词是否在错误文本中
            if keyword in error_lower:
                return category

    elog.error(f"{ip}：未匹配到错误类型")
    return "错误未分类"


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
    sys.exit(1) if isexit else None


def init_tool_logger(log_dir: str, config: SSHFleetConfig):
    global tlog

    os.makedirs(log_dir, exist_ok=True)

    # 移除默认handler
    logger.remove()

    # 添加tool日志handler
    tlog.add(
        os.path.join(log_dir, config.paths.logs.tool),
        rotation="50 MB",
        level="DEBUG",
        encoding="utf-8",
        format="{time:YYYY-MM-DD HH:mm:ss.SSS} - [{level: ^7}] - {message}",
        filter=lambda record: record["extra"].get("logger_type") == "tool",
    )

    return tlog


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


def build_final_command(args: argparse.Namespace) -> str:
    """
    根据参数构建命令字符串，添加环境变量和sudo权限

    Args:
        args: 参数字典，包含：
            - c: 命令字符串
            - m: 模式，检测到'sudo'时添加sudo
            - e: 环境变量字符串

    Returns:
        str: 组合后的完整命令字符串
    """

    # 初始化组件,设置输出编码方式（C.UTF-8是POSIX标准，所有Linux发行版内置支持）
    components = ["LC_ALL=C.UTF-8 LANG=C.UTF-8;"]

    # 1. 环境变量
    # if args.e:
    #     components.append(args.e)

    # 2. sudo
    if args.m == "sudo":
        components.append("sudo")

    # 3. 命令主体
    if args.c:
        safe_command = shlex.quote(args.c)
        if not args.nobash:
            components.append(f"bash -c {safe_command}")
        else:
            components.append(args.c)
    elif args.s:
        import base64

        # 脚本解释器选择
        interpreter = "python3" if args.s.endswith(".py") else "bash"

        # 读取脚本内容并正确使用heredoc
        with open(args.s, "r", encoding="utf-8") as f:
            script_content = f.read().strip()

        # 编码为base64
        encoded_content = base64.b64encode(script_content.encode("utf-8")).decode(
            "utf-8"
        )

        # 构建命令：使用printf输出base64字符串，然后解码并执行
        # base64编码字符集安全，不包含引号，因此使用单引号包裹
        # printf '%s' 意思是原样输出字符串，不进行转义，确保内容完整传递
        if interpreter == "bash":
            command = f"printf '%s' '{encoded_content}' | base64 -d | {'sudo ' if args.m == 'sudo' else ''}bash"
        else:  # python3
            command = f"printf '%s' '{encoded_content}' | base64 -d | {'sudo ' if args.m == 'sudo' else ''}python3"

        components.append(command)

    else:
        tlog.error("参数出现严重异常，args.c 和 args.s 不能同时为空")
        print_error_information_and_exit(
            "add_env_sudo_to_commands",
            "参数出现严重异常，args.c 和 args.s 不能同时为空",
        )

    # 组合完整命令
    final_command = " ".join(filter(None, components))

    tlog.success(f"完整命令拼接完成: {final_command}")
    return final_command.strip()


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


@error_and_exit_handling_decorator("create_exec_log_dir", "创建日志目录失败")
def create_exec_log_dir(args, config) -> str:
    """
    功能：
        创建日志目录

    参数：
        args: 命令行参数
        config: 配置对象

    返回：
        日志目录路径
    """

    # 使用可读的日期时间格式，而不是时间戳
    timestamp = datetime.now().strftime("%Y-%m-%d_%H-%M-%S")

    if args.c:
        file_name = "command"
    elif args.s:
        file_name = "script"
    elif args.u:
        file_name = "upload"
    elif args.d:
        file_name = "download"
    else:
        file_name = "unknown"  # 添加默认值避免未定义

    # 最后拼接成的大概路径样子是 history/2025-08-20_12-12-12_command/
    log_dir = posix_join(config.paths.logs.historys, f"{timestamp}_{file_name}")

    if args.r:
        log_dir = log_dir + f"_{args.r.strip()}"

    # 生成日志目录（创建完整路径）
    os.makedirs(log_dir, exist_ok=True)

    return log_dir


@error_and_exit_handling_decorator("init_execution_logger", "初始化执行日志记录器失败")
def init_execution_logger(log_dir: str, log_exec: str):
    global elog

    os.makedirs(log_dir, exist_ok=True)

    # 添加execution日志handler
    elog.add(
        os.path.join(log_dir, log_exec),
        level="DEBUG",
        enqueue=True,  # 启用线程安全队列
        encoding="utf-8",
        format="{time:YYYY-MM-DD HH:mm:ss.SSS} - [{level: ^7}] - {message}",
        filter=lambda record: record["extra"].get("logger_type") == "exec",
    )
    return elog


@error_and_exit_handling_decorator(
    "create_latest_log_symlink", "创建最新日志符号链接失败", isexit=True
)
def create_latest_log_symlink(config: SSHFleetConfig):
    """
    功能：
        创建最新日志符号链接

    参数：
        None

    返回：
        None
    """

    # 检查一下当前系统环境
    if os.name != "posix":
        tlog.warning("当前系统环境不是POSIX兼容系统，无法创建符号链接")
        return

    if not os.path.isdir(config.paths.logs.historys):
        print("错误: 历史记录目录 (historys) 不存在")
        tlog.error("历史记录目录 (historys) 不存在")
        return

    # 获取historys目录下所有子目录，并按创建时间倒序排序
    log_dirs = []
    for entry in os.scandir(config.paths.logs.historys):
        if entry.is_dir():
            log_dirs.append(entry)
    # 按创建时间排序，最新在前
    log_dirs.sort(key=lambda x: x.stat().st_ctime, reverse=True)

    if not log_dirs:
        print(
            f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:zip_latest_history]{color.COLOR_RESET} 历史记录目录 '{config.paths.logs.historys}' 中没有日志文件夹",
            file=sys.stderr,
        )
        print("提示: 请先至少一次执行任务以生成历史记录")
        tlog.error("历史记录目录 (historys) 中没有日志文件夹")
        return

    latest_log_dir = posix_join(config.paths.logs.historys, log_dirs[0].name)
    latest_link = "latest_history"

    try:
        if os.path.islink(latest_link):
            os.remove(latest_link)
        elif os.path.exists(latest_link):
            print(f"警告: 已存在同名文件 {latest_link}，无法创建符号链接")
            tlog.warning(f"已存在同名文件 {latest_link}，无法创建符号链接")
            return
        os.symlink(latest_log_dir, latest_link)
        tlog.success(f"创建最新日志符号链接函数执行成功，指向路径: {latest_log_dir}")
    except OSError as e:
        print(f"创建符号链接失败: {str(e)}")
        tlog.error(
            f"创建最新日志符号链接函数执行失败\n异常类型：\n{type(e)}\n异常信息：\n{e}"
        )
        if e.errno == 1:
            print("提示: 请尝试使用管理员/root权限运行")
            tlog.error("创建最新日志符号链接函数执行失败，权限不足")

    
