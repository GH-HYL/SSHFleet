# -*- coding: utf-8 -*-
# SSHFleet 传输预检查文件
# 处理纯文本和二进制文件上传，生成创建命令和全文本标志位

import os
import base64
import stat

from src import utils
from src.utils import tlog

# 命令长度安全上限（保守值，SSH命令通常限制在~1MB，预留余量）
MAX_COMMAND_SIZE = 500 * 1024


def check_if_all_text(upload_path: str) -> list:
    """
    检查指定路径下的所有文件是否为纯文本，并收集文件和目录清单。

    参数:
        upload_path: 本地文件或目录路径

    返回:
        list: 文件和目录清单。如果检测到二进制文件，返回空列表 []。
              清单元素为 (相对路径, 完整路径) 元组，对于目录，完整路径是目录路径。
    """
    file_list = []
    dir_list = []  # 用于存储目录路径

    if os.path.isfile(upload_path) and not os.path.islink(upload_path):
        # 检查单个文件
        with open(upload_path, 'rb') as f:
            content = f.read()
        if b'\x00' in content:
            tlog.error(f"检测到上传目标{upload_path}不是纯文本二进制文件，返回空列表file_list")
            return []  # 发现二进制文件，返回空列表
        else:
            file_list.append(('', upload_path))
            return file_list  # 单个文件没有目录
    elif os.path.isdir(upload_path):
        # 递归检查目录中的所有文件和目录
        for root, dirs, filenames in os.walk(upload_path):
            # 排除符号链接目录
            dirs[:] = [d for d in dirs if not os.path.islink(os.path.join(root, d))]
            # 添加当前目录（如果不是根目录 upload_path）
            rel_root = os.path.relpath(root, upload_path)
            # 将Windows路径分隔符转换为Linux路径分隔符
            rel_root = rel_root.replace('\\', '/')
            if rel_root != ".":
                dir_list.append((rel_root, root))
            for fname in filenames:
                full = os.path.join(root, fname)
                if os.path.islink(full):
                    continue
                with open(full, 'rb') as f:
                    content = f.read()
                if b'\x00' in content:
                    tlog.error(f"检测到上传目标{full}不是纯文本二进制文件，返回空列表file_list")
                    return []  # 发现二进制文件，返回空列表
                rel = os.path.relpath(full, upload_path)
                # 将Windows路径分隔符转换为Linux路径分隔符
                rel = rel.replace('\\', '/')
                file_list.append((rel, full))
        # 合并文件和目录列表
        all_list = file_list + dir_list
        return all_list
    else:
        # 无效路径或符号链接,直接报错
        utils.print_error_information_and_exit(
            "transfer_precheck", f" 错误: {upload_path} 不是有效的文件或目录路径，或是一个符号链接"
        )


def _shell_escape(s: str) -> str:
    """
    转义字符串用于shell单引号内使用。
    单引号内不能出现单引号，所以用 '替换为'\'' 的方式。
    """
    return s.replace("'", "'\\''")


def _get_file_mode(path: str) -> int:
    """获取文件的权限模式（八进制）"""
    try:
        return stat.S_IMODE(os.stat(path).st_mode)
    except OSError:
        return 0o644  # 默认权限


def generate_upload_command(file_list: list, upload_path: str, path: str) -> str:
    """
    生成上传命令，基于文件和目录清单。

    参数:
        file_list: 文件和目录清单，元素为 (相对路径, 完整路径) 元组
        upload_path: 本地文件或目录路径（用于当 rel 为空时获取 basename）
        path: 远程目标目录路径

    返回:
        str: 可直接在远程执行的命令字符串。如果 file_list 为空，返回空字符串。
    """
    # 退出码变量
    repetition_exit_code = 102  # 重名检查退出码
    creation_exit_code = 101  # 文件创建失败退出码
    tlog.debug(f"本次使用的退出码: 重名-{repetition_exit_code}, 文件创建失败-{creation_exit_code}")

    if not file_list:
        return ""

    # 分离文件和目录条目（仅依赖os.path.isdir，不依赖扩展名）
    file_entries = []
    dir_entries = []
    for rel, full in file_list:
        if os.path.isdir(full):
            dir_entries.append((rel, full))
        else:
            file_entries.append((rel, full))

    # 读取文件内容并 base64 编码，同时收集权限信息
    file_data = []
    total_b64_size = 0
    for rel, full in file_entries:
        with open(full, 'rb') as f:
            content = f.read()
        b64 = base64.b64encode(content).decode('ascii')
        mode = _get_file_mode(full)
        file_data.append((rel, b64, mode))
        total_b64_size += len(b64)

    # 检查命令总长度是否超限
    if total_b64_size > MAX_COMMAND_SIZE:
        tlog.warning(f"文件base64编码后总大小{total_b64_size}超过安全上限{MAX_COMMAND_SIZE}，切换为SFTP模式")
        return ""

    # 转义目标目录路径
    path_safe = _shell_escape(path)

    # 构建脚本内容
    lines = ["#!/bin/sh"]
    lines.append("export LC_ALL=C.UTF-8 LANG=C.UTF-8")
    lines.append(f"TARGET_DIR='{path_safe}'")

    # 检查重名
    lines.append("repetition_mark=0")

    # 检查文件重名
    for rel, _, _ in file_data:
        if rel:
            dest_path = f"$TARGET_DIR/{rel}"
            file_name = rel
        else:
            dest_path = f"$TARGET_DIR/{os.path.basename(upload_path)}"
            file_name = os.path.basename(upload_path)
        file_name_escaped = _shell_escape(file_name)
        lines.append(
            f'if [ -f "{dest_path}" ]; then '
            f"printf '上传文件出现重名: %s\\n' '{file_name_escaped}' >&2; "
            f'repetition_mark=1; '
            f'fi'
        )

    # 检查目录重名
    for rel, _ in dir_entries:
        if rel:
            dest_path = f"$TARGET_DIR/{rel}"
            dir_name = rel
            dir_name_escaped = _shell_escape(dir_name)
            lines.append(
                f'if [ -d "{dest_path}" ]; then '
                f"printf '上传目录出现重名: %s\\n' '{dir_name_escaped}' >&2; "
                f'repetition_mark=1; '
                f'fi'
            )

    # 如果有重名，退出
    lines.append(
        f'if [ $repetition_mark -eq 1 ]; then '
        f'exit {repetition_exit_code}; '
        f'fi'
    )

    # set -e：任何命令失败则退出
    lines.append("set -e")

    # 先创建所有目录，按路径深度排序确保父目录先创建
    dir_entries.sort(key=lambda x: x[0].count('/'))
    for rel, _ in dir_entries:
        if rel:
            dest_path = f"$TARGET_DIR/{rel}"
            lines.append(f"mkdir -p \"{dest_path}\"")

    # 文件创建，保留原始权限
    for rel, b64, mode in file_data:
        if rel:
            dest_path = f"$TARGET_DIR/{rel}"
        else:
            dest_path = f"$TARGET_DIR/{os.path.basename(upload_path)}"
        # 写入文件
        lines.append(
            f"printf '%s' '{b64}' | base64 -d > \"{dest_path}\""
        )
        # 恢复权限（八进制）
        if mode and mode != 0o644:
            lines.append(f"chmod {oct(mode)[2:]} \"{dest_path}\"")

    lines.append("echo '上传文件完成'")
    script = "\n".join(lines)
    tlog.debug(f"上传脚本拼接完成，脚本长度: {len(script)} 字符")

    # 将整个脚本 base64 编码
    script_b64 = base64.b64encode(script.encode('utf-8')).decode('ascii')
    tlog.debug(f"上传脚本base64编码完成，编码后长度: {len(script_b64)} 字符")

    # 最终命令
    command = f"printf '%s' '{script_b64}' | base64 -d | sh"
    return command


def transfer_precheck(upload_path: str, path: str) -> str:
    """
    入口函数：检查上传文件是否为纯文本，并生成相应命令。

    参数:
        upload_path: 本地文件或目录路径
        path: 远程目标目录路径（一定是目录）

    返回:
        拼装好的命令，如是空值，表示不是纯文本文件或超出命令长度限制
    """
    tlog.info(f"开始检查上传目标是否为纯文本文件")
    file_list = check_if_all_text(upload_path)
    if file_list:  # 非空列表表示全是文本
        tlog.info(f"检测到上传目标是纯文本文件，生成上传命令")
        tlog.debug(f"上传目标文件列表: {file_list}")
        command = generate_upload_command(file_list, upload_path, path)
        return command
    else:
        tlog.error(f"检测到上传目标不是纯文本文件，无上传命令")
        return ""
