# -*- coding: utf-8 -*-
# SSHFleet 路径处理模块
# 集中管理：路径规范化等纯字符串处理

import os
import re


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
