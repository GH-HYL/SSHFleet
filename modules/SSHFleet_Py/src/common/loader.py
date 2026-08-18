# -*- coding: utf-8 -*-
# SSHFleet 读取配置文件
# 该文件负责读取配置文件，包括资产文件、输出文件、日志文件等

# 系统或第三方模块
import os

import yaml
from pydantic import BaseModel, ConfigDict

from src.common.error_handler import error_and_exit_handling_decorator


class StrictModel(BaseModel):
    """严格配置模型：遇到未知字段（如已移除的 paths.logs.zip）直接报错，不做静默忽略"""

    model_config = ConfigDict(extra="forbid")


class Files(StrictModel):
    asset: str
    output: str
    output_xlsx: str
    report: str
    results_xlsx: str


class Logs(StrictModel):
    historys: str
    tool: str
    exec: str


class Exe(StrictModel):
    batch_tool_windows: str
    batch_tool_linux: str


class Keywords(StrictModel):
    error_keywords: str
    dangerous_keywords: str


class Paths(StrictModel):
    keywords: Keywords
    exe: Exe
    logs: Logs
    files: Files


class Enable(StrictModel):
    output_to_xlsx: bool
    results_to_xlsx: bool


class Execution(StrictModel):
    mode: str
    timeout_connect: int
    timeout_execute: int
    timeout_transfer: int


class Account(StrictModel):
    port: int
    user: str
    secret_dir: str
    password: str
    key: str = ""                  # 默认私钥文件路径（CSV 第 5 列留空时回退）
    key_passphrase: str = ""


class UploadConcurrencyThreshold(StrictModel):
    small_file: int        # < 此值：全并发（等于节点数）
    large_file: int        # > 此值：串行（并发=1）
    medium_concurrency: int  # 中间档并发数


class Upload(StrictModel):
    concurrency_thresholds: UploadConcurrencyThreshold


class SSHFleetConfig(StrictModel):
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

    # 读取凭据目录路径（不验证）：密码 / 私钥 / 私钥口令的相对路径都拼到这里
    secret_dir = config_dict["account"].get("secret_dir", "")
    if secret_dir:
        secret_dir = os.path.expanduser(secret_dir)
        config_dict["account"]["secret_dir"] = secret_dir

    # 读取默认密码文件路径（不验证，相对路径与 secret_dir 拼接）
    password_path = config_dict["account"]["password"]
    if password_path:
        password_path = os.path.expanduser(password_path)
        if not os.path.isabs(password_path):
            if secret_dir:
                password_path = os.path.join(secret_dir, password_path)
            else:
                raise ValueError(
                    f"account.password 为相对路径 '{password_path}'，"
                    f"但 account.secret_dir 未配置，无法拼接密码文件路径"
                )
        config_dict["account"]["password"] = password_path

    # 读取默认私钥文件路径（不验证，相对路径与 secret_dir 拼接）
    key_path = config_dict["account"].get("key", "")
    if key_path:
        key_path = os.path.expanduser(key_path)
        if not os.path.isabs(key_path):
            if secret_dir:
                key_path = os.path.join(secret_dir, key_path)
            else:
                raise ValueError(
                    f"account.key 为相对路径 '{key_path}'，"
                    f"但 account.secret_dir 未配置，无法拼接私钥文件路径"
                )
        config_dict["account"]["key"] = key_path

    # 读取默认私钥口令文件路径（不验证，相对路径与 secret_dir 拼接）
    key_passphrase_path = config_dict["account"].get("key_passphrase", "")
    if key_passphrase_path:
        key_passphrase_path = os.path.expanduser(key_passphrase_path)
        if not os.path.isabs(key_passphrase_path):
            if secret_dir:
                key_passphrase_path = os.path.join(secret_dir, key_passphrase_path)
            else:
                raise ValueError(
                    f"account.key_passphrase 为相对路径 '{key_passphrase_path}'，"
                    f"但 account.secret_dir 未配置，无法拼接路径"
                )
        config_dict["account"]["key_passphrase"] = key_passphrase_path

    return SSHFleetConfig(**config_dict)


@error_and_exit_handling_decorator("load_yaml_file", "YAML文件读取内容失败")
def load_yaml_file(path: str):
    """读取 YAML 文件并返回解析后的数据（支持 # 注释）"""
    with open(path, 'r', encoding='utf-8') as f:
        return yaml.safe_load(f)