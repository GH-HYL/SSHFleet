"""把 workbuddy 技能目录挂到工作区 skills/ 下的三个分目录（Windows）。

用法::

    python tools/skill-manager/link_skills_建链_windows.py

做完之后 ``%USERPROFILE%\\.workbuddy\\skills\\{person,engineering,productivity}`` 都是指向
``<工作区>\\skills\\`` 下同名目录的链接，改哪边都同时生效。

可重复运行：已经链对的组直接跳过；workbuddy 侧还是实体目录时先把内容并进工作区再建链
（技能不会丢，上次搬到一半的副本按内容去重后接着搬）；真正内容冲突则停下点名报出，不替开发者择一。
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
