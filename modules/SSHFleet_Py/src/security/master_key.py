# -*- coding: utf-8 -*-
# SSHFleet 主密钥管理
# 职责：读取环境变量主密钥、--gen-key 生成并持久化（Linux 写 ~/.bashrc，Windows 写注册表）
# 决策背景见 docs/adr/0007-homemade-cipher-env-key.md

import os
import re
import subprocess
import sys

from src.common.error_handler import print_error_information_and_exit
from src.security.cipher import generate_master_key

ENV_NAME = "SSHFLEET_KEY"


def get_master_key_or_exit(func_name: str) -> str:
    """
    功能：
        从环境变量读取主密钥；缺失时报错退出并提示生成方式

    参数：
        func_name: 调用方函数名（用于错误信息定位）

    返回：
        str: 主密钥
    """
    key = os.environ.get(ENV_NAME, "").strip()
    if not key:
        print_error_information_and_exit(
            func_name,
            f"缺少主密钥，无法解密/加密凭据文件\n"
            f"请先生成主密钥：python sshfleet.py --gen-key",
        )
    return key


def _confirm_overwrite(prompt: str) -> bool:
    """覆盖确认交互，默认不覆盖"""
    try:
        answer = input(f"{prompt} [y/N]: ").strip().lower()
    except EOFError:
        return False
    return answer in ("y", "yes")


def _read_persisted_key() -> str:
    """从持久化位置读取已存主密钥（Linux: ~/.bashrc；Windows: 注册表），无则返回空串"""
    if sys.platform.startswith("win"):
        try:
            result = subprocess.run(
                ["reg", "query", r"HKCU\Environment", "/v", ENV_NAME],
                capture_output=True, text=True, check=False,
            )
            if result.returncode == 0:
                match = re.search(rf"{ENV_NAME}\s+REG_SZ\s+(\S+)", result.stdout)
                return match.group(1) if match else "unknown"
        except Exception:
            pass
        return ""
    bashrc = os.path.join(os.path.expanduser("~"), ".bashrc")
    if os.path.exists(bashrc):
        try:
            with open(bashrc, "r", encoding="utf-8") as f:
                for line in f:
                    if f"export {ENV_NAME}=" in line:
                        match = re.search(rf"export {ENV_NAME}=['\"]?([^'\"\s]+)", line)
                        return match.group(1) if match else "unknown"
        except Exception:
            pass
    return ""


def _persist_key(key: str, regenerated: bool = False) -> None:
    """
    功能：
        把主密钥持久化到系统（Windows: setx 写注册表；Linux: 追加 ~/.bashrc），
        成功后提示保存位置与生效方式

    参数：
        key: 随机主密钥
        regenerated: 是否为覆盖重生成场景（影响提示措辞）

    Raises:
        SystemExit: 持久化失败
    """
    action_desc = "重新生成" if regenerated else "生成"
    if sys.platform.startswith("win"):
        try:
            subprocess.run(
                ["setx", ENV_NAME, key],
                capture_output=True, text=True, check=True,
            )
            print(f"主密钥已{action_desc}，并自动保存到本机")
            print("请重新打开终端后再使用（当前终端读不到新密钥）")
        except Exception as e:
            print_error_information_and_exit(
                "_persist_key",
                f"主密钥自动保存失败：{e}\n可手动保存：执行 setx {ENV_NAME} 你的随机密钥",
            )
        return
    bashrc = os.path.join(os.path.expanduser("~"), ".bashrc")
    export_line = f"export {ENV_NAME}='{key}'"
    lines = []
    replaced = False
    if os.path.exists(bashrc):
        with open(bashrc, "r", encoding="utf-8") as f:
            lines = f.readlines()
    for idx, line in enumerate(lines):
        if f"export {ENV_NAME}=" in line:
            lines[idx] = export_line + "\n"
            replaced = True
            break
    if not replaced:
        if lines and not lines[-1].endswith("\n"):
            lines.append("\n")
        lines.append(f"# SSHFleet 主密钥\n{export_line}\n")
    try:
        with open(bashrc, "w", encoding="utf-8") as f:
            f.writelines(lines)
    except Exception as e:
        print_error_information_and_exit(
            "_persist_key",
            f"主密钥自动保存失败：{e}\n可手动保存：在 ~/.bashrc 追加 export {ENV_NAME}='你的随机密钥'",
        )
    print(f"主密钥已{action_desc}，并自动保存到 {bashrc}")
    print("请执行 source ~/.bashrc 或重新打开终端后生效")


def handle_gen_key(disinteractive: bool = False) -> None:
    """--gen-key 入口：生成随机主密钥并持久化，已有密钥时先确认覆盖

    参数：
        disinteractive: 非交互模式；已有密钥时拒绝自动覆盖（覆盖后旧加密文件无法解密，与 forbidden 危险命令同级兜底）
    """
    new_key = generate_master_key()
    env_key = os.environ.get(ENV_NAME, "").strip()
    persisted_key = _read_persisted_key()

    overwritten = False
    if env_key or persisted_key:
        print("检测到已存在主密钥，当前未做任何修改")
        print("注意：如果覆盖，用旧密钥加密的凭据文件将永久无法解密")
        if disinteractive:
            print_error_information_and_exit(
                "handle_gen_key",
                f"检测到已存在主密钥，非交互模式不自动覆盖"
                f"（覆盖后旧密钥加密的凭据文件将无法解密）。\n"
                f"如需覆盖：去掉 --disinteractive 后重新执行 --gen-key",
            )
        if not _confirm_overwrite("是否确认覆盖？"):
            print("已保留原密钥，未做修改")
            return
        overwritten = True
        print("已确认覆盖，正在重新生成主密钥...")

    _persist_key(new_key, regenerated=overwritten)
