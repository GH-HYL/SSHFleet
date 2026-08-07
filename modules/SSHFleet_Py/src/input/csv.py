# -*- coding: utf-8 -*-
# SSHFleet CSV节点信息读取模块

import base64
import csv
import ipaddress
import io
import os
import re
import sys
from typing import Dict, List, Tuple

import src.color as color
import src.utils as utils
from src.yaml import SSHFleetConfig


def resolve_credential_path(raw_value: str, secret_dir: str) -> str:
    """
    将 CSV 凭据列原始值解析为绝对路径（密码 / 私钥 / 私钥口令 通用）

    Args:
        raw_value: CSV 凭据列原始值
        secret_dir: 配置的凭据目录

    Returns:
        绝对路径

    Raises:
        SystemExit: 路径解析失败
    """
    raw = raw_value.strip()

    # ~ 开头：展开 HOME
    if raw.startswith("~"):
        return os.path.expanduser(raw)

    # 绝对路径：直接使用
    if os.path.isabs(raw):
        return raw

    # 相对路径：与 secret_dir 拼接
    if not secret_dir or secret_dir == "None":
        utils.print_error_information_and_exit(
            "resolve_credential_path",
            "凭据列包含相对路径，但 secret_dir 未配置"
        )

    return os.path.join(secret_dir, raw)


def validate_csv_credentials(csv_infos: List[List[str]], config: SSHFleetConfig) -> Tuple[List[str], bool, bool]:
    """
    预检查所有凭据文件（密码+密钥），通过才继续处理节点

    Args:
        csv_infos: 已移除表头的CSV行列表
        config: 配置对象

    Returns:
        (errors, need_default_password, any_node_uses_key):
        - errors: 校验错误信息列表（为空表示通过）
        - need_default_password: 是否存在依赖默认密码的节点
        - any_node_uses_key: 是否存在使用密钥认证的节点
    """

    errors = []
    need_default_password = False
    any_node_uses_key = False  # 是否有节点使用了密钥

    for idx, row in enumerate(csv_infos, start=1):
        while len(row) < 6:
            row.append("")

        password = row[3].strip()
        key = row[4].strip()
        # 第5列（私钥路径）留空时，回退到配置默认私钥 account.key
        if not key and config.account.key and config.account.key != "None":
            key = config.account.key

        if password:
            # 有密码路径：解析并验证文件
            password_path = resolve_credential_path(password, config.account.secret_dir)
            # 检查文件是否存在
            if not os.path.exists(password_path):
                errors.append(f"行 {idx} (IP: {row[0]}): 密码文件不存在 → {password_path}")
                continue
            # 检查文件是否可读且非空
            try:
                with open(password_path, "r", encoding="utf-8") as f:
                    content = f.read().strip()
            except Exception as e:
                errors.append(f"行 {idx} (IP: {row[0]}): 密码文件无法读取 → {password_path} ({e})")
                continue
            if not content:
                errors.append(f"行 {idx} (IP: {row[0]}): 密码文件内容为空 → {password_path}")
                continue
            # 检查是否为有效的Base64编码
            try:
                decoded = base64.b64decode(content)
            except Exception:
                errors.append(f"行 {idx} (IP: {row[0]}): 密码文件不是有效的Base64编码 → {password_path}")
                continue
            if not decoded:
                errors.append(f"行 {idx} (IP: {row[0]}): 密码文件解码后内容为空 → {password_path}")
                continue
        elif key:
            # 仅提供密钥：无需默认密码
            pass
        else:
            # 密码列和密钥列均为空：依赖配置中的默认密码
            need_default_password = True

        if key:
            # 有密钥路径：解析并验证文件（复用 resolve_credential_path）
            key_path = resolve_credential_path(key, config.account.secret_dir)
            # 检查文件是否存在
            if not os.path.exists(key_path):
                errors.append(f"行 {idx} (IP: {row[0]}): 密钥文件不存在 → {key_path}")
                continue
            # 检查文件是否可读且非空
            try:
                with open(key_path, "r", encoding="utf-8") as f:
                    content = f.read().strip()
            except Exception as e:
                errors.append(f"行 {idx} (IP: {row[0]}): 密钥文件无法读取 → {key_path} ({e})")
                continue
            if not content:
                errors.append(f"行 {idx} (IP: {row[0]}): 密钥文件内容为空 → {key_path}")
                continue
            # 检查PEM格式（以 -----BEGIN 开头）
            if not content.startswith("-----BEGIN"):
                errors.append(f"行 {idx} (IP: {row[0]}): 密钥文件不是有效的PEM格式（缺少 -----BEGIN 头） → {key_path}")
                continue
            any_node_uses_key = True

        # 第6列：私钥口令文件（仅当密钥存在时有意义）
        if key:
            passphrase_raw = row[5].strip() if len(row) > 5 else ""
            if passphrase_raw:
                pp_path = resolve_credential_path(passphrase_raw, config.account.secret_dir)
                if not os.path.exists(pp_path):
                    errors.append(f"行 {idx} (IP: {row[0]}): 私钥口令文件不存在 → {pp_path}")
                else:
                    try:
                        with open(pp_path, "r", encoding="utf-8") as f:
                            pp_content = f.read().strip()
                    except Exception as e:
                        errors.append(f"行 {idx} (IP: {row[0]}): 私钥口令文件无法读取 → {pp_path} ({e})")
                    else:
                        if not pp_content:
                            errors.append(f"行 {idx} (IP: {row[0]}): 私钥口令文件内容为空 → {pp_path}")
                        else:
                            try:
                                base64.b64decode(pp_content)
                            except Exception:
                                errors.append(f"行 {idx} (IP: {row[0]}): 私钥口令文件不是有效的Base64编码 → {pp_path}")

    # 如果有空密码行，验证一次默认密码
    if need_default_password:
        if not config.account.password or config.account.password == "None":
            errors.append("密码列有空值，但 config 未配置默认密码(account.password)")
        else:
            if not os.path.exists(config.account.password):
                errors.append(f"默认密码文件不存在 → {config.account.password}")
            else:
                try:
                    with open(config.account.password, "r", encoding="utf-8") as f:
                        content = f.read().strip()
                except Exception as e:
                    errors.append(f"默认密码文件无法读取 → {config.account.password} ({e})")
                else:
                    if not content:
                        errors.append(f"默认密码文件内容为空 → {config.account.password}")
                    else:
                        try:
                            decoded = base64.b64decode(content)
                        except Exception:
                            errors.append(f"默认密码文件不是有效的Base64编码 → {config.account.password}")
                        else:
                            if not decoded:
                                errors.append(f"默认密码文件解码后内容为空 → {config.account.password}")

    # 如果有节点使用密钥且配置了passphrase，验证passphrase文件
    if any_node_uses_key and config.account.key_passphrase and config.account.key_passphrase != "":
        passphrase_path = config.account.key_passphrase
        if not os.path.exists(passphrase_path):
            errors.append(f"密钥passphrase文件不存在 → {passphrase_path}")
        else:
            try:
                with open(passphrase_path, "r", encoding="utf-8") as f:
                    content = f.read().strip()
            except Exception as e:
                errors.append(f"密钥passphrase文件无法读取 → {passphrase_path} ({e})")
            else:
                if not content:
                    errors.append(f"密钥passphrase文件内容为空 → {passphrase_path}")
                else:
                    try:
                        decoded = base64.b64decode(content)
                    except Exception:
                        errors.append(f"密钥passphrase文件不是有效的Base64编码 → {passphrase_path}")
                    else:
                        if not decoded:
                            errors.append(f"密钥passphrase文件解码后内容为空 → {passphrase_path}")

    if errors:
        print(f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET} CSV凭据预检查失败：")
        for error in errors:
            print(f"  {error}")
    return errors, need_default_password, any_node_uses_key


@utils.error_and_exit_handling_decorator("read_nodes_infos", "读取节点信息失败")
def read_nodes_infos(csv_path: str, config: SSHFleetConfig, disinteractive: bool = False, is_inline: bool = False) -> List[Dict[str, str]]:
    """
    功能：
        从CSV文件或内联文本中读取节点信息，并进行数据验证和处理

    参数：
        csv_path (str): CSV文件的路径或内联CSV文本
        config (SSHFleetConfig): 配置对象
        is_inline (bool): 是否为内联文本模式

    返回：
        List[Dict[str, str]]: 处理后的节点信息列表，每个节点是一个字典，包含ip、port、user和password字段
    """

    # debug=True，跳过IP重复检查
    debug = True

    # 初始化存储字典
    csv_infos = []  # 存储从CSV读取的原始内容
    node_infos = []  # 存储处理后的节点信息
    csv_errors = []  # 存储错误信息

    # 读取CSV内容
    try:
        if is_inline:
            # 内联文本模式：用 StringIO 包装
            file = io.StringIO(csv_path)
            reader = csv.reader(file)
            for row in reader:
                if not row or (row[0].startswith("#") and row[0].strip() != ""):
                    continue
                csv_infos.append(row)
            file.close()
        else:
            # 文件模式：读取文件
            with open(csv_path, "r", encoding="utf-8") as file:
                reader = csv.reader(file)
                for row in reader:
                    # 跳过空行和注释行
                    if not row or (row[0].startswith("#") and row[0].strip() != ""):
                        continue
                    csv_infos.append(row)
    except Exception as e:
        print(
            f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:read_nodes_infos]{color.COLOR_RESET} 读取内容时发生错误: {e}"
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

    # 凭据路径预检查（全部通过才继续处理节点）
    validate_errors, _, _ = validate_csv_credentials(csv_infos, config)
    if validate_errors:
        sys.exit(1)

    # 读取全局passphrase（仅一次，所有节点共用）
    key_passphrase = ""
    if config.account.key_passphrase and config.account.key_passphrase != "":
        with open(config.account.key_passphrase, "r", encoding="utf-8") as f:
            key_passphrase = base64.b64decode(f.read().strip()).decode("utf-8")

    # 处理每个节点
    for idx, row in enumerate(csv_infos, start=1):
        # 确保行有足够的列
        while len(row) < 6:
            row.append("")

        ip, port, user, password, key = row[:5]
        # 第5列（私钥路径）留空时回退到配置默认私钥 account.key
        if not key and config.account.key and config.account.key != "None":
            key = config.account.key
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
                                disinteractive=disinteractive,
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
                                disinteractive=disinteractive,
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
            password_path = resolve_credential_path(password, config.account.secret_dir)
            with open(password_path, "r", encoding="utf-8") as f:
                password = base64.b64decode(f.read().strip()).decode("utf-8")
        elif (
            config.account.password and config.account.password != "None"
        ):  # 使用配置的默认值
            # 读取密码文件内容并解码base64（预检查已验证文件有效性）
            with open(config.account.password, "r", encoding="utf-8") as f:
                password = base64.b64decode(f.read().strip()).decode("utf-8")
        elif key:  # 仅使用密钥认证，无需密码
            password = ""
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
                                disinteractive=disinteractive,
                            ):
                                password_use_input = True
                                password_input_value = password
                        break
                    else:
                        print("密码不能为空，请重新输入")
                except KeyboardInterrupt:
                    print("\n用户取消输入")
                    sys.exit(1)

        # 处理密钥内容（PEM原始文本，无需base64编码）
        key = key.strip() if key else ""
        key_content = ""
        if key:
            key_path = resolve_credential_path(key, config.account.secret_dir)
            with open(key_path, "r", encoding="utf-8") as f:
                key_content = f.read().strip()

        # 处理私钥口令（passphrase）：CSV 第6列优先，缺省用全局配置
        node_key_passphrase = ""
        passphrase_raw = row[5].strip() if len(row) > 5 else ""
        if passphrase_raw:
            pp_path = resolve_credential_path(passphrase_raw, config.account.secret_dir)
            with open(pp_path, "r", encoding="utf-8") as f:
                node_key_passphrase = base64.b64decode(f.read().strip()).decode("utf-8")
        elif key_passphrase:
            # 全局配置（已在文件开头解码）
            node_key_passphrase = key_passphrase

        # 如果有错误，添加到错误列表
        if errors:
            error_msg = "，".join(errors)
            csv_errors.append(f"行 {idx} (IP: {ip if ip else '空值'}): - {error_msg}")
        else:
            # 验证通过，添加到节点列表
            node_infos.append(
                {
                    "ip": ip,
                    "port": port,
                    "user": user,
                    "password": password,
                    "key_content": key_content,
                    "key_passphrase": node_key_passphrase,
                }
            )

    # 检查是否有错误
    if csv_errors:
        print("发现以下错误:")
        for error in csv_errors:
            print(error)
        sys.exit(1)

    return node_infos
