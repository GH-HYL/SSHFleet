# -*- coding: utf-8 -*-
# SSHFleet 危险命令检查模块
#
# 职责：把待下发的命令内容分类成「风险等级 + 命中规则」，用于提醒使用者（ADR-0011）。
#
# 规则文件是平级正则列表（name / example / regex / risk_level）：
#   正则对该命令的任意位置匹配（不限定开头），命中多条取最高风险。
# 解析链在 src/check/shell_tokenize.py：切分命令段、剥除包装命令、拍平 rm 旗标，
# 使「一条正则」能同时覆盖 -rf / -r -f / -R / --recursive 等写法。

import re
import sys
from typing import Dict, List, Optional, Tuple

import src.common.constants as color

from src.check.shell_tokenize import parse_line
from src.common.error_handler import error_and_exit_handling_decorator, print_error_information_and_exit
from src.common.text_utils import display_width
from src.input.interaction import get_user_confirmation

# 风险级别排序：forbidden 最高
_RISK_ORDER = {"forbidden": 0, "high": 1, "medium": 2, "low": 3}

# 规则数量下限（防整份文件被误删/清空）
_MIN_RULES = 10

# rm 旗标归一顺序与写法（归一化后固定成这个次序，正则才能写短）
_FLAG_ORDER = (("recursive", "-r"), ("force", "-f"), ("no_preserve_root", "--no-preserve-root"))


def _rule_enabled(rule: Dict) -> bool:
    """规则是否启用（缺省启用；enabled: false 即禁用）"""
    return rule.get("enabled", True) is not False


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


def _exit_with(label: str, message: str):
    """规则校验收尾：打印并退出"""
    print_error_information_and_exit(label, message)
    raise SystemExit(1)


def check_dangerous_dict(config: List):
    """校验规则文件合法性（启动即校验，改错了立刻指名报错）

    参数：
        config: 规则文件解析结果（规则列表）
    """
    if not config:
        _exit_with("check_dangerous_dict", "危险命令检测规则为空，程序退出！")

    if not isinstance(config, list):
        _exit_with(
            "check_dangerous_dict",
            "危险命令检测规则结构非法（应为规则列表），程序退出！",
        )

    if len(config) < _MIN_RULES:
        _exit_with(
            "check_dangerous_dict",
            f"危险命令检测规则不足（当前{len(config)}条，需要至少{_MIN_RULES}条），程序退出！",
        )

    for idx, rule in enumerate(config, 1):
        if not isinstance(rule, dict):
            _exit_with("check_dangerous_dict", f"rules 第{idx}条结构非法，程序退出！")
        label = rule.get("name") or f"第{idx}条"
        for field in ("name", "example", "risk_level", "regex"):
            if not rule.get(field):
                _exit_with(
                    "check_dangerous_dict",
                    f"rules {label} 缺少字段或字段为空：'{field}'，程序退出！",
                )
        if rule["risk_level"] not in _RISK_ORDER:
            _exit_with(
                "check_dangerous_dict",
                f"rules {label} 的 risk_level 非法：'{rule['risk_level']}'"
                f"（仅支持 {'/'.join(_RISK_ORDER)}），程序退出！",
            )
        try:
            re.compile(rule["regex"])
        except re.error as e:
            _exit_with(
                "check_dangerous_dict",
                f"rules {label} 的正则无法编译：{e}，程序退出！",
            )


def _normalize_path(raw: str) -> str:
    """路径归一化：合并重复斜杠、去尾斜杠、词法解析 . 与 ..（相对路径原样返回）"""
    path = re.sub(r"/{2,}", "/", raw)
    if not path.startswith("/"):
        return path

    parts: List[str] = []
    for seg in path.split("/"):
        if seg in ("", "."):
            continue
        if seg == "..":
            if parts:
                parts.pop()
            continue
        parts.append(seg)
    return "/" + "/".join(parts) if parts else "/"


def _parse_flags(token: str) -> set:
    """解析单个 rm 旗标 token，归一为 recursive / force / no_preserve_root"""
    flags = set()

    if token == "--no-preserve-root":
        flags.add("no_preserve_root")
        return flags

    if token.startswith("--"):
        name = token[2:]
        if name == "recursive":
            flags.add("recursive")
        elif name == "force":
            flags.add("force")
        return flags

    for ch in token[1:]:
        if ch in ("r", "R"):
            flags.add("recursive")
        elif ch == "f":
            flags.add("force")
    return flags


def _normalize_for_match(tokens: List[str]) -> str:
    """把命令段的 token 归一成用于正则匹配的字符串。

    - rm：旗标拍平为固定次序（-r / -f / --no-preserve-root），目标路径归一化
    - 其他命令：token 原样以单空格连接
    - 末尾补一个空格：让「\\s」能作为最后一个 token 的边界
    """
    if not tokens:
        return ""

    if tokens[0] != "rm":
        return " ".join(tokens) + " "

    flags: set = set()
    targets: List[str] = []
    end_of_flags = False

    for tok in tokens[1:]:
        if not end_of_flags and tok == "--":
            end_of_flags = True
            continue
        if not end_of_flags and tok.startswith("-") and len(tok) > 1:
            flags |= _parse_flags(tok)
            continue
        targets.append(_normalize_path(tok))

    parts = ["rm"]
    for name, text in _FLAG_ORDER:
        if name in flags:
            parts.append(text)
    parts.extend(targets)
    return " ".join(parts) + " "


def _judge_segment(
    normalized: str, rules: List[Tuple[Dict, "re.Pattern"]]
) -> Optional[Dict]:
    """判定单个命令段，返回命中的最高风险规则（无命中返回 None）"""
    if not normalized:
        return None

    best: Optional[Dict] = None
    for rule, pattern in rules:
        if not pattern.search(normalized):
            continue
        if best is None or _RISK_ORDER[rule["risk_level"]] < _RISK_ORDER[best["risk_level"]]:
            best = rule
    return best


def analyze_lines(
    lines: List[str], config: List, is_script: bool = False, script_path: str = ""
) -> List[Dict]:
    """分析若干行命令，返回全部命中项（纯函数，无交互、无退出）

    返回：
        [{line, content, pattern, is_script, script_path}]
    """
    rules = [
        (r, re.compile(r["regex"], re.IGNORECASE))
        for r in (config or [])
        if isinstance(r, dict) and _rule_enabled(r)
    ]

    matches: List[Dict] = []
    for line_num, line in enumerate(lines, 1):
        stripped = line.strip()

        # 跳过注释行和空行
        if not stripped or stripped.startswith("#"):
            continue

        for tokens, text in parse_line(stripped):
            rule = _judge_segment(_normalize_for_match(tokens), rules)
            if not rule:
                continue
            matches.append(
                {
                    "line": line_num,
                    "content": text,
                    "pattern": rule,
                    "is_script": is_script,
                    "script_path": script_path if is_script else None,
                }
            )

    return matches


def check_dangerous_patterns(args, dangerous_keywords: List):
    """检查命令或脚本内容的危险模式（交互与退出策略保持原有行为）"""

    # 与确认模块同一约定：从 args 统一读取非交互标记（ADR/确认链唯一来源）
    disinteractive = getattr(args, "disinteractive", False)

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

    all_matches = analyze_lines(lines, dangerous_keywords, is_script, script_path)

    # 如果没有匹配，直接返回
    if not all_matches:
        return

    # 取最高风险匹配项
    highest_risk_match = min(
        all_matches, key=lambda x: _RISK_ORDER.get(x["pattern"]["risk_level"], 99)
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
    """打印危险命令警告信息（合并函数）

    框内对齐按显示宽度计算（中文占 2 宽），避免中文内容导致边框错位。
    """

    if not matches:
        return

    if is_forbidden:
        title = "🚫  发现禁止命令 🚫"
        footer = "此命令被禁止执行，程序将立即退出！"
    else:
        title = "⚠️  发现危险命令 ⚠️"
        footer = "是否继续执行？这可能会带来安全风险！"

    # 框内宽度（不含两侧 ║），与顶部 ╔══...╗ 的 ═ 数量一致
    inner_width = 56
    bar = "═" * inner_width

    def pad(text: str) -> str:
        """按显示宽度右补空格到 inner_width"""
        return text + " " * max(0, inner_width - display_width(text))

    def center(text: str) -> str:
        """按显示宽度居中到 inner_width"""
        remain = max(0, inner_width - display_width(text))
        left = remain // 2
        return " " * left + text + " " * (remain - left)

    # 统一按最高风险级别着色（调用方传入的就是最高风险匹配项）
    risk_level = min(
        (m["pattern"]["risk_level"] for m in matches),
        key=lambda x: _RISK_ORDER.get(x, 99),
    )
    risk_color = (
        color.COLOR_RED
        if risk_level in ("forbidden", "high")
        else color.COLOR_YELLOW
        if risk_level == "medium"
        else color.COLOR_WHITE
    )

    warning_msg = [
        f"{risk_color}\n╔{bar}╗",
        f"{risk_color}║{center(title)}║",
        f"{risk_color}╠{bar}╣",
    ]
    for match in matches:
        source_info = (
            f"来源: {'脚本: ' + match['script_path'] if match['is_script'] else '命令'}"
        )
        warning_msg.extend(
            [
                f"{risk_color}║{pad('    ' + source_info)}║",
                f"{risk_color}║{pad('    行号: ' + str(match['line']))}║",
                f"{risk_color}║{pad('    内容: ' + match['content'][:46])}║",
                f"{risk_color}║{pad('    分类: ' + match['pattern']['name'][:46])}║",
                f"{risk_color}║{pad('    级别: ' + risk_level.upper())}║",
                f"{risk_color}╠{bar}╢",
            ]
        )

    warning_msg.extend(
        [
            f"{risk_color}║{center(footer)}║",
            f"{risk_color}╚{bar}╝",
        ]
    )

    # 末尾必须重置颜色，否则终端颜色状态残留（尤其 forbidden 直接退出时，后续输入会一直保持红色）
    print("\n".join(warning_msg) + color.COLOR_RESET)
