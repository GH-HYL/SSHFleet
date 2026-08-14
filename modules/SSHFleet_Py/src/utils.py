# -*- coding: utf-8 -*-
# SSHFleet 工具文件（兼容层）
# 该文件为历史兼容转发层：按职责拆分后的函数统一从这里再导出，
# 避免 11 个调用方逐个改 import。新代码请直接引用拆分后的模块：
#   - 错误处理/退出：src.error_handler
#   - 交互确认：src.interaction
#   - 路径处理：src.path_utils
#   - Excel 清洗/大小格式化：src.text_utils

from src.error_handler import error_and_exit_handling_decorator, print_error_information_and_exit
from src.interaction import get_user_confirmation
from src.path_utils import args_normalize_path
from src.text_utils import clean_for_excel, format_size

__all__ = [
    "clean_for_excel",
    "get_user_confirmation",
    "print_error_information_and_exit",
    "args_normalize_path",
    "error_and_exit_handling_decorator",
    "format_size",
]
