# -*- coding: utf-8 -*-
# SSHFleet CSV节点信息读取模块

import base64
import csv
import ipaddress
import io
import os
import re
import sys
from dataclasses import dataclass
from typing import Dict, List, Optional, Tuple, Union

import src.common.constants as color
from src.common.error_handler import error_and_exit_handling_decorator, print_error_information_and_exit
from src.common.loader import SSHFleetConfig
from src.input.interaction import get_user_confirmation


@dataclass
class FieldMemory:
    """跨节点累积的输入记忆：是否把本次交互输入应用到后续空字段节点"""

    port_use_input: bool = False
    port_input_value: Optional[int] = None
    user_use_input: bool = False
    user_input_value: Optional[str] = None
    password_use_input: bool = False
    password_input_value: Optional[str] = None


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
        print_error_information_and_exit(
            "resolve_credential_path",
            "凭据列包含相对路径，但 secret_dir 未配置"
        )

    return os.path.join(secret_dir, raw)


def _get_key_mode(args) -> str:
    """密钥三态：off=未指定 -k；default=仅 -k 无路径；universal=-k 带路径"""
    if not args.k:
        return "off"
    if args.k == "no_value":
        return "default"
    return "universal"


def _read_credential(path: str, decode_base64: bool = True) -> str:
    """读取凭据文件：去空白，按需 Base64 解码为 UTF-8 文本"""
    with open(path, "r", encoding="utf-8") as f:
        content = f.read().strip()
    if decode_base64:
        return base64.b64decode(content).decode("utf-8")
    return content


def _check_credential_file(path: str, kind: str = "base64") -> List[Tuple[str, Optional[str]]]:
    """校验凭据文件，返回 [(错误码, 细节)] 列表，空列表=通过

    kind: "base64"=内容可解码（与原实现的口令校验一致，不判空）；
         "base64_nonempty"=内容可解码且非空（密码类校验）；
         "pem"=内容以 -----BEGIN 开头
    错误码: missing / read_error / empty / bad_base64 / empty_decoded / bad_pem
    """
    if not os.path.exists(path):
        return [("missing", None)]
    try:
        with open(path, "r", encoding="utf-8") as f:
            content = f.read().strip()
    except Exception as e:
        return [("read_error", str(e))]
    if not content:
        return [("empty", None)]
    if kind in ("base64", "base64_nonempty"):
        try:
            decoded = base64.b64decode(content)
        except Exception:
            return [("bad_base64", None)]
        if kind == "base64_nonempty" and not decoded:
            return [("empty_decoded", None)]
    elif kind == "pem":
        if not content.startswith("-----BEGIN"):
            return [("bad_pem", None)]
    return []


_CREDENTIAL_MSG = {
    "missing": "不存在",
    "read_error": "无法读取",
    "empty": "内容为空",
    "bad_base64": "不是有效的Base64编码",
    "empty_decoded": "解码后内容为空",
    "bad_pem": "不是有效的PEM格式（缺少 -----BEGIN 头）",
}


def _credential_msg(code: str, path: str, detail: Optional[str] = None) -> str:
    """凭据校验错误码 → 完整错误文案（不含行号/IP 前缀）"""
    if code == "read_error":
        return f"{_CREDENTIAL_MSG[code]} → {path} ({detail})"
    return f"{_CREDENTIAL_MSG[code]} → {path}"


def validate_csv_credentials(csv_infos: List[List[str]], config: SSHFleetConfig, args) -> Tuple[List[str], bool, bool]:
    """
    预检查所有凭据文件（密码+密钥），通过才继续处理节点

    密钥预检查受 -k 三态控制：
      - 状态1(off, 未指定 -k)：跳过所有密钥相关检查
      - 状态2(default, 仅 -k)：逐节点检查 CSV 第5列/account.key 密钥文件及第6列口令文件
      - 状态3(universal, -k 路径)：统一检查 -k 指定的私钥文件一次，忽略节点自带密钥/口令

    Args:
        csv_infos: 已移除表头的CSV行列表
        config: 配置对象
        args: 命令行参数对象（使用 args.k 判断密钥模式）

    Returns:
        (errors, need_default_password, any_node_uses_key):
        - errors: 校验错误信息列表（为空表示通过）
        - need_default_password: 是否存在依赖默认密码的节点
        - any_node_uses_key: 是否存在使用密钥认证的节点
    """

    errors = []
    need_default_password = False
    any_node_uses_key = False  # 是否有节点使用了密钥

    key_mode = _get_key_mode(args)

    # 状态3：统一检查命令行私钥一次（不走 secret_dir，走终端工作目录）
    if key_mode == "universal":
        kp = os.path.expanduser(args.k)
        if not os.path.exists(kp):
            errors.append(f"-k 指定的私钥文件不存在 → {kp}")
        elif not os.access(kp, os.R_OK):
            errors.append(f"-k 指定的私钥文件不可读 → {kp}")
        else:
            try:
                with open(kp, "r", encoding="utf-8") as f:
                    content = f.read()
            except Exception as e:
                errors.append(f"-k 指定的私钥文件读取失败 → {kp} ({e})")
            else:
                if not content.strip():
                    errors.append(f"-k 指定的私钥文件内容为空 → {kp}")
                elif not content.strip().startswith("-----BEGIN"):
                    errors.append(f"-k 指定的文件不是有效的PEM私钥 → {kp}")

    for idx, row in enumerate(csv_infos, start=1):
        while len(row) < 6:
            row.append("")

        password = row[3].strip()
        key = row[4].strip()

        # 状态3：所有节点统一用命令行私钥；状态2：CSV > 配置默认；状态1：强制空
        if key_mode == "universal":
            key = args.k
        elif key_mode == "default":
            if not key and config.account.key and config.account.key != "None":
                key = config.account.key

        if password:
            # 有密码路径：解析并验证文件
            password_path = resolve_credential_path(password, config.account.secret_dir)
            password_errs = _check_credential_file(password_path, "base64_nonempty")
            if password_errs:
                errors.extend(
                    f"行 {idx} (IP: {row[0]}): 密码文件{_credential_msg(code, password_path, detail)}"
                    for code, detail in password_errs
                )
                continue
        elif key:
            # 仅提供密钥：无需默认密码
            pass
        else:
            # 密码列和密钥列均为空：依赖配置中的默认密码
            need_default_password = True

        # 密钥文件检查：仅状态2 逐节点（状态3 已统一检查；状态1 key 为空跳过）
        if key_mode == "default" and key:
            key_path = resolve_credential_path(key, config.account.secret_dir)
            key_errs = _check_credential_file(key_path, "pem")
            if key_errs:
                errors.extend(
                    f"行 {idx} (IP: {row[0]}): 密钥文件{_credential_msg(code, key_path, detail)}"
                    for code, detail in key_errs
                )
                continue
            any_node_uses_key = True

        # 第6列：私钥口令文件（仅状态2 检查；状态3 走交互忽略第6列；状态1 跳过）
        if key_mode == "default" and key:
            passphrase_raw = row[5].strip() if len(row) > 5 else ""
            if passphrase_raw:
                pp_path = resolve_credential_path(passphrase_raw, config.account.secret_dir)
                pp_errs = _check_credential_file(pp_path, "base64")
                errors.extend(
                    f"行 {idx} (IP: {row[0]}): 私钥口令文件{_credential_msg(code, pp_path, detail)}"
                    for code, detail in pp_errs
                )

    # 如果有空密码行，验证一次默认密码
    if need_default_password:
        if not config.account.password or config.account.password == "None":
            errors.append("密码列有空值，但 config 未配置默认密码(account.password)")
        else:
            errors.extend(
                f"默认密码文件{_credential_msg(code, config.account.password, detail)}"
                for code, detail in _check_credential_file(config.account.password, "base64_nonempty")
            )

    # 如果有节点使用密钥且配置了passphrase，验证passphrase文件（仅状态2）
    if key_mode == "default" and any_node_uses_key and config.account.key_passphrase and config.account.key_passphrase != "":
        errors.extend(
            f"密钥passphrase文件{_credential_msg(code, config.account.key_passphrase, detail)}"
            for code, detail in _check_credential_file(config.account.key_passphrase, "base64_nonempty")
        )

    if errors:
        print(f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET} CSV凭据预检查失败：")
        for error in errors:
            print(f"  {error}")
    return errors, need_default_password, any_node_uses_key

@error_and_exit_handling_decorator("read_nodes_infos", "读取节点信息失败")
def read_nodes_infos(csv_path: str, config: SSHFleetConfig, args, is_inline: bool = False) -> List[Dict[str, str]]:
    """
    功能：
        从CSV文件或内联文本中读取节点信息，并进行数据验证和处理

    参数：
        csv_path (str): CSV文件的路径或内联CSV文本
        config (SSHFleetConfig): 配置对象
        args: 命令行参数对象（使用 args.disinteractive / args.k 判断密钥模式）
        is_inline (bool): 是否为内联文本模式

    返回：
        List[Dict[str, str]]: 处理后的节点信息列表，每个节点是一个字典，包含ip、port、user和password字段
    """

    key_mode = _get_key_mode(args)

    # 1. 读取并清洗 CSV 行（inline/文件、判空、去表头）
    csv_infos = _read_csv_rows(csv_path, is_inline)

    # 2. 凭据路径预检查（全部通过才继续处理节点）
    validate_errors, _, _ = validate_csv_credentials(csv_infos, config, args)
    if validate_errors:
        sys.exit(1)

    # 3. 读取全局passphrase（仅状态2，一次读取所有节点共用）
    key_passphrase = ""
    if key_mode == "default" and config.account.key_passphrase and config.account.key_passphrase != "":
        key_passphrase = _read_credential(config.account.key_passphrase)

    # 4. 状态3：统一私钥——循环前读取一次，口令直接问用户（空/回车=无口令，真不真交给 Go）
    universal_key_content = ""
    universal_key_passphrase = ""
    if key_mode == "universal":
        universal_key_content = _read_credential(os.path.expanduser(args.k), decode_base64=False)
        if args.disinteractive:
            print_error_information_and_exit(
                "read_nodes_infos",
                "状态3(-k 路径)需交互输入私钥口令，但处于 --disinteractive 模式；"
                "请去掉 --disinteractive 或改用状态2(-k 不带路径)"
            )
        import getpass
        universal_key_passphrase = getpass.getpass("请输入私钥口令(无口令直接回车): ")

    # 5. 逐节点解析（跨节点输入记忆由 FieldMemory 累积）
    mem = FieldMemory()
    node_infos = []
    csv_errors = []
    total_nodes = len(csv_infos)
    for idx, row in enumerate(csv_infos, start=1):
        node, error_msg = _parse_node(
            row, idx, key_mode, config, args, mem,
            key_passphrase, universal_key_content, universal_key_passphrase,
            total_nodes,
        )
        if error_msg:
            csv_errors.append(error_msg)
        else:
            node_infos.append(node)

    # 6. 错误输出与返回
    if csv_errors:
        print("发现以下错误:")
        for error in csv_errors:
            print(error)
        sys.exit(1)

    return node_infos


def _read_csv_rows(csv_path: str, is_inline: bool) -> List[List[str]]:
    """读取并清洗 CSV 行：跳过空行/注释行、判空、移除表头"""
    csv_infos = []

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

    # 有效节点判空（前置）：csv_infos 里每行就是一个候选节点，一行都没有直接提示退出
    # 同时避免下方 csv_infos[0][0] 在空列表时 IndexError 崩溃
    if not csv_infos:
        print(
            f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:read_nodes_infos]{color.COLOR_RESET} CSV 中未解析出任何有效节点，请检查节点文件或内联文本内容"
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
        # 移除表头后仍无有效行（如文件只有表头一行）→ 提示退出
        if not csv_infos:
            print(
                f"{color.COLOR_RED}[ERROR]{color.COLOR_RESET}{color.COLOR_YELLOW} [function:read_nodes_infos]{color.COLOR_RESET} CSV 中未解析出任何有效节点，请检查节点文件或内联文本内容"
            )
            sys.exit(1)

    return csv_infos


def _parse_node(row, idx, key_mode, config, args, mem, key_passphrase, universal_key_content, universal_key_passphrase, total_nodes) -> Tuple[Optional[Dict], Optional[str]]:
    """解析单个节点行，返回 (node_info, error_msg) 二选一"""
    # 确保行有足够的列
    while len(row) < 6:
        row.append("")

    ip, port, user, password, key = row[:5]
    # 密钥路径按三态决定：命令行 -k 路径 > CSV第5列 > 配置默认 account.key
    if key_mode == "universal":
        key = args.k
    elif key_mode == "default":
        if not key and config.account.key and config.account.key != "None":
            key = config.account.key
    # 状态1(off)：key 保持空
    errors = []

    # 验证IP地址（原实现 IP 重复检查由 debug=True 恒关闭，随重构移除）
    if not ip or ip.strip() == "":
        errors.append("IP必须存在")
    else:
        ip = ip.strip()
        # 简单的IP格式验证
        ip_pattern = r"^(\d{1,3}\.){3}\d{1,3}$"
        if not re.match(ip_pattern, ip):
            errors.append("IP格式不正确")

    # 字段补全：端口 / 用户名 / 密码（CSV > config > 输入记忆 > 交互输入）
    port, port_errors = _resolve_port(port, config.account.port, mem, idx, total_nodes, ip, args.disinteractive)
    errors.extend(port_errors)
    user = _resolve_user(user, config.account.user, mem, idx, total_nodes, ip, args.disinteractive)
    password = _resolve_password(password, config, bool(key), mem, idx, total_nodes, ip, args.disinteractive)

    # 处理密钥内容（PEM原始文本，无需base64编码）
    key = key.strip() if key else ""
    key_content = ""
    if key_mode == "universal":
        key_content = universal_key_content
    elif key:
        key_path = resolve_credential_path(key, config.account.secret_dir)
        key_content = _read_credential(key_path, decode_base64=False)

    # 处理私钥口令（passphrase）
    node_key_passphrase = ""
    if key_mode == "universal":
        # 状态3：统一口令（交互输入，可能为空的字符串）
        node_key_passphrase = universal_key_passphrase
    else:
        # 状态1/状态2：CSV 第6列优先，缺省用全局配置
        passphrase_raw = row[5].strip() if len(row) > 5 else ""
        if passphrase_raw:
            pp_path = resolve_credential_path(passphrase_raw, config.account.secret_dir)
            node_key_passphrase = _read_credential(pp_path)
        elif key_passphrase:
            # 全局配置（已在文件开头解码）
            node_key_passphrase = key_passphrase

    # 出错返回错误消息，成功返回节点信息（二选一）
    if errors:
        return None, f"行 {idx} (IP: {ip if ip else '空值'}): - {'，'.join(errors)}"
    return {
        "ip": ip,
        "port": port,
        "user": user,
        "password": password,
        "key_content": key_content,
        "key_passphrase": node_key_passphrase,
    }, None


def _resolve_port(port_raw, default_port, mem, idx, total_nodes, ip, disinteractive) -> Tuple[Union[int, str], List[str]]:
    """端口字段补全：CSV > config > 输入记忆 > 交互输入
    （端口可能为 int（CSV/配置数字）或 str（交互输入），与原行为一致）"""
    errors = []
    port = port_raw.strip() if port_raw else ""
    if port:  # CSV中有值，最高优先级
        if not port.isdigit() or not (1 <= int(port) <= 65535):
            errors.append("端口必须是1-65535之间的整数，当前值为：" + port)
        else:
            port = int(port)
    elif default_port and default_port != "None":  # 使用配置的默认值
        port = default_port
        # 验证配置的默认端口
        if not str(port).isdigit() or not (1 <= int(port) <= 65535):
            errors.append("配置文件中的默认端口格式错误")
    elif mem.port_use_input:  # 使用之前用户输入的值
        port = mem.port_input_value
    else:  # 请求用户输入新值
        while True:
            try:
                input_port = input(
                    f"行 {idx} (IP: {ip if ip else '空值'}): 端口为空，请输入端口号: "
                )
                if input_port.isdigit() and 1 <= int(input_port) <= 65535:
                    port = input_port
                    # 询问是否将此端口号应用于所有后续端口为空的节点
                    if not mem.port_use_input and idx < total_nodes:
                        if get_user_confirmation(
                            f"\n{color.COLOR_YELLOW}是否将此端口号应用于所有后续端口为空的节点？{color.COLOR_RESET}",
                            yorn=True,
                            disinteractive=disinteractive,
                        ):
                            mem.port_use_input = True
                            mem.port_input_value = int(port)
                    break
                else:
                    print("端口必须是1-65535之间的整数")
            except (KeyboardInterrupt, EOFError):  # EOFError：管道/重定向输入结束时同样优雅退出
                print("\n用户取消输入")
                sys.exit(1)
    return port, errors


def _resolve_user(user_raw, default_user, mem, idx, total_nodes, ip, disinteractive) -> str:
    """用户名字段补全：CSV > config > 输入记忆 > 交互输入"""
    user = user_raw.strip() if user_raw else ""
    if user:  # CSV中有值，最高优先级
        return user
    if default_user and default_user != "None":  # 使用配置的默认值
        return default_user
    if mem.user_use_input:  # 使用之前用户输入的值
        return mem.user_input_value
    # 请求用户输入新值
    while True:
        try:
            input_user = input(
                f"行 {idx} (IP: {ip if ip else '空值'}): 用户名为空，请输入用户名: "
            )
            if input_user.strip():
                user = input_user.strip()
                # 询问是否将此用户名应用于所有后续用户为空的节点
                if not mem.user_use_input and idx < total_nodes:
                    if get_user_confirmation(
                        f"\n{color.COLOR_YELLOW}是否将此用户名应用于所有后续用户为空的节点？{color.COLOR_RESET}",
                        yorn=True,
                        disinteractive=disinteractive,
                    ):
                        mem.user_use_input = True
                        mem.user_input_value = user
                return user
            else:
                print("用户名不能为空")
        except (KeyboardInterrupt, EOFError):  # EOFError：管道/重定向输入结束时同样优雅退出
            print("\n用户取消输入")
            sys.exit(1)


def _resolve_password(password_raw, config, has_key, mem, idx, total_nodes, ip, disinteractive) -> str:
    """密码字段补全：CSV > config > 密钥认证 > 输入记忆 > 交互输入"""
    password = password_raw.strip() if password_raw else ""
    if password:  # CSV中有值，最高优先级
        password_path = resolve_credential_path(password, config.account.secret_dir)
        return _read_credential(password_path)
    if config.account.password and config.account.password != "None":  # 使用配置的默认值
        # 读取密码文件内容并解码base64（预检查已验证文件有效性）
        return _read_credential(config.account.password)
    if has_key:  # 仅使用密钥认证，无需密码
        return ""
    if mem.password_use_input:  # 使用之前用户输入的值
        return mem.password_input_value
    # 请求用户输入新值（输出信息时才导入getpass模块，优化不必要的模块导入）
    import getpass
    while True:
        try:
            input_password = getpass.getpass(
                f"行 {idx} (IP: {ip if ip else '空值'}): 密码为空，请输入密码: "
            )
            if input_password:
                password = input_password
                # 询问是否将此密码应用于所有后续密码为空的节点
                if not mem.password_use_input and idx < total_nodes:
                    if get_user_confirmation(
                        f"\n{color.COLOR_YELLOW}是否将此密码应用于所有后续密码为空的节点？{color.COLOR_RESET}",
                        yorn=True,
                        disinteractive=disinteractive,
                    ):
                        mem.password_use_input = True
                        mem.password_input_value = password
                return password
            else:
                print("密码不能为空，请重新输入")
        except (KeyboardInterrupt, EOFError):  # EOFError：管道/重定向输入结束时同样优雅退出
            print("\n用户取消输入")
            sys.exit(1)
