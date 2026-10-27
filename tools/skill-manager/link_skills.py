"""把 workbuddy 技能目录挂到工作区 skills/ 下的三个分目录（Windows）。

用法::

    python tools/skill-manager/link_skills.py

做完之后 ``%USERPROFILE%\\.workbuddy\\skills\\{person,engineering,productivity}`` 都是指向
``<工作区>\\skills\\{个人,工程,生产}`` 的链接，改哪边都同时生效。

可重复运行：已经链对的组直接跳过；workbuddy 侧还是实体目录、而工作区那边还空着，
先把内容搬过去再建链（技能不会丢）；两侧都有内容则停下报冲突，不替开发者择一。
"""

from __future__ import annotations

import sys

import common


def main() -> int:
    print("对齐工作区 skills/ 与 workbuddy 技能目录：")
    results = common.apply_links()
    return common.print_report(results, common.workbuddy_skills_dir(), common.skills_root())


if __name__ == "__main__":
    sys.exit(main())
