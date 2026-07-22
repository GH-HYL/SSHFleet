# -*- coding: utf-8 -*-
# SSHFleet 检查文件
# 该文件负责检查文件是否存在、是否可写等操作

import os

# 系统或第三方模块
import re
import sys
from typing import List
from posixpath import join as posix_join

import src.color as color

# 源代码模块
from src.yaml import SSHFleetConfig
import src.utils as utils


@utils.error_and_exit_handling_decorator(
    "check_dangerous_content", "危险字典内容检查失败"
)
def check_dangerous_content(args, dangerous_keywords: List):

    # 执行危险字典内容检查
    try:
        check_dangerous_dict(dangerous_keywords)
    except Exception as e:
        utils.print_error_information_and_exit(
            "check_dangerous_dict", f" 危险命令筛查失败: {str(e)}"
        )

    # 执行危险命令检查
    try:
        if args.c or args.s:
            check_dangerous_patterns(args, dangerous_keywords)
    except Exception as e:
        utils.print_error_information_and_exit(
            "check_dangerous_patterns", f" 危险命令检查失败: {str(e)}"
        )


@utils.error_and_exit_handling_decorator("check_arguments", "参数合规性检查失败")
def check_arguments(args):
    """检查命令行参数的有效性"""

    # 检查互斥执行模式参数
    lock_args = [args.c, args.s, args.u, args.z]
    lock_args_count = sum(1 for arg in lock_args if arg)
    if lock_args_count != 1:
        utils.print_error_information_and_exit(
            "check_arguments", " 执行模式参数：-c、-s、-u、-z 互斥，只能指定一个"
        )

    # 检查 -p 参数
    if args.p:
        if args.u:
            if not args.p.startswith("/"):
                utils.print_error_information_and_exit(
                    "check_arguments",
                    f" 上传模式：-p 参数指定的上传目录必须是绝对路径，当前值：{args.p}",
                )
            if not args.p.endswith("/"):
                utils.print_error_information_and_exit(
                    "check_arguments",
                    f" 上传模式：-u 是目录的情况下 -p 参数指定的必须是目录（以/结尾），-p 当前值：{args.p}",
                )

        elif args.c or args.s:
            # -p 不能与 -c 或 -s 同时使用
            utils.print_error_information_and_exit(
                "check_arguments",
                " -p 参数不能搭配 -c 或 -s 使用，请单独使用 -u 参数指定上传文件或目录后再使用 -c 或 -s",
            )

    # 检查 -u 参数
    if args.u:
        if not args.p:
            utils.print_error_information_and_exit(
                "check_arguments", " -u 参数必须搭配 -p 参数使用"
            )
        if not os.path.exists(args.u):
            utils.print_error_information_and_exit(
                "check_arguments", f" -u 参数指定的上传文件或目录不存在：{args.u}"
            )
        if os.path.islink(args.u):
            utils.print_error_information_and_exit(
                "check_arguments",
                f" -u 参数指定的上传文件或目录是符号链接，不能上传：{args.u}",
            )
        if os.path.isdir(args.u):
            # 递归检查整个目录树
            has_real_file = False
            for root, dirs, files in os.walk(args.u):
                for file in files:
                    file_path = os.path.join(root, file)
                    if os.path.isfile(file_path) and not os.path.islink(file_path):
                        has_real_file = True
                        break  # 找到一个就退出内层循环
                if has_real_file:
                    break  # 找到一个就退出外层循环

            if not has_real_file:
                utils.print_error_information_and_exit(
                    "check_arguments",
                    f"-u 参数指定的上传目录及其所有子目录中都没有真正的文件（只有符号链接）：{args.u}",
                )

    # # 检查 -e 参数
    # if args.e:
    #     if args.u or args.d:
    #         utils.print_error_information_and_exit(
    #             "check_arguments", " -e 参数不能搭配 -u 或 -d 参数使用"
    #         )
    #     env_pattern = r'^[a-zA-Z_]+=[\'"]?[a-zA-Z0-9_./-]+[\'"]?$'  # key=value 格式
    #     if not re.match(env_pattern, args.e):
    #         utils.print_error_information_and_exit(
    #             "check_arguments",
    #             " -e 参数格式错误，必须是 key=value 格式 值可以是单引号或双引号包裹的字符串",
    #         )

    # 检查 -m 参数
    if args.m:
        if args.m not in ["direct", "sudo"]:
            utils.print_error_information_and_exit(
                "check_arguments",
                f" -m 参数必须是 'direct' 或 'sudo'，当前值：{args.m}",
            )

    # 检查 -c 参数
    if args.c:
        if not args.c.strip():
            utils.print_error_information_and_exit(
                "check_arguments", " -c 参数不能为空，请提供要执行的命令"
            )

        first_word = re.findall(r"^[^a-zA-Z]*([a-zA-Z]+)", args.c.strip())
        if first_word and first_word[0] in ["for", "while", "until", "if", "case"]:
            utils.print_error_information_and_exit(
                "check_arguments", " -c 参数不兼容执行循环的命令，请使用脚本模式执行"
            )

    # 检查 -s 参数
    if args.s:
        if os.path.isdir(args.s):
            utils.print_error_information_and_exit(
                "check_arguments", f" -s 参数指定的路径是目录，不是脚本文件：{args.s}"
            )
        script_path = args.s
        if os.path.getsize(script_path) == 0:
            utils.print_error_information_and_exit(
                "check_arguments", f" -s 参数指定的脚本文件为空：{script_path}"
            )
        if not script_path.endswith((".sh", ".py")):
            utils.print_error_information_and_exit(
                "check_arguments",
                f" -s 参数指定的脚本文件扩展名必须是 .sh 或 .py，当前值：{script_path}",
            )
        check_script_file(script_path)

    # 检查 -f 参数
    if args.f:
        if not os.path.exists(args.f):
            utils.print_error_information_and_exit(
                "check_arguments", f" -f 参数指定的 CSV 文件不存在：{args.f}"
            )
        if os.path.getsize(args.f) == 0:
            utils.print_error_information_and_exit(
                "check_arguments", f" -f 参数指定的 CSV 文件为空：{args.f}"
            )
        # 不能是二进制文件
        with open(args.f, "rb") as f:
            if b"\x00" in f.read(1024):
                utils.print_error_information_and_exit(
                    "check_arguments", f" 错误: {args.f} 是二进制文件"
                )

    # 检查 -n 参数
    if args.n:
        if not isinstance(args.n, int) or args.n <= 0:
            utils.print_error_information_and_exit(
                "check_arguments",
                f" -n 参数格式错误，并发连接数必须是正整数，当前值：{args.n}",
            )

    # # 检查 -k 参数
    # if args.k:
    #     if not os.path.isfile(args.k):
    #         utils.print_error_information_and_exit(
    #             "check_arguments", f" -k 指向的秘钥文件不存在，请检查路径：{args.k}"
    #         )

    # 检查 --disinteractive 参数
    if args.disinteractive and args.z:
        utils.print_error_information_and_exit(
            "check_arguments", " --disinteractive 参数不能与 -z 一起使用"
        )

    # 检查 -t 参数, 必须是 int 类型
    if args.t:
        if not isinstance(args.t, int) or args.t <= 0:
            utils.print_error_information_and_exit(
                "check_arguments",
                f" -t 参数格式错误，命令或传输超时时间必须是正整数，当前值：{args.t}",
            )

    if args.T:
        if not isinstance(args.T, int) or args.T <= 0:
            utils.print_error_information_and_exit(
                "check_arguments",
                f" -T 参数格式错误，连接超时时间必须是正整数，当前值：{args.T}",
            )


def check_script_file(script_path):
    """检查脚本文件内容的有效性"""

    # 脚本必须使用UTF-8编码（支持带BOM）
    with open(script_path, "rb") as f:
        raw_data = f.read()

        # 不能是二进制文件
        if b"\x00" in raw_data:
            utils.print_error_information_and_exit(
                "check_script_file", f" 错误: {script_path} 是二进制文件"
            )

        if raw_data.startswith(b"\xef\xbb\xbf"):
            # 有BOM，去掉BOM
            data = raw_data[3:]
        else:
            data = raw_data
        if not data.decode("utf-8", errors="ignore").encode("utf-8") == data:
            utils.print_error_information_and_exit(
                "check_script_file", f" 错误: {script_path} 不是UTF-8编码"
            )

        # 必须使用LF换行符(Unix格式)，如果不是，直接替换成LF并提示用户
        if b"\r\n" in data:
            with open(script_path, "wb") as f:
                f.write(data.replace(b"\r\n", b"\n"))
            from src.utils import tlog as _tlog
            _tlog.warning(f"{script_path} 的换行符已自动转换为LF格式")


@utils.error_and_exit_handling_decorator("check_files_exist", "检查代码文件存在性失败")
def check_files_exist(config: SSHFleetConfig) -> None:
    """检查所有代码文件必须存在"""
    current_dir = os.getcwd()

    code_files = [
        "src/config/SSHFleet.yaml",
        config.paths.jsons.dangerous_keywords,
        config.paths.jsons.error_keywords,
    ]

    # 循环检查文件，然后把所有缺失文件打印出来
    missing_files = []
    for code_file in code_files:
        if not os.path.exists(posix_join(current_dir, code_file)):
            missing_files.append(code_file)

    if missing_files:
        utils.print_error_information_and_exit(
            "check_files_exist", f" 代码文件缺失: {', '.join(missing_files)}"
        )


def check_dangerous_dict(dangerous_patterns: List):
    """
    功能：
        检查命令或脚本内容是否包含危险模式
        包含规则完整性检查和防篡改验证

    参数：
        dangerous_patterns: 危险命令检测规则列表
        validation_code: 验证码（可选）

    返回：
        None
    """

    # 1. 检查规则是否为空
    if not dangerous_patterns:
        utils.print_error_information_and_exit(
            "check_dangerous_dict", "危险命令检测规则为空，程序退出！"
        )

    # 2. 检查规则数量是否足够（至少10行）
    if len(dangerous_patterns) < 10:
        utils.print_error_information_and_exit(
            "check_dangerous_dict",
            f"危险命令检测规则不足（当前{len(dangerous_patterns)}条，需要至少10条），程序退出！",
        )

    # 3. 检查规则完整性（每个规则必须包含必要的字段）
    required_fields = ["name", "example", "regex", "risk_level"]
    for i, pattern in enumerate(dangerous_patterns):
        for field in required_fields:
            if field not in pattern:
                utils.print_error_information_and_exit(
                    "check_dangerous_dict",
                    f"危险命令检测规则不完整（第{i+1}条缺少字段'{field}'），程序退出！",
                )
            if not pattern[field]:
                utils.print_error_information_and_exit(
                    "check_dangerous_dict",
                    f"危险命令检测规则不完整（第{i+1}条字段'{field}'为空），程序退出！",
                )

    # 4. 校验危险模式规则是否被篡改（预留接口，暂未启用）


def check_dangerous_patterns(args, dangerous_keywords: List, disinteractive=False):
    """检查命令或脚本内容是否包含危险模式"""

    is_script = bool(args.s)
    script_path = args.s or ""

    # 检查是否是脚本文件路径
    if args.s:
        try:
            # 读取脚本文件内容
            with open(args.s, "r", encoding="utf-8") as f:
                lines = f.readlines()
        except Exception as e:
            utils.print_error_information_and_exit(
                "check_dangerous_patterns", f"读取脚本内容失败: {str(e)}"
            )
    else:
        lines = [args.c]

    # 预处理所有行：分割命令和处理前缀
    processed_lines = []
    for line_num, line_content in enumerate(lines, 1):
        stripped_line = line_content.strip()

        # 跳过注释行和空行
        if not stripped_line or stripped_line.startswith("#"):
            continue

        # 第一步：按命令分隔符分割
        commands = []
        temp_line = stripped_line

        # 处理所有可能的分隔符
        separators = [";", "&&", "||", "|", "&"]
        for sep in separators:
            if sep in temp_line:
                parts = temp_line.split(sep)
                # 将分隔符前后的部分分别处理
                for i, part in enumerate(parts):
                    if part.strip():  # 非空部分
                        commands.append(part.strip())
                break  # 一次只处理一种分隔符，避免复杂嵌套
        else:
            # 如果没有分隔符，整个作为一条命令
            commands = [temp_line]

        # 第二步：处理每条命令的前缀（如sudo）
        final_commands = []
        for cmd in commands:
            # 去除常见的前缀命令
            prefixes = ["sudo", "time", "nohup", "setsid", "stdbuf"]
            for prefix in prefixes:
                if cmd.startswith(prefix + " "):
                    # 去掉前缀和后面的空格
                    cmd = cmd[len(prefix) :].strip()
                    break  # 一次只去除一个前缀

            if cmd:  # 如果去除前缀后还有内容
                final_commands.append(cmd)

        # 将处理后的命令添加到最终行列表
        processed_lines.extend([(line_num, cmd) for cmd in final_commands])

    all_matches = []

    # 检查处理后的命令
    for line_info in processed_lines:
        line_num, command = line_info

        # 检查每个危险模式
        for pattern in dangerous_keywords:
            # 跳过注释掉的正则规则
            if pattern["regex"].strip().startswith("#"):
                continue

            # 使用正则表达式在命令中搜索匹配模式
            if re.search(pattern["regex"], command, re.IGNORECASE):
                match_info = {
                    "line": line_num,
                    "content": command,
                    "pattern": pattern,
                    "is_script": is_script,
                    "script_path": script_path if is_script else None,
                }
                all_matches.append(match_info)

    # 如果没有匹配，直接返回
    if not all_matches:
        return

    # 按风险级别排序：forbidden > high > medium > low
    risk_order = {"forbidden": 0, "high": 1, "medium": 2, "low": 3}
    highest_risk_match = min(
        all_matches, key=lambda x: risk_order.get(x["pattern"]["risk_level"], 99)
    )

    # 根据最高风险级别处理
    if highest_risk_match["pattern"]["risk_level"] == "forbidden":
        print_danger_warning([highest_risk_match], is_forbidden=True)
        sys.exit(1)
    elif not disinteractive:
        print_danger_warning([highest_risk_match], is_forbidden=False)

        # 获取用户确认
        if not utils.get_user_confirmation(
            f"\n{color.COLOR_YELLOW}已明确风险继续执行？{color.COLOR_RESET}", yorn=False
        ):
            print(f"{color.COLOR_YELLOW}操作已取消{color.COLOR_RESET}")
            sys.exit(1)


def print_danger_warning(matches, is_forbidden=False):
    """打印危险命令警告信息（合并函数）"""

    if not matches:
        return

    if is_forbidden:
        title = "🚫  发现禁止命令 🚫"
        footer = "此命令被禁止执行，程序将立即退出！"
    else:
        title = "⚠️  发现危险命令 ⚠️"
        footer = "是否继续执行？这可能会带来安全风险！"

    for match in matches:
        source_info = (
            f"来源: {'脚本: ' + match['script_path'] if match['is_script'] else '命令'}"
        )

        # 根据风险级别选择颜色
        risk_level = match["pattern"]["risk_level"]
        if risk_level == "forbidden":
            risk_color = color.COLOR_RED
        elif risk_level == "high":
            risk_color = color.COLOR_RED
        elif risk_level == "medium":
            risk_color = color.COLOR_YELLOW
        else:
            risk_color = color.COLOR_WHITE

        warning_msg = [
            f"{risk_color}\n╔════════════════════════════════════════════════════════════╗",
            f"{risk_color}║                  {title.center(15)}                   ",
            f"{risk_color}╠════════════════════════════════════════════════════════════╣",
        ]
        warning_msg.extend(
            [
                f"{risk_color}║    {risk_color}{source_info.ljust(55)}{risk_color}",
                f"{risk_color}║    {risk_color}行号: {match['line']}{' '*(53-len(str(match['line'])))}",
                f"{risk_color}║    内容:{color.COLOR_RED} {match['content'][:46]}{' '*(50-len(match['content'][:46]))}",
                f"{risk_color}║    {risk_color}分类: {match['pattern']['name'][:46]}{' '*(50-len(match['pattern']['name'][:46]))}",
                f"{risk_color}║    {risk_color}级别: {risk_level.upper()}{' '*(50-len(risk_level))}",
                f"{risk_color}╠════════════════════════════════════════════════════════════╢",
            ]
        )

    warning_msg.extend(
        [
            f"{risk_color}║ {risk_color}{footer.center(40)}",
            f"{risk_color}╚════════════════════════════════════════════════════════════╝",
        ]
    )

    print("\n".join(warning_msg))
