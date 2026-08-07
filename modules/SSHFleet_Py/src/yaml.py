# -*- coding: utf-8 -*-
# SSHFleet 读取配置文件
# 该文件负责读取配置文件，包括资产文件、输出文件、日志文件等

# 系统或第三方模块
import os

import yaml
from pydantic import BaseModel


class Files(BaseModel):
    asset: str
    output: str
    output_xlsx: str
    report: str
    results_xlsx: str


class Logs(BaseModel):
    zip: str
    historys: str
    tool: str
    exec: str


class Exe(BaseModel):
    batch_tool_windows: str
    batch_tool_linux: str


class Jsons(BaseModel):
    error_keywords: str
    dangerous_keywords: str


class Paths(BaseModel):
    jsons: Jsons
    exe: Exe
    logs: Logs
    files: Files


class Enable(BaseModel):
    output_to_xlsx: bool
    results_to_xlsx: bool


class Execution(BaseModel):
    mode: str
    timeout_connect: int
    timeout_execute: int
    timeout_transfer: int


class Account(BaseModel):
    port: int
    user: str
    password_dir: str
    password: str
    key_passphrase: str = ""


class UploadConcurrencyThreshold(BaseModel):
    small_file: int        # < 此值：全并发（等于节点数）
    large_file: int        # > 此值：串行（并发=1）
    medium_concurrency: int  # 中间档并发数


class Upload(BaseModel):
    concurrency_thresholds: UploadConcurrencyThreshold


class SSHFleetConfig(BaseModel):
    account: Account
    execution: Execution
    enable: Enable
    paths: Paths
    upload: Upload


def load_config(config_path: str) -> SSHFleetConfig:
    """
    功能：
        加载配置文件
    参数：
        config_path: 配置文件路径
    返回：
        SSHFleetConfig: 配置对象
    """
    if not os.path.exists(config_path):
        raise FileNotFoundError(f"配置文件 {config_path} 不存在")

    with open(config_path, 'r', encoding='utf-8') as f:
        config_dict = yaml.safe_load(f)

    # 读取密码目录路径（不验证）
    password_dir = config_dict["account"].get("password_dir", "")
    if password_dir:
        password_dir = os.path.expanduser(password_dir)
        config_dict["account"]["password_dir"] = password_dir

    # 读取密码文件路径（不验证，相对路径与 password_dir 拼接）
    password_path = config_dict["account"]["password"]
    if password_path:
        password_path = os.path.expanduser(password_path)
        if not os.path.isabs(password_path):
            if password_dir:
                password_path = os.path.join(password_dir, password_path)
            else:
                raise ValueError(
                    f"account.password 为相对路径 '{password_path}'，"
                    f"但 account.password_dir 未配置，无法拼接密码文件路径"
                )
        config_dict["account"]["password"] = password_path

    # 读取密钥passphrase文件路径（不验证，相对路径与 password_dir 拼接）
    key_passphrase_path = config_dict["account"].get("key_passphrase", "")
    if key_passphrase_path:
        key_passphrase_path = os.path.expanduser(key_passphrase_path)
        if not os.path.isabs(key_passphrase_path):
            if password_dir:
                key_passphrase_path = os.path.join(password_dir, key_passphrase_path)
            else:
                raise ValueError(
                    f"account.key_passphrase 为相对路径 '{key_passphrase_path}'，"
                    f"但 account.password_dir 未配置，无法拼接路径"
                )
        config_dict["account"]["key_passphrase"] = key_passphrase_path

    return SSHFleetConfig(**config_dict)