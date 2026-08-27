# -*- coding: utf-8 -*-
# SSHFleet 交互模块
# 集中管理：用户确认、文本输入（prompt_text）、取消处理

import sys

import src.common.constants as color


def prompt_text(prompt, validator=None, sensitive=False):
    """统一交互输入助手：提示 → 校验 → 取消处理（KeyboardInterrupt/EOFError 统一优雅退出）

    Args:
        prompt: 提示文本
        validator: 校验回调，接收原始输入返回 (value, error_msg)；
                   error_msg 非空则打印后重试；None 表示接受任意输入
        sensitive: 敏感输入（密码类），不回显（延迟导入 getpass）

    Returns:
        value: 校验通过的值

    Raises:
        SystemExit: 用户取消（Ctrl+C）或输入流结束（EOF/管道关闭）
    """
    while True:
        try:
            if sensitive:
                import getpass
                raw = getpass.getpass(prompt)
            else:
                raw = input(prompt)
        except (KeyboardInterrupt, EOFError):  # EOFError：管道/重定向输入结束时同样优雅退出
            print("\n用户取消输入")
            sys.exit(1)
        if validator is None:
            return raw
        value, error_msg = validator(raw)
        if error_msg:
            print(error_msg)
            continue
        return value


def get_user_confirmation(prompt, yorn=False, disinteractive=False):
    """
    功能：
        获取用户确认的通用函数

    参数：
        prompt: 确认提示信息
        yorn: 是否为 yes/no 确认，默认 False
        disinteractive: 跳过确认模式，yorn=True时自动确认，yorn=False时自动拒绝

    返回：
        confirm: 用户确认结果，True 或 False
    """

    # 非交互模式：自动跳过确认
    if disinteractive:
        return yorn  # yorn=True 自动确认，yorn=False 自动拒绝

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
