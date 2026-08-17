# -*- coding: utf-8 -*-
# SSHFleet 交互确认模块
# 集中管理：用户确认、输入取消处理

import sys

import src.common.constants as color


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
