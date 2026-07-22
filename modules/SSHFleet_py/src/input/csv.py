# -*- coding: utf-8 -*-
# SSHFleet CSV节点信息读取模块

import base64
import csv
import ipaddress
import os
import re
import sys
from typing import Dict, List

import src.color as color
import src.utils as utils
from src.yaml import SSHFleetConfig


@utils.error_and_exit_handling_decorator("read_nodes_infos", "读取节点信息失败")
def read_nodes_infos(csv_path: str, config: SSHFleetConfig) -> List[Dict[str, str]]:
    """
    功能：
        从CSV文件中读取节点信息，并进行数据验证和处理

    参数：
        csv_path (str): CSV文件的路径
        config (SSHFleetConfig): 配置对象

    返回：
        List[Dict[str, str]]: 处理后的节点信息列表，每个节点是一个字典，包含ip、port、user和password字段
    """

    # debug=True，跳过IP重复检查
    debug = True

    # 初始化存储字典
    csv_infos = []  # 存储从CSV读取的原始内容
    node_infos = []  # 存储处理后的节点信息
    csv_errors = []  # 存储错误信息

    # 读取CSV文件
    try:
        with open(csv_path, "r", encoding="utf-8") as file:
            reader = csv.reader(file)
            for row in reader:
                # 跳过空行和注释行
                if not row or (row[0].startswith("#") and row[0].strip() != ""):
                    continue
                csv_infos.append(row)
    except Exception as e:
        print(
            f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:read_nodes_infos]{color.COLOR_RESET} 读取文件时发生错误: {e}"
        )
        sys.exit(1)

    try:
        # 第0行0列不是ip格式，移除表头
        ipaddress.ip_address(csv_infos[0][0])  # 尝试解析 IP
    except ValueError:  # 如果不是合法IP，则移除表头
        csv_infos.pop(0)
        print(
            f"{color.COLOR_RED}[INFO]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:read_nodes_infos]{color.COLOR_RESET} 第一行不是IP格式，已移除表头行"
        )

    # 处理端口和用户的默认值及用户输入标志
    port_use_input = False  # 是否使用用户输入的端口值
    port_input_value = None  # 用户输入的端口值
    user_use_input = False  # 是否使用用户输入的用户名值
    user_input_value = None  # 用户输入的用户名值
    password_use_input = False  # 是否使用用户输入的密码值
    password_input_value = None  # 用户输入的密码值

    # 处理每个节点
    for idx, row in enumerate(csv_infos, start=1):
        # 确保行有足够的列
        while len(row) < 4:
            row.append("")

        ip, port, user, password = row[:4]
        errors = []

        # 验证IP地址
        if not ip or ip.strip() == "":
            errors.append("IP必须存在")
        else:
            ip = ip.strip()
            # 简单的IP格式验证
            ip_pattern = r"^(\d{1,3}\.){3}\d{1,3}$"
            if not re.match(ip_pattern, ip):
                errors.append("IP格式不正确")
            # 检查IP是否重复（在当前已处理的节点中）
            if not debug:
                for processed_node in node_infos:
                    if processed_node["ip"] == ip:
                        errors.append("IP地址重复")
                        break
        # 处理端口 - 按照优先级：CSV > config > 用户之前输入的值 > 用户输入新值
        port = port.strip() if port else ""
        if port:  # CSV中有值，最高优先级
            if not port.isdigit() or not (1 <= int(port) <= 65535):
                errors.append("端口必须是1-65535之间的整数，当前值为：" + port)
            else:
                port = int(port)
        elif config.account.port and config.account.port != "None":  # 使用配置的默认值
            port = config.account.port
            # 验证配置的默认端口
            if not str(port).isdigit() or not (1 <= int(port) <= 65535):
                errors.append("配置文件中的默认端口格式错误")
        elif port_use_input:  # 使用之前用户输入的值
            port = port_input_value
        else:  # 请求用户输入新值
            while True:
                try:
                    input_port = input(
                        f"行 {idx} (IP: {ip if ip else '空值'}): 端口为空，请输入端口号: "
                    )
                    if input_port.isdigit() and 1 <= int(input_port) <= 65535:
                        port = input_port
                        # 询问是否将此端口号应用于所有后续端口为空的节点
                        if not port_use_input and idx < len(
                            csv_infos
                        ):  # 检查是否还有后续节点
                            if utils.get_user_confirmation(
                                f"\n{color.COLOR_YELLOW}是否将此端口号应用于所有后续端口为空的节点？{color.COLOR_RESET}",
                                yorn=True,
                            ):
                                port_use_input = True
                                port_input_value = int(port)
                        break
                    else:
                        print("端口必须是1-65535之间的整数")
                except KeyboardInterrupt:
                    print("\n用户取消输入")
                    sys.exit(1)

        # 处理用户名 - 按照优先级：CSV > config > 用户之前输入的值 > 用户输入新值
        user = user.strip() if user else ""
        if user:  # CSV中有值，最高优先级
            pass  # 已经有值，不需要处理
        elif config.account.user and config.account.user != "None":  # 使用配置的默认值
            user = config.account.user
        elif user_use_input:  # 使用之前用户输入的值
            user = user_input_value
        else:  # 请求用户输入新值
            while True:
                try:
                    input_user = input(
                        f"行 {idx} (IP: {ip if ip else '空值'}): 用户名为空，请输入用户名: "
                    )
                    if input_user.strip():
                        user = input_user.strip()
                        # 询问是否将此用户名应用于所有后续用户为空的节点
                        if not user_use_input and idx < len(
                            csv_infos
                        ):  # 检查是否还有后续节点
                            if utils.get_user_confirmation(
                                f"\n{color.COLOR_YELLOW}是否将此用户名应用于所有后续用户为空的节点？{color.COLOR_RESET}",
                                yorn=True,
                            ):
                                user_use_input = True
                                user_input_value = user
                        break
                    else:
                        print("用户名不能为空")
                except KeyboardInterrupt:
                    print("\n用户取消输入")
                    sys.exit(1)

        # 处理密码 - 按照优先级：CSV > config > 用户之前输入的值 > 用户输入新值
        password = password.strip() if password else ""
        if password:  # CSV中有值，最高优先级
            pass  # 已经有值，不需要处理
        elif (
            config.account.password and config.account.password != "None"
        ):  # 使用配置的默认值
            # 验证密码文件有效性
            from src.input.args import validate_password_file
            validate_password_file(config.account.password)
            # 读取密码文件内容并解码base64
            with open(config.account.password, "r", encoding="utf-8") as f:
                password = base64.b64decode(f.read().strip()).decode("utf-8")
        elif password_use_input:  # 使用之前用户输入的值
            password = password_input_value
        else:  # 请求用户输入新值
            # 输出信息时才导入getpass模块，优化不必要的模块导入
            import getpass

            while True:
                try:
                    input_password = getpass.getpass(
                        f"行 {idx} (IP: {ip if ip else '空值'}): 密码为空，请输入密码: "
                    )
                    if input_password:
                        password = input_password
                        # 询问是否将此密码应用于所有后续密码为空的节点
                        if not password_use_input and idx < len(
                            csv_infos
                        ):  # 检查是否还有后续节点
                            if utils.get_user_confirmation(
                                f"\n{color.COLOR_YELLOW}是否将此密码应用于所有后续密码为空的节点？{color.COLOR_RESET}",
                                yorn=True,
                            ):
                                password_use_input = True
                                password_input_value = password
                        break
                    else:
                        print("密码不能为空，请重新输入")
                except KeyboardInterrupt:
                    print("\n用户取消输入")
                    sys.exit(1)

        # 如果有错误，添加到错误列表
        if errors:
            error_msg = "，".join(errors)
            csv_errors.append(f"行 {idx} (IP: {ip if ip else '空值'}): - {error_msg}")
        else:
            # 验证通过，添加到节点列表
            node_infos.append(
                {"ip": ip, "port": port, "user": user, "password": password}
            )

    # 检查是否有错误
    if csv_errors:
        print("发现以下错误:")
        for error in csv_errors:
            print(error)
        sys.exit(1)

    return node_infos
