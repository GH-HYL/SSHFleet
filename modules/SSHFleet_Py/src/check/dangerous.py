# -*- coding: utf-8 -*-
# SSHFleet 危险命令检查模块

import re
import sys
from typing import List

import src.color as color

from src import utils


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
            f"\n{color.COLOR_YELLOW}已明确风险继续执行？{color.COLOR_RESET}",
            yorn=False,
            disinteractive=disinteractive,
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
