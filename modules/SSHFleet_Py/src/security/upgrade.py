# -*- coding: utf-8 -*-
# SSHFleet 凭据转换处理
# 职责：--convert-password 入口。自适应识别文件内容格式（明文/base64/加密），
#       统一还原为明文后，按目标等级（配置 account.password_security）重新编码写回。
#       支持升级（明文→base64→加密）与降级（加密→base64→明文，降级需主密钥）。
# 决策背景见 docs/adr/0007-homemade-cipher-env-key.md

import base64
import os

from src.common.error_handler import print_error_information_and_exit
from src.security.cipher import (
    CipherError,
    classify_credential,
    decrypt_password,
    encrypt_password,
)
from src.security.credential import read_cred_file_content
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
        f"找不到凭据文件：{raw}，请检查路径是否正确",
    )


def handle_convert_password(raw_path: str, secret_dir: str, level: str) -> None:
    """
    功能：
        --convert-password 入口：识别文件内容格式（明文/base64/加密），统一还原为明文后，
        按目标等级（配置 account.password_security）重新编码写回，完成后从磁盘回读内容输出确认。

    参数：
        raw_path: 用户输入的凭据文件路径
        secret_dir: 配置的凭据目录（相对路径回退拼接用）
        level: 目标密码安全等级（1=明文 / 2=base64 / 3=加密）

    转换矩阵：
        输入\\目标   1（明文）    2（base64）    3（加密）
        明文        无需转换     转 base64      加密
        base64      解码为明文   已是 base64    解码后加密
        加密        解密为明文   解密为 base64  已是加密

    防御：
        - 加密内容转换需要主密钥，缺失时提示生成方式
        - 解密失败（密钥不一致 / 文件损坏）明确提示
        - 目标为加密同样需要主密钥；空凭据拒绝转换
    """
    path = _resolve_cred_path(raw_path, secret_dir)
    content = _read_cred_content(path)
    if level not in ("1", "2", "3"):
        print_error_information_and_exit(
            "handle_convert_password",
            f"不支持的密码安全等级：{level}（支持 1=明文 / 2=base64 / 3=加密）",
        )

    fmt = classify_credential(content)

    # ① 统一还原为明文（加密格式需主密钥；解密失败=密钥不一致/文件损坏，明确提示）
    if fmt == "encrypted":
        master_key = get_master_key_or_exit("handle_convert_password")
        try:
            plaintext = decrypt_password(content, master_key)
        except CipherError as e:
            print_error_information_and_exit(
                "handle_convert_password",
                f"解密失败：主密钥与加密该文件时使用的密钥不一致，"
                f"或文件已损坏/被修改：{path}\n原因：{e}",
            )
        source_format = "加密"
    elif fmt == "base64":
        try:
            plaintext = base64.b64decode(content).decode("utf-8")
        except Exception as e:
            print_error_information_and_exit(
                "handle_convert_password", f"base64 内容无法解码，文件可能已损坏：{path}\n原因：{e}"
            )
        source_format = "base64"
    else:
        plaintext = content
        source_format = "明文"

    if not plaintext:
        print_error_information_and_exit(
            "handle_convert_password",
            f"内容解码后是空的，拒绝转换：{path}",
        )

    # ② 按目标等级编码写回
    if level == "3":
        if fmt == "encrypted":
            print(f"当前已是加密格式，无需转换：{path}")
            return
        master_key = get_master_key_or_exit("handle_convert_password")
        token = encrypt_password(plaintext, master_key)
        _write_and_echo(path, token, source_format=source_format, target_format="加密", level=level)
    elif level == "2":
        if fmt == "base64":
            print(f"当前已是 base64 编码，无需转换：{path}")
            return
        token = base64.b64encode(plaintext.encode("utf-8")).decode("ascii")
        _write_and_echo(path, token, source_format=source_format, target_format="base64", level=level)
    else:  # level == "1"
        if fmt == "plain":
            print(f"当前已是明文（文件内容即密码本身），无需转换：{path}")
            return
        _write_and_echo(path, plaintext, source_format=source_format, target_format="明文", level=level)


def _read_cred_content(path: str) -> str:
    """读取凭据文件内容并判空，空文件直接报错退出；附带文本/单行防御校验（复用凭据深模块）"""
    return read_cred_file_content(path)


def _write_and_echo(
    path: str, new_content: str, source_format: str, target_format: str, level: str
) -> None:
    """就地覆盖写入，再从磁盘回读输出（而非打印内存变量），让用户以文件内容为准确认转换成功"""
    try:
        with open(path, "w", encoding="utf-8") as f:
            f.write(new_content)
    except Exception as e:
        print_error_information_and_exit(
            "handle_convert_password", f"转换内容写入失败：{path}\n原因：{e}"
        )
    try:
        with open(path, "r", encoding="utf-8") as f:
            on_disk = f.read().strip()
    except Exception as e:
        print_error_information_and_exit(
            "handle_convert_password", f"写入后校验失败，无法读取转换结果：{path}\n原因：{e}"
        )

    print(f"转换成功：{source_format} → {target_format}（等级 {level}）：{path}")
    print()
    print("---------- 转换后文件内容 ----------")
    print(on_disk)
    print("----------------------------------")
