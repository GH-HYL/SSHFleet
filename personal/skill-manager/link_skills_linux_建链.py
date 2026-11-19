"""Ubuntu 上把 ~/.workbuddy/skills 的三个分目录软链接到工作区（真正的 symlink）。

用法::

    python3 tools/skill-manager/link_skills_linux_建链.py
    python3 tools/skill-manager/link_skills_linux_建链.py --workspace ~/code/SSHFleet

映射与安全规则跟 Windows 版同一个（都走 common.py），差别只在：这里建的是真符号链接，
不搞目录联接；另外会先把 ``~/.workbuddy/skills`` 建出来——Ubuntu 上首次装 workbuddy 时它可能还不存在。

默认工作区取脚本自身往上三级；脚本放在别处跑（比如软链接过来）时用 ``--workspace`` 指明。
"""

from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path

import common


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="在 Ubuntu 上把 workbuddy 技能目录软链接到工作区 skills/ 的三个分目录。")
    parser.add_argument("--workspace", type=Path, default=None, help="工作区根目录（默认取脚本上溯三级）")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    workspace = (args.workspace or common.workspace_root()).expanduser().resolve()

    if not (workspace / "AGENTS.md").exists():
        print(f"失败：{workspace} 不像工作区根（找不到 AGENTS.md），已退出。用 --workspace 指明正确路径。")
        return 1

    if os.name == "nt":
        print("提示：当前不是 Linux，跑出来的会是 Windows 那套链接形态；本机请用 link_skills_建链_windows.py。\n")

    print(f"对齐 {workspace}/skills/ 与 workbuddy 技能目录：")
    results = common.apply_links(workspace)
    return common.print_report(results, common.workbuddy_skills_dir(), common.skills_root(workspace))


if __name__ == "__main__":
    sys.exit(main())
