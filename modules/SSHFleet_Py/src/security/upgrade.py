# -*- coding: utf-8 -*-
# SSHFleet 凭据转换处理
# 职责：--convert-password 入口，按密码安全等级把凭据文件就地转换为对应格式
#   medium 等级：明文 → base64 转码
#   high 等级：明文/base64 → 加密（需主密钥）
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
        解析 --convert-password 传入的文件路径：先按输入原样找，找不到再按 secret_dir 相对路径找

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
        "handle_convert_password",
        f"凭据文件不存在：{raw}（已尝试原样路径与 secret_dir 相对路径）",
    )


def handle_convert_password(raw_path: str, secret_dir: str, level: str) -> None:
    """
    功能：
        --convert-password 入口：按密码安全等级把凭据文件就地转换为对应格式，完成后从磁盘回读内容输出确认

    参数：
        raw_path: 用户输入的凭据文件路径
        secret_dir: 配置的凭据目录（相对路径回退拼接用）
        level: 密码安全等级（medium=base64 转码；high=加密），来自配置 account.password_security
    """
    path = _resolve_cred_path(raw_path, secret_dir)
    content = _read_cred_content(path)

    if level == "high":
        _convert_to_encrypted(path, content)
    elif level == "medium":
        _convert_to_base64(path, content)
    else:
        print_error_information_and_exit(
            "handle_convert_password",
            f"不支持的密码安全等级：{level}（仅支持 medium / high）",
        )


def _read_cred_content(path: str) -> str:
    """读取凭据文件内容并判空，空文件直接报错退出"""
    try:
        with open(path, "r", encoding="utf-8") as f:
            content = f.read().strip()
    except Exception as e:
        print_error_information_and_exit(
            "handle_convert_password", f"读取凭据文件失败：{path}\n异常信息：{e}"
        )
    if not content:
        print_error_information_and_exit(
            "handle_convert_password", f"凭据文件内容为空：{path}"
        )
    return content


def _convert_to_base64(path: str, content: str) -> None:
    """medium 分支：明文 → base64 转码；已转码/已是加密格式时提示并跳过"""
    if looks_encrypted(content):
        print_error_information_and_exit(
            "handle_convert_password",
            f"文件已是加密格式，与当前 medium 密码等级不匹配，不做降级转换：{path}\n"
            f"如需处理该文件，请先将配置 account.password_security 改为 high 再操作",
        )
    if is_probably_base64_text(content):
        print(f"该文件已是 base64 编码，无需转码：{path}")
        return
    token = base64.b64encode(content.encode("utf-8")).decode("ascii")
    _write_and_echo(path, token, source_format="明文", action_desc="已将明文转码为 base64")


def _convert_to_encrypted(path: str, content: str) -> None:
    """high 分支：明文/base64 → 加密；已加密时提示并跳过（先判已加密，无需主密钥）"""
    if looks_encrypted(content):
        print(f"原文件已经是加密文件，无需重复加密：{path}")
        return

    master_key = get_master_key_or_exit("handle_convert_password")
    if is_probably_base64_text(content):
        try:
            plaintext = base64.b64decode(content).decode("utf-8")
        except Exception as e:
            print_error_information_and_exit(
                "handle_convert_password", f"base64 解码失败：{path}\n异常信息：{e}"
            )
        source_format = "base64"
    else:
        plaintext = content
        source_format = "明文"

    if not plaintext:
        print_error_information_and_exit(
            "handle_convert_password",
            f"{source_format} 解码后内容为空，拒绝加密空凭据：{path}",
        )

    token = encrypt_password(plaintext, master_key)
    _write_and_echo(path, token, source_format=source_format, action_desc="已完成加密")


def _write_and_echo(path: str, new_content: str, source_format: str, action_desc: str) -> None:
    """就地覆盖写入，再从磁盘回读输出（而非打印内存变量），让用户以文件内容为准确认转换成功"""
    try:
        with open(path, "w", encoding="utf-8") as f:
            f.write(new_content)
    except Exception as e:
        print_error_information_and_exit(
            "handle_convert_password", f"写入转换内容失败：{path}\n异常信息：{e}"
        )
    try:
        with open(path, "r", encoding="utf-8") as f:
            on_disk = f.read().strip()
    except Exception as e:
        print_error_information_and_exit(
            "handle_convert_password", f"回读转换后文件失败：{path}\n异常信息：{e}"
        )

    print(f"识别为{source_format}格式，{action_desc}：{path}")
    print("转换后文件内容（磁盘回读）：")
    print(on_disk)
