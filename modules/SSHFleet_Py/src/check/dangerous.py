# -*- coding: utf-8 -*-
# SSHFleet 危险命令检查模块

import re
import sys
from typing import List

import src.common.constants as color

from src.common.error_handler import error_and_exit_handling_decorator, print_error_information_and_exit
from src.input.interaction import get_user_confirmation

# 命令分隔符（&&/|| 双字符在前，避免 | 或 & 单字符先拆导致组合符被拆坏）
_SEPARATORS_RE = re.compile(r"&&|\|\||[;&|]")


def _split_by_separators(line: str) -> List[str]:
    """
    按命令分隔符切分一行命令，支持一行内多种分隔符嵌套。

    示例：
        "echo a; rm -rf / && chmod 777 x"
        -> ["echo a", "rm -rf /", "chmod 777 x"]
    """
    parts = _SEPARATORS_RE.split(line)
    return [p.strip() for p in parts if p.strip()]


def _strip_command_prefixes(cmd: str) -> str:
    """
    去除命令开头的常见前缀（支持多前缀叠加，如 "sudo nohup chmod ..."）。

    示例：
        "sudo nohup chmod 777 /x" -> "chmod 777 /x"
    """
    prefixes = ["sudo", "time", "nohup", "setsid", "stdbuf"]
    changed = True
    while changed:
        changed = False
        for prefix in prefixes:
            if cmd.startswith(prefix + " "):
                cmd = cmd[len(prefix):].strip()
                changed = True
                break  # 去除一个前缀后重新扫描，支持叠加
    return cmd


def remove_command_first_last_same_symbol(cmd_str):
    """
    功能：
        去除命令字符串首尾相同的特殊符号（引号等包裹符号）

    说明：
        危险检测的锚定规则（^ 开头）对带引号包裹的命令会漏检，
        如 'rm -rf /' 需先剥壳为 rm -rf / 才能命中锚定规则；
        shell 只剥除最外层引号，内层包裹符号会原样进入命令字符串，由这里处理。

    参数：
        cmd_str: 命令字符串

    返回：
        removed_symbol: 被移除的特殊符号（无则 None）
        cmd_str: 处理后的命令字符串
    """

    # 特殊符号黑名单，以下符号不移除
    forbidden_chars = r"^$*+?.()[]{}|\/"

    # 命令大于一个字符、首尾相同、首尾不是字母或数字、首尾不在特殊符号黑名单中
    if (
        len(cmd_str) > 1
        and cmd_str[0] == cmd_str[-1]
        and not cmd_str[0].isalnum()
        and cmd_str[0] not in forbidden_chars
    ):

        removed_symbol = cmd_str[0]  # 记录被移除的符号
        cmd_str = cmd_str[1:-1]  # 实际移除操作
        return removed_symbol, cmd_str
    else:
        return None, cmd_str


@error_and_exit_handling_decorator(
    "check_dangerous_content", "危险关键词内容检查失败"
)
def check_dangerous_content(args, dangerous_keywords: List):

    # 执行危险关键词内容检查
    try:
        check_dangerous_dict(dangerous_keywords)
    except Exception as e:
        print_error_information_and_exit(
            "check_dangerous_dict", f" 危险命令筛查失败: {str(e)}"
        )

    # 执行危险命令检查
    try:
        if args.c or args.s:
            check_dangerous_patterns(args, dangerous_keywords)
    except Exception as e:
        print_error_information_and_exit(
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
        print_error_information_and_exit(
            "check_dangerous_dict", "危险命令检测规则为空，程序退出！"
        )

    # 2. 检查规则数量是否足够（至少10行）
    if len(dangerous_patterns) < 10:
        print_error_information_and_exit(
            "check_dangerous_dict",
            f"危险命令检测规则不足（当前{len(dangerous_patterns)}条，需要至少10条），程序退出！",
        )

    # 3. 检查规则完整性（每个规则必须包含必要的字段）
    required_fields = ["name", "example", "regex", "risk_level"]
    for i, pattern in enumerate(dangerous_patterns):
        for field in required_fields:
            if field not in pattern:
                print_error_information_and_exit(
                    "check_dangerous_dict",
                    f"危险命令检测规则不完整（第{i+1}条缺少字段'{field}'），程序退出！",
                )
            if not pattern[field]:
                print_error_information_and_exit(
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
            print_error_information_and_exit(
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

        # 第零步：剥除首尾相同的包裹符号（引号等），防止引号包裹绕过锚定规则
        stripped_line = remove_command_first_last_same_symbol(stripped_line)[1]

        # 第一步：按命令分隔符分割（支持一行内多种分隔符嵌套）
        # 用正则一次性切分 ; && || | &（&&/|| 优先于单个 &/|，避免拆错）
        commands = _split_by_separators(stripped_line)

        # 第二步：处理每条命令的前缀（如 sudo，支持多前缀叠加）
        final_commands = []
        for cmd in commands:
            cmd = _strip_command_prefixes(cmd)

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
        if not get_user_confirmation(
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

    # 末尾必须重置颜色，否则终端颜色状态残留（尤其 forbidden 直接退出时，后续输入会一直保持红色）
    print("\n".join(warning_msg) + color.COLOR_RESET)
