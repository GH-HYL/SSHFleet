# -*- coding: utf-8 -*-
# SSHFleet 凭据深模块
# 职责：密码/口令类凭据「读盘 → 格式分类 → 安全等级匹配 → 解码/解密」的单一实现。
# 消除 csv.py 两套与 upgrade.py 一套的重复错配矩阵（架构审查候选 #2），
# 并缓存解码结果，避免校验阶段与使用阶段双重读盘/取主密钥（等级3时每节点只读盘一次）。
# 决策背景见 docs/adr/0007-homemade-cipher-env-key.md

import base64
import os
from typing import List, Optional, Tuple

from src.security.cipher import CipherError, classify_credential, decrypt_password, is_probably_base64_text, looks_encrypted
from src.security.master_key import get_master_key_or_exit

# 解码结果缓存：(path, level) -> 明文。进程内单次运行，仅加速不改变语义；
# 凭据文件运行中不会变化，无需失效；明文本就存在于流程内存（ADR-0007 不防内存明文）。
_decode_cache: dict = {}


def _check_fmt_level(fmt: str, level: str) -> Optional[str]:
    """凭据内容格式 × 密码安全等级的错配诊断（全工具单一事实来源）。

    返回错误码或 None（匹配）。错误码语义：
      mismatch_encrypted / mismatch_base64 / bad_cipher / bad_base64
    规则（与历史 _check_credential_file 一致，修正 _read_credential 缺失的 base64×3 分支）：
      encrypted 仅等级3匹配；base64 仅等级2匹配（等级3要求加密格式，报 bad_cipher）；
      plain 仅等级1匹配。
    """
    if fmt == "encrypted" and level != "3":
        return "mismatch_encrypted"
    if fmt == "base64" and level == "1":
        return "mismatch_base64"
    if fmt == "base64" and level == "3":
        return "bad_cipher"
    if fmt == "plain" and level == "2":
        return "bad_base64"
    if fmt == "plain" and level == "3":
        return "bad_cipher"
    return None


def decode_credential(content: str, level: str) -> Tuple[Optional[str], Optional[str], Optional[str]]:
    """按密码安全等级把凭据内容还原为明文（不读盘）。

    返回 (明文, 错误码, 错误细节)，明文与错误互斥：
      level=1 原样返回；level=2 base64 解码；level=3 主密钥解密。
    """
    fmt = classify_credential(content)
    code = _check_fmt_level(fmt, level)
    if code:
        return None, code, None
    if level == "1":
        return content, None, None
    if level == "3":
        master_key = get_master_key_or_exit("read_credential")
        try:
            return decrypt_password(content, master_key), None, None
        except CipherError as e:
            return None, "bad_cipher", str(e)
    try:
        return base64.b64decode(content).decode("utf-8"), None, None
    except Exception:
        return None, "bad_base64", None


def read_credential(path: str, level: str, require_nonempty: bool = False) -> Tuple[Optional[str], List[Tuple[str, Optional[str]]]]:
    """密码/口令类凭据：读盘 → 判空 → 格式分类 → 等级匹配 → 解码/解密（带缓存）。

    返回 (明文, 错误码列表)，空列表=通过；错误码与历史 _check_credential_file 兼容：
      missing / read_error / empty / bad_base64 / bad_cipher / mismatch_encrypted / mismatch_base64
    成功时结果按 (path, level) 缓存，后续读取（如逐节点解析）不再读盘/取主密钥。
    """
    cache_key = (path, level)
    if cache_key in _decode_cache:
        return _decode_cache[cache_key], []

    if not os.path.exists(path):
        return None, [("missing", None)]
    try:
        with open(path, "r", encoding="utf-8") as f:
            content = f.read().strip()
    except Exception as e:
        return None, [("read_error", str(e))]
    if not content:
        return None, [("empty", None)]

    plain, code, detail = decode_credential(content, level)
    if code:
        return None, [(code, detail)]
    if require_nonempty and not plain:
        return None, [("empty_decoded", None)]

    _decode_cache[cache_key] = plain
    return plain, []


def read_credential_pem(path: str) -> Tuple[Optional[str], List[Tuple[str, Optional[str]]]]:
    """私钥 PEM 读取校验：读盘 → 判空 → -----BEGIN 前缀检查（不参与 fmt×level 矩阵）。

    返回 (内容, 错误码列表)；错误码：missing / read_error / empty / bad_pem
    """
    if not os.path.exists(path):
        return None, [("missing", None)]
    try:
        with open(path, "r", encoding="utf-8") as f:
            content = f.read().strip()
    except Exception as e:
        return None, [("read_error", str(e))]
    if not content:
        return None, [("empty", None)]
    if not content.startswith("-----BEGIN"):
        return None, [("bad_pem", None)]
    return content, []


def read_cred_file_content(path: str) -> str:
    """凭据文件内容读取（--convert-password 转换场景）：读盘 + 判空 + 文本/单行防御。

    与原 upgrade._read_cred_content 行为一致，供转换流程复用；
    错误直接 print_error_information_and_exit 退出（转换流程约定）。

    Raises:
        SystemExit: 读盘失败 / 空文件 / 含二进制数据 / 多行非 base64 或加密格式
    """
    from src.common.error_handler import print_error_information_and_exit

    try:
        with open(path, "r", encoding="utf-8") as f:
            content = f.read().strip()
    except Exception as e:
        print_error_information_and_exit(
            "handle_convert_password", f"读取凭据文件失败：{path}\n原因：{e}"
        )
    if not content:
        print_error_information_and_exit(
            "handle_convert_password", f"凭据文件是空的，请先填入密码再转换：{path}"
        )
    # 防御：含 NUL 字节视为二进制文件，明确提示而非当作文本处理
    if "\x00" in content:
        print_error_information_and_exit(
            "handle_convert_password",
            f"凭据文件含二进制数据，不是文本格式，无法转换：{path}",
        )
    # 防御：明文凭据应为单行；多行 base64 / 加密格式（去空白后合法存储形态）放行
    if "\n" in content or "\r" in content:
        if not (is_probably_base64_text(content) or looks_encrypted(content)):
            print_error_information_and_exit(
                "handle_convert_password",
                f"凭据文件有多行内容，但密码应为单行，请检查是否误粘贴：{path}",
            )
    return content
