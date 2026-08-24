# -*- coding: utf-8 -*-
# SSHFleet 凭据升级处理
# 职责：--encrypt-password 入口，把旧格式（明文/base64）凭据文件就地加密升级
# 决策背景见 docs/adr/0007-homemade-cipher-env-key.md

import base64
import os

from src.common.error_handler import print_error_information_and_exit
from src.security.cipher import (
    encrypt_password,
    is_probably_base64_text,
    looks_encrypted,
)
from src.security.master_key import get_master_key_or_exit


def _resolve_cred_path(raw: str, secret_dir: str) -> str:
    """
    功能：
        解析 --encrypt-password 传入的文件路径：先按输入原样找，找不到再按 secret_dir 相对路径找

    参数：
        raw: 用户输入的路径
        secret_dir: 配置的凭据目录

    返回：
        str: 绝对路径

    Raises:
        SystemExit: 两处都找不到对应文件
    """
    expanded = os.path.expanduser(raw.strip())
    if os.path.exists(expanded):
        return expanded
    if not os.path.isabs(expanded) and secret_dir:
        joined = os.path.join(secret_dir, expanded)
        if os.path.exists(joined):
            return joined
    print_error_information_and_exit(
        "handle_encrypt_password",
        f"凭据文件不存在：{raw}（已尝试原样路径与 secret_dir 相对路径）",
    )


def handle_encrypt_password(raw_path: str, secret_dir: str) -> None:
    """
    功能：
        --encrypt-password 入口：识别旧格式并就地加密升级，完成后从磁盘回读内容输出确认

    参数：
        raw_path: 用户输入的凭据文件路径
        secret_dir: 配置的凭据目录（相对路径回退拼接用）
    """
    master_key = get_master_key_or_exit("handle_encrypt_password")
    path = _resolve_cred_path(raw_path, secret_dir)

    try:
        with open(path, "r", encoding="utf-8") as f:
            content = f.read().strip()
    except Exception as e:
        print_error_information_and_exit(
            "handle_encrypt_password", f"读取凭据文件失败：{path}\n异常信息：{e}"
        )
    if not content:
        print_error_information_and_exit(
            "handle_encrypt_password", f"凭据文件内容为空：{path}"
        )

    if looks_encrypted(content):
        print(f"该文件已是加密格式，无需升级：{path}")
        return

    if is_probably_base64_text(content):
        try:
            plaintext = base64.b64decode(content).decode("utf-8")
        except Exception as e:
            print_error_information_and_exit(
                "handle_encrypt_password", f"base64 解码失败：{path}\n异常信息：{e}"
            )
        source_format = "base64"
    else:
        plaintext = content
        source_format = "明文"

    if not plaintext:
        print_error_information_and_exit(
            "handle_encrypt_password",
            f"{source_format} 解码后内容为空，拒绝加密空凭据：{path}",
        )

    token = encrypt_password(plaintext, master_key)
    try:
        with open(path, "w", encoding="utf-8") as f:
            f.write(token)
    except Exception as e:
        print_error_information_and_exit(
            "handle_encrypt_password", f"写入加密内容失败：{path}\n异常信息：{e}"
        )

    # 从磁盘重新读回输出（而非打印内存变量），让用户直接以文件内容为准确认升级成功
    try:
        with open(path, "r", encoding="utf-8") as f:
            on_disk = f.read().strip()
    except Exception as e:
        print_error_information_and_exit(
            "handle_encrypt_password", f"回读加密文件失败：{path}\n异常信息：{e}"
        )

    print(f"识别为{source_format}格式，已完成加密升级：{path}")
    print("升级后文件内容（磁盘回读）：")
    print(on_disk)
