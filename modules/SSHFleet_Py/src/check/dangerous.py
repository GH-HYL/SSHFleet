# -*- coding: utf-8 -*-
# SSHFleet 危险命令检查模块
#
# 职责：把待下发的命令内容分类成「风险等级 + 命中规则」，用于提醒使用者（ADR-0011）。
# 解析链在 src/check/shell_tokenize.py，本模块只做判定与交互。

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

# rm 旗标条件取值
_VALID_REQUIRES = ("recursive", "force", "no_preserve_root")

# rm 目标条件取值
_VALID_TARGETS = ("root", "system_dir")

# 规则数量下限（防整份文件被误删/清空）
_MIN_RULES = 10


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


def check_dangerous_dict(config: Dict):
    """校验规则文件结构合法性（启动即校验，改错了立刻指名报错）

    参数：
        config: 规则文件解析结果（mapping）
    """
    if not config:
        _exit_with("check_dangerous_dict", "危险命令检测规则为空，程序退出！")

    if not isinstance(config, dict):
        _exit_with(
            "check_dangerous_dict",
            "危险命令检测规则结构非法（应为 mapping，含 rm_rules / commands），程序退出！",
        )

    rm_rules = config.get("rm_rules") or []
    cmd_rules = config.get("commands") or []

    if not isinstance(rm_rules, list) or not isinstance(cmd_rules, list):
        _exit_with(
            "check_dangerous_dict", "rm_rules / commands 必须为列表，程序退出！"
        )

    total = len(rm_rules) + len(cmd_rules)
    if total < _MIN_RULES:
        _exit_with(
            "check_dangerous_dict",
            f"危险命令检测规则不足（当前{total}条，需要至少{_MIN_RULES}条），程序退出！",
        )

    system_dirs = (config.get("path_classes") or {}).get("system_dir")
    if not system_dirs:
        _exit_with(
            "check_dangerous_dict",
            "路径分级清单 path_classes.system_dir 为空，程序退出！",
        )

    # rm 决策表校验
    for idx, rule in enumerate(rm_rules, 1):
        if not isinstance(rule, dict):
            _exit_with("check_dangerous_dict", f"rm_rules 第{idx}条结构非法，程序退出！")
        label = rule.get("name") or f"第{idx}条"
        for field in ("name", "example", "risk_level"):
            if not rule.get(field):
                _exit_with(
                    "check_dangerous_dict",
                    f"rm_rules {label} 缺少字段或字段为空：'{field}'，程序退出！",
                )
        if rule["risk_level"] not in _RISK_ORDER:
            _exit_with(
                "check_dangerous_dict",
                f"rm_rules {label} 的 risk_level 非法：'{rule['risk_level']}'"
                f"（仅支持 {'/'.join(_RISK_ORDER)}），程序退出！",
            )
        for req in rule.get("requires") or []:
            if req not in _VALID_REQUIRES:
                _exit_with(
                    "check_dangerous_dict",
                    f"rm_rules {label} 的 requires 取值非法：'{req}'"
                    f"（仅支持 {'/'.join(_VALID_REQUIRES)}），程序退出！",
                )
        if rule.get("targets") and rule["targets"] not in _VALID_TARGETS:
            _exit_with(
                "check_dangerous_dict",
                f"rm_rules {label} 的 targets 取值非法：'{rule['targets']}'"
                f"（仅支持 {'/'.join(_VALID_TARGETS)}），程序退出！",
            )
        for field in ("targets_prefix", "targets_suffix"):
            if rule.get(field) and not isinstance(rule[field], list):
                _exit_with(
                    "check_dangerous_dict",
                    f"rm_rules {label} 的 {field} 必须为列表，程序退出！",
                )

    # 其他命令规则校验
    for idx, rule in enumerate(cmd_rules, 1):
        if not isinstance(rule, dict):
            _exit_with("check_dangerous_dict", f"commands 第{idx}条结构非法，程序退出！")
        label = rule.get("name") or f"第{idx}条"
        for field in ("name", "example", "risk_level", "regex"):
            if not rule.get(field):
                _exit_with(
                    "check_dangerous_dict",
                    f"commands {label} 缺少字段或字段为空：'{field}'，程序退出！",
                )
        if rule["risk_level"] not in _RISK_ORDER:
            _exit_with(
                "check_dangerous_dict",
                f"commands {label} 的 risk_level 非法：'{rule['risk_level']}'"
                f"（仅支持 {'/'.join(_RISK_ORDER)}），程序退出！",
            )
        try:
            re.compile(rule["regex"])
        except re.error as e:
            _exit_with(
                "check_dangerous_dict",
                f"commands {label} 的正则无法编译：{e}，程序退出！",
            )


def _top_segment(path: str) -> str:
    """取绝对路径的首段目录名：/etc/nginx → etc"""
    trimmed = path.strip().strip("/")
    return trimmed.split("/")[0] if trimmed else ""


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


def _classify_target(raw: str, system_dirs: set) -> Tuple[str, str]:
    """目标路径分类。

    返回：
        (kind, normalized)：kind ∈ root / system / business / relative / dynamic
        dynamic = 含变量或命令替换，静态不可判定（只提醒不拦）
    """
    if "$" in raw or "`" in raw:
        return "dynamic", raw

    norm = _normalize_path(raw)
    if not norm.startswith("/"):
        return "relative", norm
    if norm in ("/", "/*"):
        return "root", norm

    return ("system" if _top_segment(norm) in system_dirs else "business"), norm


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


def _parse_rm_args(args: List[str]) -> Tuple[set, List[str]]:
    """拆分 rm 的参数为 (旗标集合, 目标列表)，支持 -- 之后全为目标"""
    flags: set = set()
    targets: List[str] = []
    end_of_flags = False

    for tok in args:
        if not end_of_flags and tok == "--":
            end_of_flags = True
            continue
        if not end_of_flags and tok.startswith("-") and len(tok) > 1:
            flags |= _parse_flags(tok)
            continue
        targets.append(tok)

    return flags, targets


def _rm_rule_matches(rule: Dict, flags: set, classified: List[Tuple[str, str]]) -> bool:
    """判断单条 rm 规则是否命中当前命令"""
    requires = rule.get("requires") or []
    if any(req not in flags for req in requires):
        return False

    kinds = {kind for kind, _ in classified}
    target_type = rule.get("targets")
    prefixes = rule.get("targets_prefix") or []
    suffixes = rule.get("targets_suffix") or []

    # 无任何条件的规则会命中所有 rm，视为配置失误，不参与匹配
    if not requires and not target_type and not prefixes and not suffixes:
        return False

    if target_type == "root" and "root" not in kinds:
        return False
    if target_type == "system_dir" and "system" not in kinds:
        return False

    if prefixes and not any(
        norm == p.rstrip("/") or norm.startswith(p)
        for _, norm in classified
        for p in prefixes
    ):
        return False
    if suffixes and not any(
        norm.endswith(s) for _, norm in classified for s in suffixes
    ):
        return False

    return True


def _judge_rm(args: List[str], rules: List[Dict], system_dirs: set) -> Optional[Dict]:
    """判定 rm 命令，返回命中的最高风险规则（无命中返回 None）"""
    flags, targets = _parse_rm_args(args)
    if not flags and not targets:
        return None

    classified = [_classify_target(t, system_dirs) for t in targets]

    best: Optional[Dict] = None
    for rule in rules:
        if not _rm_rule_matches(rule, flags, classified):
            continue
        if best is None or _RISK_ORDER[rule["risk_level"]] < _RISK_ORDER[best["risk_level"]]:
            best = rule
    return best


def _judge_segment(
    tokens: List[str],
    text: str,
    rm_rules: List[Dict],
    cmd_rules: List[Tuple[Dict, "re.Pattern"]],
    system_dirs: set,
) -> Optional[Dict]:
    """判定单个命令段，返回命中的最高风险规则"""
    if not tokens:
        return None

    if tokens[0] == "rm":
        return _judge_rm(tokens[1:], rm_rules, system_dirs)

    best: Optional[Dict] = None
    for rule, pattern in cmd_rules:
        if not pattern.search(text):
            continue
        if best is None or _RISK_ORDER[rule["risk_level"]] < _RISK_ORDER[best["risk_level"]]:
            best = rule
    return best


def analyze_lines(
    lines: List[str], config: Dict, is_script: bool = False, script_path: str = ""
) -> List[Dict]:
    """分析若干行命令，返回全部命中项（纯函数，无交互、无退出）

    返回：
        [{line, content, pattern, is_script, script_path}]
    """
    syntax = config.get("syntax") or {}
    rm_rules = [r for r in (config.get("rm_rules") or []) if _rule_enabled(r)]
    cmd_rules = [
        (r, re.compile(r["regex"], re.IGNORECASE))
        for r in (config.get("commands") or [])
        if _rule_enabled(r)
    ]
    system_dirs = {
        _top_segment(p)
        for p in ((config.get("path_classes") or {}).get("system_dir") or [])
    }

    matches: List[Dict] = []
    for line_num, line in enumerate(lines, 1):
        stripped = line.strip()

        # 跳过注释行和空行
        if not stripped or stripped.startswith("#"):
            continue

        for tokens, text in parse_line(stripped, syntax):
            rule = _judge_segment(tokens, text, rm_rules, cmd_rules, system_dirs)
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


def check_dangerous_patterns(args, dangerous_keywords: Dict):
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
