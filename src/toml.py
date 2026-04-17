# -*- coding: utf-8 -*-
# SSHexec 读取配置文件
# 该文件负责读取配置文件，包括资产文件、输出文件、日志文件等

# 系统或第三方模块
import base64
import os
import sys

import toml
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

class SSHExecConfig(BaseModel):
    account: Account
    execution: Execution
    enable: Enable
    paths: Paths

def load_config(config_path: str) -> SSHExecConfig:
    """
    功能：
        加载配置文件
    参数：
        config_path: 配置文件路径
    返回：
        SSHExecConfig: 配置对象
    """
    if not os.path.exists(config_path):
        raise FileNotFoundError(f"配置文件 {config_path} 不存在")

    config_dict = toml.load(config_path)

    # 检查密码是否为有效的base64
    try:
        if config_dict["account"]["password"]:
            base64.b64decode(config_dict["account"]["password"])
    except Exception as e:
        print(f"\033[91m[ERROR]\033[0m\033[93m [function:load_config]\033[0m 密码不是有效的base64编码\n异常类型：\n{type(e)}\n异常信息：\n{e}")
        sys.exit(1)

    return SSHExecConfig(**config_dict)
