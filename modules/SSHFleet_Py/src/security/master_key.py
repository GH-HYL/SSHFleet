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

# 缺少主密钥时的通用教程文案
TUTORIAL_TEXT = (
    f"配置方法（二选一）：\n"
    f"  方式一（推荐）：在本工程目录执行  python sshfleet.py --gen-key  ，自动生成并持久化密钥\n"
    f"  方式二（手动）：\n"
    f"    Linux  : 在 ~/.bashrc 追加  export {ENV_NAME}='你的随机密钥'  ，然后执行 source ~/.bashrc\n"
    f"    Windows: 执行  setx {ENV_NAME} 你的随机密钥  ，然后重新打开终端生效"
)


def get_master_key_or_exit(func_name: str) -> str:
    """
    功能：
        从环境变量读取主密钥；缺失时报错退出并附教程

    参数：
        func_name: 调用方函数名（用于错误信息定位）

    返回：
        str: 主密钥
    """
    key = os.environ.get(ENV_NAME, "").strip()
    if not key:
        print_error_information_and_exit(
            func_name,
            f"未检测到环境变量 {ENV_NAME}，无法解密凭据文件\n{TUTORIAL_TEXT}",
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


def _persist_key(key: str) -> None:
    """
    功能：
        把主密钥持久化到系统（Windows: setx 写注册表；Linux: 追加 ~/.bashrc）

    参数：
        key: 随机主密钥

    Raises:
        SystemExit: 持久化失败
    """
    if sys.platform.startswith("win"):
        try:
            result = subprocess.run(
                ["setx", ENV_NAME, key],
                capture_output=True, text=True, check=True,
            )
            print("已写入用户注册表（HKCU\\Environment）")
            print("注意：当前终端读不到新密钥，请重新打开终端后使用")
        except Exception as e:
            print_error_information_and_exit(
                "_persist_key",
                f"写入注册表失败：{e}\n可手动执行：setx {ENV_NAME} 你的随机密钥",
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
            f"写入 {bashrc} 失败：{e}\n可手动在 ~/.bashrc 追加：export {ENV_NAME}='你的随机密钥'",
        )
    print(f"已写入 {bashrc}")
    print("提示：执行 source ~/.bashrc 或重新打开终端后生效")


def handle_gen_key(disinteractive: bool = False) -> None:
    """--gen-key 入口：生成随机主密钥并持久化，已有密钥时先确认覆盖

    参数：
        disinteractive: 非交互模式；已有密钥时拒绝自动覆盖（覆盖后旧加密文件无法解密，与 forbidden 危险命令同级兜底）
    """
    new_key = generate_master_key()
    env_key = os.environ.get(ENV_NAME, "").strip()
    persisted_key = _read_persisted_key()

    if env_key or persisted_key:
        print(f"检测到已存在主密钥：")
        if env_key:
            print(f"  - 当前进程环境变量 {ENV_NAME}: 已设置")
        if persisted_key:
            shown = persisted_key if persisted_key != "unknown" else "（无法解析内容）"
            print(f"  - 持久化位置: {shown[:12]}...")
        if disinteractive:
            print_error_information_and_exit(
                "handle_gen_key",
                f"检测到已存在主密钥，--disinteractive 非交互模式下不自动覆盖"
                f"（覆盖后旧加密文件将无法解密）。\n"
                f"如需覆盖请去掉 --disinteractive 后重新执行 --gen-key 并确认",
            )
        if not _confirm_overwrite("是否覆盖原密钥？覆盖后旧加密文件将无法解密"):
            print("已保留原密钥，未做任何修改")
            return
        print("已确认覆盖，正在重新生成...")

    _persist_key(new_key)
    print(f"新主密钥已生成并持久化到环境变量 {ENV_NAME}")
