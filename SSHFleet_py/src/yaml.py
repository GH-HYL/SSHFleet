# -*- coding: utf-8 -*-
# SSHFleet 读取配置文件
# 该文件负责读取配置文件，包括资产文件、输出文件、日志文件等

# 系统或第三方模块
import base64
import os
import sys

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
    private_key: str


class Account(BaseModel):
    port: int
    user: str
    password: str


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

    # 读取密码文件
    password_path = config_dict["account"]["password"]
    if password_path:
        password_path = os.path.expanduser(password_path)
        if not os.path.exists(password_path):
            raise FileNotFoundError(f"密码文件 {password_path} 不存在")

        with open(password_path, "r", encoding="utf-8") as f:
            password_b64 = f.read().strip()

        # 验证 base64 格式
        try:
            base64.b64decode(password_b64)
        except Exception as e:
            print(
                f"\033[91m[ERROR]\033[0m\033[93m [function:load_config]\033[0m 密码文件内容不是有效的base64编码\n异常类型：\n{type(e)}\n异常信息：\n{e}"
            )
            sys.exit(1)

        config_dict["account"]["password"] = password_b64

    return SSHFleetConfig(**config_dict)