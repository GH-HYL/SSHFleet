#!/usr/bin/env python3
"""
代码规模评估工具（行业通用口径）

设计原则：
- 规模指标与复杂度指标分开呈现，不做自创的混合评分
- 每个指标给出行业参考区间，便于横向对照
- 阈值尽量贴近行业常用口径（KLOC 分级、复杂度密度区间）

规模分级（按 NCLOC / KLOC）：
    < 1       Micro       微型脚本
    1 - 10    Small       小型工具
    10 - 50   Medium      中型系统
    50 - 100  Large       大型应用
    > 100     Very Large  企业级套件

复杂度密度参考区间（复杂度 / KLOC）：
    < 300        低风险
    300 - 600    中风险
    > 600        高风险
"""

import os
import re
import sys
from collections import defaultdict
from datetime import datetime
from pathlib import Path


IGNORED_DIRS = {
    'vendor', 'node_modules', '.venv', 'venv', '__pycache__',
    '.git', 'dist', 'build', 'site-packages', 'third_party',
    'third-party', '.pytest_cache', '.mypy_cache', '.tox',
    'eggs', 'egg-info', 'wheel', '.idea', '.vscode',
}

LANGUAGE_EXTENSIONS = {
    '.py': 'Python',
    '.go': 'Go',
}

SCALE_LEVELS = [
    (1, 'Micro', '微型脚本'),
    (10, 'Small', '小型工具'),
    (50, 'Medium', '中型系统'),
    (100, 'Large', '大型应用'),
    (float('inf'), 'Very Large', '企业级套件'),
]

DENSITY_LEVELS = [
    (300, 'Low', '低'),
    (600, 'Medium', '中'),
    (float('inf'), 'High', '高'),
]


# ---------- 语言处理 ----------

def strip_comments(content, language):
    """剔除注释与空行，返回 (code, comment_lines, blank_lines)。

    这是一个轻量实现，不解析字符串字面量；字符串内出现注释标记的场景
    会被误判，但对工程估算影响很小。
    """
    lines = content.split('\n')
    comment_count = 0
    blank_count = 0
    out = []

    in_block = False
    block_end = None

    for line in lines:
        stripped = line.strip()

        if not stripped:
            blank_count += 1
            out.append('')
            continue

        if in_block:
            comment_count += 1
            if block_end in stripped:
                in_block = False
            continue

        if language == 'Python':
            if stripped.startswith('#'):
                comment_count += 1
                continue
            if stripped.startswith('"""') or stripped.startswith("'''"):
                quote = '"""' if stripped.startswith('"""') else "'''"
                if stripped.count(quote) >= 2:
                    comment_count += 1
                    continue
                in_block = True
                block_end = quote
                comment_count += 1
                continue
        elif language == 'Go':
            if stripped.startswith('//'):
                comment_count += 1
                continue
            if stripped.startswith('/*'):
                if '*/' in stripped[2:]:
                    comment_count += 1
                    continue
                in_block = True
                block_end = '*/'
                comment_count += 1
                continue

        out.append(line)

    return '\n'.join(out), comment_count, blank_count


def count_ncloc(code):
    """非注释代码行数。跳过空行与孤立括号行。"""
    bare = {'{', '}', '(', ')', '};', '];', '},', '),'}
    count = 0
    for line in code.split('\n'):
        s = line.strip()
        if not s or s in bare:
            continue
        count += 1
    return count


def count_entities(code, language):
    """统计函数/方法、类/结构体、接口。"""
    functions = classes = interfaces = 0

    if language == 'Python':
        functions = len(re.findall(
            r'^\s*(?:async\s+)?def\s+\w+\s*\(', code, re.MULTILINE))
        classes = len(re.findall(
            r'^\s*class\s+\w+', code, re.MULTILINE))
        interfaces = len(re.findall(
            r'^\s*class\s+\w+\s*\(\s*(?:Protocol|ABC)[\s,)]',
            code, re.MULTILINE))
    elif language == 'Go':
        # 普通函数：func Name(
        functions = len(re.findall(
            r'^\s*func\s+\w+\s*\(', code, re.MULTILINE))
        # 方法：func (recv) Name(
        functions += len(re.findall(
            r'^\s*func\s*\([^)]*\)\s+\w+\s*\(', code, re.MULTILINE))
        classes = len(re.findall(
            r'^\s*type\s+\w+\s+struct\s*\{', code, re.MULTILINE))
        interfaces = len(re.findall(
            r'^\s*type\s+\w+\s+interface\s*\{', code, re.MULTILINE))

    return functions, classes, interfaces


def count_complexity(code, language, function_count):
    """简化圈复杂度：决策点 + 函数数（每个函数基础 1）。

    决策点：
        Python: if / elif / for / while / except / case / and / or
        Go:     if / for / case / && / ||
    """
    if language == 'Python':
        pattern = r'\b(?:if|elif|for|while|except|case|and|or)\b'
    elif language == 'Go':
        pattern = r'\b(?:if|for|case)\b|&&|\|\|'
    else:
        return 0

    return len(re.findall(pattern, code)) + function_count


# ---------- 数据容器 ----------

class FileMetrics:
    __slots__ = (
        'path', 'language', 'physical_lines', 'comment_lines',
        'blank_lines', 'ncloc', 'complexity', 'functions',
        'classes', 'interfaces',
    )

    def __init__(self, path, language):
        self.path = path
        self.language = language
        self.physical_lines = 0
        self.comment_lines = 0
        self.blank_lines = 0
        self.ncloc = 0
        self.complexity = 0
        self.functions = 0
        self.classes = 0
        self.interfaces = 0

    @property
    def density(self):
        """复杂度密度：复杂度 / KLOC。"""
        if self.ncloc == 0:
            return 0.0
        return self.complexity * 1000 / self.ncloc


# ---------- 扫描与分析 ----------

class Analyzer:
    def __init__(self, root):
        self.root = Path(root).resolve()
        self.files = []
        self.skipped = 0

    def discover(self):
        for ext in LANGUAGE_EXTENSIONS:
            for p in self.root.rglob(f'*{ext}'):
                if any(part.lower() in IGNORED_DIRS for part in p.parts):
                    self.skipped += 1
                    continue
                self.files.append(p)
        self.files.sort()

    def analyze_all(self):
        for path in self.files:
            m = self._analyze(path)
            if m:
                yield m

    def _analyze(self, path):
        try:
            content = path.read_text(encoding='utf-8', errors='ignore')
        except OSError as e:
            print(f"  ⚠️  无法读取 {path}: {e}", file=sys.stderr)
            return None

        language = LANGUAGE_EXTENSIONS[path.suffix.lower()]
        code, comment_lines, blank_lines = strip_comments(content, language)

        m = FileMetrics(str(path), language)
        m.physical_lines = len(content.split('\n'))
        m.comment_lines = comment_lines
        m.blank_lines = blank_lines
        m.ncloc = count_ncloc(code)
        m.functions, m.classes, m.interfaces = count_entities(code, language)
        m.complexity = count_complexity(code, language, m.functions)
        return m


# ---------- 报告 ----------

def scale_label(ncloc):
    kloc = ncloc / 1000
    for threshold, label, desc in SCALE_LEVELS:
        if kloc < threshold:
            return f"{label}（{desc}）"
    return "Unknown"


def density_label(density):
    for threshold, _, desc in DENSITY_LEVELS:
        if density < threshold:
            return desc
    return '?'


def print_report(root, files, skipped):
    total = FileMetrics('<total>', '-')
    for f in files:
        total.physical_lines += f.physical_lines
        total.comment_lines += f.comment_lines
        total.blank_lines += f.blank_lines
        total.ncloc += f.ncloc
        total.complexity += f.complexity
        total.functions += f.functions
        total.classes += f.classes
        total.interfaces += f.interfaces

    by_lang = defaultdict(lambda: {
        'files': 0, 'ncloc': 0, 'complexity': 0,
        'functions': 0, 'classes': 0, 'interfaces': 0,
    })
    for f in files:
        s = by_lang[f.language]
        s['files'] += 1
        s['ncloc'] += f.ncloc
        s['complexity'] += f.complexity
        s['functions'] += f.functions
        s['classes'] += f.classes
        s['interfaces'] += f.interfaces

    density = total.complexity * 1000 / total.ncloc if total.ncloc else 0

    print()
    print(f"📂 扫描: {len(files)} 个文件（跳过 {skipped} 个）")

    # ---- 规模 ----
    print()
    print("📏 规模指标")
    print(f"  NCLOC（非注释代码行）: {total.ncloc:,}")
    print(f"  物理行数: {total.physical_lines:,}"
          f"  |  注释行: {total.comment_lines:,}"
          f"  |  空行: {total.blank_lines:,}")
    print(f"  函数/方法: {total.functions:,}"
          f"  |  类/结构体: {total.classes:,}"
          f"  |  接口: {total.interfaces:,}")
    print(f"  规模等级: {scale_label(total.ncloc)}")

    # ---- 复杂度 ----
    print()
    print("🔀 复杂度指标（McCabe 简化口径）")
    print(f"  总圈复杂度: {total.complexity:,}")
    if total.functions:
        print(f"  平均函数复杂度: {total.complexity / total.functions:.2f}")
    else:
        print(f"  平均函数复杂度: N/A")
    print(f"  复杂度密度: {density:.1f} / KLOC")
    print(f"    参考区间: < 300 低  |  300-600 中  |  > 600 高")
    print(f"    结论: {density_label(density)}")

    # ---- 语言 ----
    print()
    print("🌐 语言分布")
    for lang, s in sorted(by_lang.items()):
        d = s['complexity'] * 1000 / s['ncloc'] if s['ncloc'] else 0
        avg = s['ncloc'] / s['functions'] if s['functions'] else 0
        print(f"  {lang}")
        print(f"    文件 {s['files']}  |  NCLOC {s['ncloc']:,}"
              f"  |  复杂度 {s['complexity']:,}")
        print(f"    函数 {s['functions']}  |  类 {s['classes']}"
              f"  |  接口 {s['interfaces']}")
        if s['functions']:
            print(f"    行/函数 {avg:.1f}  |  复杂度密度 {d:.1f}/KLOC")

    # ---- 高风险文件（避免极小文件干扰，设 NCLOC ≥ 50 门槛）----
    print()
    print("⚠️  高风险文件（NCLOC ≥ 50，按复杂度密度排序）")
    candidates = [f for f in files if f.ncloc >= 50]
    candidates.sort(key=lambda f: f.density, reverse=True)
    if candidates:
        for i, f in enumerate(candidates[:5], 1):
            rel = os.path.relpath(f.path, root)
            print(f"  {i}. {rel}")
            print(f"     NCLOC {f.ncloc}  |  复杂度 {f.complexity}"
                  f"  |  密度 {f.density:.1f}/KLOC  |  函数 {f.functions}")
    else:
        print("  无（无 NCLOC ≥ 50 的文件）")

    # ---- 最大文件 ----
    print()
    print("📦 最大文件 Top 5（按 NCLOC）")
    by_size = sorted(files, key=lambda f: f.ncloc, reverse=True)
    for i, f in enumerate(by_size[:5], 1):
        rel = os.path.relpath(f.path, root)
        print(f"  {i}. {rel}")
        print(f"     NCLOC {f.ncloc}  |  复杂度 {f.complexity}"
              f"  |  函数 {f.functions}")

    # ---- 健康度提示 ----
    print()
    print("🏥 健康度检查")
    issues = []

    large = [f for f in files if f.ncloc > 500]
    if large:
        issues.append(f"  - {len(large)} 个文件 NCLOC 超过 500，考虑按职责拆分")

    dense = [f for f in files if f.density > 600 and f.ncloc >= 50]
    if dense:
        issues.append(f"  - {len(dense)} 个文件复杂度密度 > 600/KLOC，建议重构")

    if total.ncloc and total.functions:
        fpk = total.functions * 1000 / total.ncloc
        if fpk < 5:
            issues.append(f"  - 函数密度偏低（{fpk:.1f}/KLOC），可能存在大函数")
        elif fpk > 50:
            issues.append(f"  - 函数密度偏高（{fpk:.1f}/KLOC），可能存在过度拆分")

    if issues:
        for it in issues:
            print(it)
    else:
        print("  ✅ 未发现明显问题")

    print()
    print("─" * 72)
    print("✅ 分析完成")
    print("─" * 72)


def main():
    if len(sys.argv) < 2:
        print("用法: python code_metrics.py <项目路径>")
        sys.exit(1)

    root = sys.argv[1]
    if not os.path.exists(root):
        print(f"路径不存在: {root}")
        sys.exit(1)

    print()
    print("=" * 72)
    print("📊 代码规模评估报告")
    print(f"📁 项目: {root}")
    print(f"🕐 时间: {datetime.now():%Y-%m-%d %H:%M:%S}")
    print("=" * 72)

    analyzer = Analyzer(root)
    analyzer.discover()
    files = list(analyzer.analyze_all())

    if not files:
        print("\n未找到可分析的源代码文件。")
        return

    print_report(root, files, analyzer.skipped)


if __name__ == '__main__':
    main()