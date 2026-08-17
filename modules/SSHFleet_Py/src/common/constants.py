# -*- coding: utf-8 -*-
# SSHFleet 公共常量模块

# 成功分类名（classifier 与 statistics 共用，改动只此一处）
SUCCESS_CATEGORY_EXECUTE = "执行成功"
SUCCESS_CATEGORY_TRANSPORT = "传输成功"

# 颜色常量（原 src/color.py 并入，用于终端输出着色，谨慎修改）
COLOR_GREEN = "\033[32m"  # 绿色
COLOR_RED = "\033[31m"  # 红色
COLOR_CYAN = "\033[36m"  # 青色
COLOR_YELLOW = "\033[33m"  # 黄色
COLOR_BLUE = "\033[34m"  # 蓝色
COLOR_MAGENTA = "\033[35m"  # 品红色
COLOR_WHITE = "\033[37m"  # 白色
COLOR_RESET = "\033[0m"  # 重置颜色
COLOR_BOLD = "\033[1m"  # 加粗
COLOR_BRIGHT_YELLOW = "\033[93m"  # 亮黄（黑底终端上比 33 更醒目）
COLOR_BRIGHT_CYAN = "\033[96m"  # 亮青（黑底终端上比 36 更醒目）
COLOR_BRIGHT_ORANGE = "\033[38;5;214m"  # 亮橙（256 色，黑底终端上醒目）
