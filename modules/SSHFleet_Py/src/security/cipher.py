# -*- coding: utf-8 -*-
# SSHFleet 凭据加密引擎（纯标准库实现）
# 算法：SHA256 派生双钥 + 计数器密钥流 XOR 加密 + HMAC-SHA256 完整性校验
# 文件格式：base64( 版本号1字节 + 随机数16字节 + 密文n字节 + HMAC32字节 )
# 决策背景见 docs/adr/0007-homemade-cipher-env-key.md

import base64
import hashlib
import hmac
import os
import secrets
import re

# 格式版本号：未来算法升级时递增，解密侧按版本分发
VERSION = b"\x01"
NONCE_SIZE = 16
MAC_SIZE = 32
BLOCK_SIZE = 32

_BASE64_RE = re.compile(r"^[A-Za-z0-9+/]+={0,2}$")


class CipherError(ValueError):
    """加解密失败（格式非法 / HMAC 校验不通过）"""


def _derive_keys(master_key: str):
    """由主密钥派生加密钥与校验钥，避免同一把钥匙承担两个职责"""
    seed = hashlib.sha256(master_key.encode("utf-8")).digest()
    enc_key = hashlib.sha256(seed + b"enc").digest()
    mac_key = hashlib.sha256(seed + b"mac").digest()
    return enc_key, mac_key


def _keystream_xor(enc_key: bytes, nonce: bytes, data: bytes) -> bytes:
    """SHA256 计数器密钥流与数据逐块异或"""
    out = bytearray(len(data))
    pos = 0
    counter = 0
    while pos < len(data):
        block = hashlib.sha256(nonce + counter.to_bytes(8, "big") + enc_key).digest()
        chunk = data[pos:pos + BLOCK_SIZE]
        for i, byte in enumerate(chunk):
            out[pos + i] = byte ^ block[i]
        pos += BLOCK_SIZE
        counter += 1
    return bytes(out)


def encrypt_password(plaintext: str, master_key: str) -> str:
    """
    功能：
        把明文凭据加密为可落盘的文本形式

    参数：
        plaintext: 明文凭据
        master_key: 主密钥（环境变量 SSHFLEET_KEY 的值）

    返回：
        str: base64 文本，可直接写入凭据文件
    """
    nonce = os.urandom(NONCE_SIZE)
    enc_key, mac_key = _derive_keys(master_key)
    ciphertext = _keystream_xor(enc_key, nonce, plaintext.encode("utf-8"))
    mac = hmac.new(mac_key, VERSION + nonce + ciphertext, hashlib.sha256).digest()
    return base64.b64encode(VERSION + nonce + ciphertext + mac).decode("ascii")


def decrypt_password(token: str, master_key: str) -> str:
    """
    功能：
        解密 encrypt_password 产出的文本，还原明文凭据

    参数：
        token: 凭据文件内容（base64 文本）
        master_key: 主密钥

    返回：
        str: 明文凭据

    Raises:
        CipherError: 内容格式非法或 HMAC 校验失败（密钥不匹配 / 文件损坏）
    """
    try:
        raw = base64.b64decode("".join(token.split()), validate=True)
    except Exception as e:
        raise CipherError(f"不是有效的加密格式：{e}")
    if len(raw) < 1 + NONCE_SIZE + MAC_SIZE:
        raise CipherError("加密内容长度不足，文件可能被截断")
    if raw[:1] != VERSION:
        raise CipherError(f"不支持的加密格式版本：{raw[0]}")
    nonce = raw[1:1 + NONCE_SIZE]
    mac = raw[-MAC_SIZE:]
    ciphertext = raw[1 + NONCE_SIZE:-MAC_SIZE]
    enc_key, mac_key = _derive_keys(master_key)
    expected = hmac.new(mac_key, VERSION + nonce + ciphertext, hashlib.sha256).digest()
    if not hmac.compare_digest(mac, expected):
        raise CipherError("完整性校验失败：主密钥不匹配或文件已损坏")
    return _keystream_xor(enc_key, nonce, ciphertext).decode("utf-8")


def looks_encrypted(text: str) -> bool:
    """判断文本是否为本引擎产出的加密格式（仅看结构，不解密）"""
    try:
        raw = base64.b64decode("".join(text.split()), validate=True)
    except Exception:
        return False
    return len(raw) >= 1 + NONCE_SIZE + MAC_SIZE and raw[:1] == VERSION


def is_probably_base64_text(text: str) -> bool:
    """判断文本是否为规范 base64（重编码一致性回验，避免恰好合法的明文误判）"""
    compact = "".join(text.split())
    if len(compact) < 4 or len(compact) % 4 != 0:
        return False
    if not _BASE64_RE.fullmatch(compact):
        return False
    try:
        raw = base64.b64decode(compact, validate=True)
    except Exception:
        return False
    return base64.b64encode(raw).decode("ascii") == compact


def generate_master_key() -> str:
    """生成随机主密钥（约48字符 URL 安全文本）"""
    return secrets.token_urlsafe(36)
