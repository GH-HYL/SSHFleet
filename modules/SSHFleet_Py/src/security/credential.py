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


# ---- 错误码 → 用户文案（单一事实来源，校验汇总 / 退出指引两条路径共用） ----
# 本表贴着错误码的产生地（上方读取函数）放置。收敛前 csv.py 内 if 链与字典两张文案表
# 已措辞漂移（mismatch_encrypted），且互相缺码：if 链缺 empty_decoded/bad_pem
# （静默落 bad_base64 文案），字典无兜底（未知码直接 KeyError）。

_CREDENTIAL_LABELS = {
    "missing": "不存在",
    "read_error": "无法读取",
    "empty": "内容为空",
    "bad_base64": "不是有效的Base64编码",
    "empty_decoded": "解码后内容为空",
    "bad_pem": "不是有效的PEM格式（缺少 -----BEGIN 头）",
    "bad_cipher": "不是有效的加密格式或主密钥不匹配（请先用 --convert-password 转换该文件）",
    "mismatch_encrypted": "文件是本工具等级3（加密）格式，与当前密码安全等级不匹配",
    "mismatch_base64": "文件是等级2（base64）格式，与当前密码安全等级不匹配",
}


def credential_error_label(code: str, path: str, detail: Optional[str] = None) -> str:
    """凭据错误码 → 汇总短文案（CSV 校验汇总路径用）。未知码兜底，不再 KeyError。"""
    label = _CREDENTIAL_LABELS.get(code, f"未知凭据错误({code})")
    if detail:
        return f"{label} → {path} ({detail})"
    return f"{label} → {path}"


def credential_error_detail(code: str, level: str, path: str, detail: Optional[str]) -> str:
    """凭据错误码 → 退出前完整指引文案（读取失败即退路径用）。未知码兜底，不再误标为 bad_base64。"""
    if code == "missing":
        return f"凭据文件不存在：{path}"
    if code == "read_error":
        return f"凭据文件无法读取：{path} ({detail})"
    if code == "empty":
        return f"凭据文件内容为空：{path}"
    if code == "mismatch_encrypted":
        return (
            f"凭据文件是本工具等级3（加密）格式，与当前密码安全等级 {level}（1=明文 2=base64 3=加密）不匹配：{path}\n"
            f"请将配置 account.password_security 改为 3，或先用 --convert-password 处理该文件"
        )
    if code == "mismatch_base64":
        return (
            f"凭据文件是等级2（base64）格式，与当前密码安全等级 1（明文）不匹配：{path}\n"
            f"请先将该文件内容还原为明文，或将配置改为 2"
        )
    if code == "bad_cipher":
        if detail:  # 解密失败（密钥不匹配/文件损坏）
            return (
                f"凭据文件解密失败（文件可能尚未用 --convert-password 转换，或主密钥不匹配）：{path}\n{detail}"
            )
        return f"凭据文件不是等级{level}（加密）格式，请先 --convert-password 转换：{path}"
    if code == "bad_base64":
        return f"凭据文件不是等级{level}（base64）格式（内容疑似明文），请先 --convert-password 转换：{path}"
    if code == "empty_decoded":
        return f"凭据文件内容解码后为空，请检查文件是否填入了有效密码：{path}"
    if code == "bad_pem":
        return f"凭据文件不是有效的PEM格式（缺少 -----BEGIN 头）：{path}"
    return f"凭据文件校验失败（{code}）：{path}"


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
