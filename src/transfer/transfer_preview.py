# -*- coding: utf-8 -*-
# SSHexec 传输文件优化版本
# 处理纯文本和二进制文件上传，生成创建命令和全文本标志位

import os
import base64

from src import utils

# 脚本内容样例
"""
#!/bin/sh
export LC_ALL=en_US.UTF-8 LANG=en_US.UTF-8
TARGET_DIR='/home/user/upload'
[ -f "$TARGET_DIR/example.txt" ] && { echo "重名: example.txt" >&2; exit 1; }
mkdir -p "$(dirname "$TARGET_DIR/example.txt")" && printf '%s' 'SGVsbG8gV29ybGQh' | base64 -d > "$TARGET_DIR/example.txt" || exit 2
"""

def check_if_all_text(u: str) -> list:
    """
    检查指定路径下的所有文件是否为纯文本，并收集文件和目录清单。

    参数:
        u: 本地文件或目录路径

    返回:
        list: 文件和目录清单。如果检测到二进制文件，返回空列表 []。
              清单元素为 (相对路径, 完整路径) 元组，对于目录，完整路径是目录路径。
    """
    file_list = []
    dir_list = []  # 用于存储目录路径

    if os.path.isfile(u) and not os.path.islink(u):
        # 检查单个文件
        with open(u, 'rb') as f:
            content = f.read()
        if b'\x00' in content:
            return []  # 发现二进制文件，返回空列表
        else:
            file_list.append(('', u))
            return file_list  # 单个文件没有目录
    elif os.path.isdir(u):
        # 递归检查目录中的所有文件和目录
        for root, dirs, filenames in os.walk(u):
            # 排除符号链接目录
            dirs[:] = [d for d in dirs if not os.path.islink(os.path.join(root, d))]
            # 添加当前目录（如果不是根目录 u）
            rel_root = os.path.relpath(root, u)
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
                    return []  # 发现二进制文件，返回空列表
                rel = os.path.relpath(full, u)
                # 将Windows路径分隔符转换为Linux路径分隔符
                rel = rel.replace('\\', '/')
                file_list.append((rel, full))
        # 合并文件和目录列表
        all_list = file_list + dir_list
        return all_list
    else:
        # 无效路径或符号链接,直接报错
        utils.print_error_informantion_and_exit(
            "transfer_preview", f" 错误: {u} 不是有效的文件或目录路径，或是一个符号链接"
        )
        # 防ide报错，实际上不会执行，上面的函数已经退出了
        return []


def generate_upload_command(file_list: list, u: str, p: str) -> str:
    """
    生成上传命令，基于文件和目录清单。

    参数:
        file_list: 文件和目录清单，元素为 (相对路径, 完整路径) 元组
        u: 本地文件或目录路径（用于当 rel 为空时获取 basename）
        p: 远程目标目录路径

    返回:
        str: 可直接在远程执行的命令字符串。如果 file_list 为空，返回空字符串。
    """
    # 退出码变量
    repetition_exit_code = 102  # 重名检查退出码
    creation_exit_code = 101  # 文件创建失败退出码

    if not file_list:
        return ""

    # 分离文件和目录条目
    file_entries = []
    dir_entries = []
    for rel, full in file_list:
        # 检查是否为目录（使用os.path.isdir，但full可能是Windows路径）
        # 我们使用rel是否以'/'结尾或检查原始路径
        if os.path.isdir(full) or (rel and not rel.endswith('.txt') and not rel.endswith('.csv') and not rel.endswith('.sh')):
            # 简单判断：如果rel中没有点号或者以'/'结尾，可能是目录
            # 但更可靠的是检查full是否为目录
            if os.path.isdir(full):
                dir_entries.append((rel, full))
            else:
                file_entries.append((rel, full))
        else:
            file_entries.append((rel, full))

    # 读取文件内容并 base64 编码（已确认纯文本）
    file_data = []
    for rel, full in file_entries:
        with open(full, 'rb') as f:
            content = f.read()
        b64 = base64.b64encode(content).decode('ascii')
        file_data.append((rel, b64))

    # 转义目标目录路径中的单引号
    p_safe = p.replace("'", "'\\''")

    # 构建脚本内容
    lines = ["#!/bin/sh"]
    # 配置语言环境，确保远程执行环境正确处理文本
    lines.append("export LC_ALL=en_US.UTF-8 LANG=en_US.UTF-8")
    # 插入目标目录变量定义（放在重名检查之前）
    lines.insert(1, f"TARGET_DIR='{p_safe}'")

    # 先创建所有目录（包括空文件夹）
    # 按路径深度排序，确保父目录先创建
    dir_entries.sort(key=lambda x: x[0].count('/'))
    for rel, _ in dir_entries:
        if rel:
            dest_path = f"$TARGET_DIR/{rel}"
        else:
            continue  # 跳过根目录
        lines.append(f"mkdir -p \"{dest_path}\"")

    # 检查重名：收集所有重名文件，然后一起处理（只针对文件）
    lines.append("repetition_found=0")
    for rel, _ in file_data:
        # 确定目标路径和文件名
        if rel:
            dest_path = f"$TARGET_DIR/{rel}"
            file_name = rel
        else:
            dest_path = f"$TARGET_DIR/{os.path.basename(u)}"
            file_name = os.path.basename(u)
        # 转义文件名用于嵌入到双引号中（在 sh 脚本中）
        file_name_escaped = file_name.replace('"', '\\"').replace('\\', '\\\\')
        # 生成检查命令：如果文件存在，打印重名消息并设置标志
        lines.append(
            f'if [ -f "{dest_path}" ]; then '
            f"printf '上传文件出现重名: %s\\n' \"{file_name_escaped}\" >&2; "
            f'repetition_found=1; '
            f'fi'
        )
    # 如果有重名，退出
    lines.append(
        f'if [ $repetition_found -eq 1 ]; then '
        f'exit {repetition_exit_code}; '
        f'fi'
    )

    # 文件创建
    for rel, b64 in file_data:
        if rel:
            dest_path = f"$TARGET_DIR/{rel}"
        else:
            dest_path = f"$TARGET_DIR/{os.path.basename(u)}"
        dir_part = f'"$(dirname "{dest_path}")"'
        lines.append(
            f"mkdir -p {dir_part} && "
            f"printf '%s' '{b64}' | base64 -d > \"{dest_path}\" || exit {creation_exit_code}"
        )
    # 使用单引号包裹完成消息
    lines.append("echo '上传文件完成'")
    script = "\n".join(lines)
    # 将整个脚本 base64 编码
    script_b64 = base64.b64encode(script.encode('utf-8')).decode('ascii')

    # 最终命令
    command = f"printf '%s' '{script_b64}' | base64 -d | sh"
    return command



def transfer_preview(u: str, p: str) -> str:
    """
    入口函数：检查上传文件是否为纯文本，并生成相应命令。

    参数:
        u: 本地文件或目录路径
        p: 远程目标目录路径（一定是目录）

    返回:
        拼装好的命令，如是空值，表示不是纯文本文件
    """
    file_list = check_if_all_text(u)
    print(file_list)
    if file_list:  # 非空列表表示全是文本
        command = generate_upload_command(file_list, u, p)
        return command
    else:
        return ""
