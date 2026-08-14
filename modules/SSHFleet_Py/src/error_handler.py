# -*- coding: utf-8 -*-
# SSHFleet 错误处理与退出约定模块
# 集中管理：错误信息打印、程序退出、函数级异常装饰器

import sys

import src.color as color
from src.log import tlog


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
