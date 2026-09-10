# -*- coding: utf-8 -*-
# SSHFleet 命令解析模块
#
# 职责：把一行命令文本解析成「以真实命令词开头的命令段」列表，供危险检测判定。
# 只做结构解析，不做风险判定——判定见 src/check/dangerous.py。
#
# 解析链：引号/替换感知切分 → 分词 → 重定向剥除 → 环境变量赋值与包装命令剥除
#        → 间接执行内部命令提取（递归）

import re
from typing import Dict, List, Tuple

# 环境变量赋值形式：NAME=value
_ASSIGN_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*=")

# 重定向：> >> < << &> &>> 及带 fd 前缀形式（2> 3>> 等）
_REDIR_RE = re.compile(r"^(?:\d*)(?:>>?|<<?)$|^&>>?$|^(?:\d*)>&(?:\d+|-)$")

# 单个 token 内的替换占位符（避免内部内容干扰外层判定）
_SUBST_PLACEHOLDER = "$(...)"
_BACKTICK_PLACEHOLDER = "`...`"


def _capture_paren(text: str, i: int) -> Tuple[str, int]:
    """从 text[i] 起捕获与 $( 配对的括号内容（支持嵌套与引号内的括号）"""
    depth = 1
    buf: List[str] = []
    quote = None
    while i < len(text):
        ch = text[i]
        if quote == "'":
            if ch == "'":
                quote = None
            buf.append(ch)
            i += 1
            continue
        if quote == '"':
            if ch == "\\" and i + 1 < len(text):
                buf.append(text[i : i + 2])
                i += 2
                continue
            if ch == '"':
                quote = None
            buf.append(ch)
            i += 1
            continue
        if ch == "\\" and i + 1 < len(text):
            buf.append(text[i : i + 2])
            i += 2
            continue
        if ch in "'\"":
            quote = ch
            buf.append(ch)
            i += 1
            continue
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
            if depth == 0:
                return "".join(buf), i + 1
        buf.append(ch)
        i += 1
    return "".join(buf), i  # 未闭合：原样返回


def _capture_backtick(text: str, i: int) -> Tuple[str, int]:
    """从 text[i] 起捕获与反引号配对的内容"""
    buf: List[str] = []
    while i < len(text):
        ch = text[i]
        if ch == "\\" and i + 1 < len(text):
            buf.append(text[i : i + 2])
            i += 2
            continue
        if ch == "`":
            return "".join(buf), i + 1
        buf.append(ch)
        i += 1
    return "".join(buf), i


def _split_and_extract(line: str) -> Tuple[List[str], List[str]]:
    """按命令分隔符切分，同时抽取命令替换内容。

    规则：
        - 引号（' "）内不切分，单引号内不展开、反斜杠不转义
        - $() 与反引号内容不参与外层切分，抽出后单独作为命令分析
        - 分隔符：; & | && ||（重定向中的 & > 组合不当作分隔符）

    返回：
        (segments, extracted)：命令段原始文本列表、抽出的替换内容列表
    """
    segments: List[str] = []
    extracted: List[str] = []
    buf: List[str] = []
    quote = None
    i = 0
    n = len(line)

    while i < n:
        ch = line[i]

        if quote == "'":
            buf.append(ch)
            if ch == "'":
                quote = None
            i += 1
            continue

        if quote == '"':
            if ch == "\\" and i + 1 < n:
                buf.append(line[i : i + 2])
                i += 2
                continue
            if ch == '"':
                quote = None
                buf.append(ch)
                i += 1
                continue
            if ch == "$" and i + 1 < n and line[i + 1] == "(":
                inner, i = _capture_paren(line, i + 2)
                extracted.append(inner)
                buf.append(_SUBST_PLACEHOLDER)
                continue
            if ch == "`":
                inner, i = _capture_backtick(line, i + 1)
                extracted.append(inner)
                buf.append(_BACKTICK_PLACEHOLDER)
                continue
            buf.append(ch)
            i += 1
            continue

        # 引号外
        if ch == "\\" and i + 1 < n:
            buf.append(line[i : i + 2])
            i += 2
            continue

        if ch in "'\"":
            quote = ch
            buf.append(ch)
            i += 1
            continue

        if ch == "$" and i + 1 < n and line[i + 1] == "(":
            inner, i = _capture_paren(line, i + 2)
            extracted.append(inner)
            buf.append(_SUBST_PLACEHOLDER)
            continue

        if ch == "`":
            inner, i = _capture_backtick(line, i + 1)
            extracted.append(inner)
            buf.append(_BACKTICK_PLACEHOLDER)
            continue

        # 重定向里的 &：2>&1、&>file、>&2
        if ch == "&":
            prev = line[i - 1] if i > 0 else ""
            nxt = line[i + 1] if i + 1 < n else ""
            if prev == ">" or (nxt == ">" and not prev.endswith("&")):
                buf.append(ch)
                i += 1
                continue

        if ch in ";&|":
            j = i
            if ch in "&|" and i + 1 < n and line[i + 1] == ch:
                j = i + 1
            segments.append("".join(buf))
            buf = []
            i = j + 1
            continue

        buf.append(ch)
        i += 1

    segments.append("".join(buf))
    return [s.strip() for s in segments if s.strip()], extracted


def _tokenize(segment: str) -> List[str]:
    """shell 分词：引号包裹的内容合并为单个 token，引号本身剥除"""
    tokens: List[str] = []
    buf: List[str] = []
    quote = None
    i = 0
    n = len(segment)

    while i < n:
        ch = segment[i]

        if quote == "'":
            if ch == "'":
                quote = None
            else:
                buf.append(ch)
            i += 1
            continue

        if quote == '"':
            if ch == "\\" and i + 1 < n:
                buf.append(segment[i + 1])
                i += 2
                continue
            if ch == '"':
                quote = None
            else:
                buf.append(ch)
            i += 1
            continue

        if ch == "\\" and i + 1 < n:
            buf.append(segment[i + 1])
            i += 2
            continue

        if ch in "'\"":
            quote = ch
            i += 1
            continue

        if ch.isspace():
            if buf:
                tokens.append("".join(buf))
                buf = []
            i += 1
            continue

        buf.append(ch)
        i += 1

    if buf:
        tokens.append("".join(buf))
    return tokens


def _strip_redirections(tokens: List[str]) -> List[str]:
    """剥除重定向操作符及其目标（> out 2>>err 2>&1 等），避免被当成参数"""
    out: List[str] = []
    skip_next = False
    for tok in tokens:
        if skip_next:
            skip_next = False
            continue
        if _REDIR_RE.match(tok):
            # fd 复制形式（2>&1）自带操作数，不需要再跳过下一个 token
            if not re.match(r"^(?:\d*)>&(?:\d+|-)$", tok):
                skip_next = True
            continue
        out.append(tok)
    return out


def _strip_prefixes(tokens: List[str], syntax: Dict) -> List[str]:
    """剥除环境变量赋值与前导包装命令，返回从真实命令词开始的 token 列表"""
    wrappers = set(syntax.get("wrappers") or [])
    wrapper_options = syntax.get("wrapper_options") or {}
    i = 0
    while i < len(tokens):
        tok = tokens[i]

        if _ASSIGN_RE.match(tok):
            i += 1
            continue

        if tok in wrappers:
            i += 1
            value_opts = set(wrapper_options.get(tok) or [])
            while i < len(tokens):
                nxt = tokens[i]
                if nxt.startswith("-") and nxt != "-":
                    i += 2 if nxt in value_opts else 1
                    continue
                break
            continue

        break
    return tokens[i:]


def _extract_inner_commands(tokens: List[str], syntax: Dict) -> List[str]:
    """提取间接执行的内部命令字符串（shell -c / eval / 远端命令）"""
    if not tokens:
        return []

    cmd = tokens[0].rsplit("/", 1)[-1]
    rest = tokens[1:]
    inner: List[str] = []

    if cmd in set(syntax.get("shell_commands") or []):
        for idx, tok in enumerate(rest):
            if tok == "-c" and idx + 1 < len(rest):
                inner.append(" ".join(rest[idx + 1 :]))
                break
    elif cmd in set(syntax.get("eval_commands") or []):
        if rest:
            inner.append(" ".join(rest))
    elif cmd in set(syntax.get("remote_commands") or []):
        # 跳过选项与选项值，第一个非选项参数是主机，其余是远端命令字符串
        value_opts = {"-p", "-l", "-F", "-o", "-i", "-J", "-L", "-R", "-W", "-b", "-e", "-c"}
        i = 0
        while i < len(rest) and rest[i].startswith("-") and rest[i] != "-":
            i += 2 if rest[i] in value_opts and i + 1 < len(rest) else 1
        args = rest[i:]
        if len(args) > 1:
            remote_cmd = " ".join(args[1:]).strip()
            if remote_cmd:
                inner.append(remote_cmd)

    return [x for x in inner if x.strip()]


def parse_line(
    line: str, syntax: Dict, max_depth: int = 5, _depth: int = 0
) -> List[Tuple[List[str], str]]:
    """解析一行命令，返回待判定的命令段。

    参数：
        line: 一行命令文本
        syntax: 语法清单（wrappers / shell_commands / eval_commands / remote_commands 等）
        max_depth: 间接执行递归深度上限

    返回：
        [(tokens, text)]：tokens 为真实命令词起的 token 列表，text 为对应文本
        （命令词已归一为不含目录的形式，便于规则匹配）
    """
    segments, extracted = _split_and_extract(line)
    result: List[Tuple[List[str], str]] = []

    for raw in segments:
        tokens = _strip_prefixes(_strip_redirections(_tokenize(raw)), syntax)
        if not tokens:
            continue

        # 命令词归一：/bin/rm → rm，./rm → rm
        normalized = [tokens[0].rsplit("/", 1)[-1]] + tokens[1:]
        result.append((normalized, " ".join(normalized)))

        if _depth < max_depth:
            for inner in _extract_inner_commands(tokens, syntax):
                result.extend(parse_line(inner, syntax, max_depth, _depth + 1))

    if _depth < max_depth:
        for inner in extracted:
            result.extend(parse_line(inner, syntax, max_depth, _depth + 1))

    return result
