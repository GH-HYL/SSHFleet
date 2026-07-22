# -*- coding: utf-8 -*-
# SSHFleet 文件检查模块

import os
from posixpath import join as posix_join

from src.yaml import SSHFleetConfig
import src.utils as utils


@utils.error_and_exit_handling_decorator("check_files_exist", "检查代码文件存在性失败")
def check_files_exist(config: SSHFleetConfig) -> None:
    """检查所有代码文件必须存在"""
    current_dir = os.getcwd()

    code_files = [
        "src/config/SSHFleet.yaml",
        config.paths.jsons.dangerous_keywords,
        config.paths.jsons.error_keywords,
    ]

    # 循环检查文件，然后把所有缺失文件打印出来
    missing_files = []
    for code_file in code_files:
        if not os.path.exists(posix_join(current_dir, code_file)):
            missing_files.append(code_file)

    if missing_files:
        utils.print_error_information_and_exit(
            "check_files_exist", f" 代码文件缺失: {', '.join(missing_files)}"
        )


def check_script_file(script_path):
    """检查脚本文件内容的有效性"""

    # 脚本必须使用UTF-8编码（支持带BOM）
    with open(script_path, "rb") as f:
        raw_data = f.read()

        # 不能是二进制文件
        if b"\x00" in raw_data:
            utils.print_error_information_and_exit(
                "check_script_file", f" 错误: {script_path} 是二进制文件"
            )

        if raw_data.startswith(b"\xef\xbb\xbf"):
            # 有BOM，去掉BOM
            data = raw_data[3:]
        else:
            data = raw_data
        if not data.decode("utf-8", errors="ignore").encode("utf-8") == data:
            utils.print_error_information_and_exit(
                "check_script_file", f" 错误: {script_path} 不是UTF-8编码"
            )

        # 必须使用LF换行符(Unix格式)，如果不是，直接替换成LF并提示用户
        if b"\r\n" in data:
            with open(script_path, "wb") as f:
                f.write(data.replace(b"\r\n", b"\n"))
            from src.utils import tlog as _tlog
            _tlog.warning(f"{script_path} 的换行符已自动转换为LF格式")
