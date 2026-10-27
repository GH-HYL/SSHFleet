"""skill 管理工具的共用部分：三组目录映射、工作区定位、链接的建／查／换。

三个脚本共用这一份映射，免得链接脚本与更新脚本各写一套、名字还对不上。

方向固定：工作区 ``skills/<中文名>/`` 是实体，``~/.workbuddy/skills/<英文名>`` 是指过去的链接。
这一向照既有先例定的（``person`` 早先就链在工作区的技能目录上），Ubuntu 侧同理。

**链接形态按平台走**：Ubuntu 上是真符号链接；Windows 上优先符号链接，建不成退成目录联接
（junction，不需要提权，客户端扫描时同样当普通目录读）。判成没成一律看解析结果——
本机上见过 ``os.symlink`` 不抛错、却只落下一个空目录的情形，认返回值会误判成建好了。
"""

from __future__ import annotations

import os
import shutil
import subprocess
from dataclasses import dataclass
from pathlib import Path

# (workbuddy 侧链接名, 工作区侧目录名)
MAPPINGS: tuple[tuple[str, str], ...] = (
    ("person", "个人"),
    ("engineering", "工程"),
    ("productivity", "生产"),
)


def workspace_root() -> Path:
    """``tools/skill-manager/common.py`` 上溯三级即工作区根。"""
    return Path(__file__).resolve().parents[2]


def skills_root(workspace: Path | None = None) -> Path:
    return (workspace or workspace_root()) / "skills"


def workbuddy_skills_dir() -> Path:
    return Path.home() / ".workbuddy" / "skills"


def is_link(path: Path) -> bool:
    """链接有两种：符号链接，以及 Windows 上的目录联接。"""
    return path.is_symlink() or (os.name == "nt" and os.path.isjunction(path))


def points_at(link: Path, target: Path) -> bool:
    """链接是否真的指到 target——只认解析结果。"""
    if not is_link(link):
        return False
    try:
        return Path(os.path.realpath(link)) == Path(os.path.realpath(target))
    except OSError:
        return False


def remove_any(path: Path) -> None:
    """按实际形态删掉一个链接或空目录；非空目录会抛错，正好拦住误删。"""
    if not (path.exists() or is_link(path)):
        return
    if os.name == "nt" and os.path.isjunction(path):
        os.rmdir(path)
    elif path.is_symlink():
        os.unlink(path)
    elif path.is_dir():
        os.rmdir(path)
    else:
        os.unlink(path)


def make_link(link: Path, target: Path) -> str:
    """建目录链接，返回实际用的形态：``symlink`` 或 ``junction``。"""
    if os.name != "nt":
        os.symlink(target, link, target_is_directory=True)
        if not points_at(link, target):
            raise RuntimeError(f"符号链接建好了却没指到 {target}，已停下")
        return "symlink"

    try:
        os.symlink(target, link, target_is_directory=True)
    except OSError:
        pass
    if points_at(link, target):
        return "symlink"

    # 符号链接没成（需提权，或建出来是个空壳）：清掉重来，改用目录联接
    remove_any(link)
    proc = subprocess.run(
        ["cmd", "/c", "mklink", "/J", str(link), str(target)],
        capture_output=True,
        text=True,
    )
    if proc.returncode != 0 or not points_at(link, target):
        raise RuntimeError("建立链接失败（符号链接与目录联接都试过）：" + (proc.stdout + proc.stderr).strip())
    return "junction"


@dataclass
class GroupResult:
    link_name: str
    target_name: str
    state: str
    detail: str


def apply_links(workspace: Path | None = None) -> list[GroupResult]:
    """按 MAPPINGS 逐个对齐；单组出错不影响其余组，最后统一回报。"""
    ws = skills_root(workspace)
    wb = workbuddy_skills_dir()
    wb.mkdir(parents=True, exist_ok=True)

    results: list[GroupResult] = []
    for link_name, target_name in MAPPINGS:
        link = wb / link_name
        target = ws / target_name
        try:
            results.append(_apply_one(link, target, target_name))
        except (OSError, RuntimeError) as exc:
            results.append(GroupResult(link_name, target_name, "失败", str(exc)))
    return results


def _apply_one(link: Path, target: Path, target_name: str) -> GroupResult:
    # 已经链对了：工作区那边还在就什么都不用做
    if points_at(link, target):
        if target.is_dir():
            return GroupResult(link.name, target_name, "跳过", f"已链到 {target}，未改动")
        target.mkdir(parents=True, exist_ok=True)
        return GroupResult(link.name, target_name, "补建", f"链接已在，目标目录缺失，已建出 {target}")

    # 链到了别处（含指丢了的）：改指过去，原链接本身没有内容，不丢东西
    if is_link(link):
        old = os.path.realpath(link)
        remove_any(link)
        target.mkdir(parents=True, exist_ok=True)
        kind = make_link(link, target)
        return GroupResult(link.name, target_name, "改指", f"{old} → {target}（{kind}）")

    # workbuddy 侧还是实体目录：把内容搬进工作区再建链，避免技能凭空消失
    if link.is_dir():
        target.mkdir(parents=True, exist_ok=True)
        # 空壳目录没有内容可搬，直接换掉——否则半途中断留下的空目录会一直被当成冲突
        if any(link.iterdir()) and any(target.iterdir()):
            return GroupResult(
                link.name,
                target_name,
                "冲突",
                f"两侧都有内容（{link} 与 {target}），未改动；先自行并成一份再重跑",
            )
        moved = 0
        for entry in list(link.iterdir()):
            shutil.move(str(entry), str(target / entry.name))
            moved += 1
        if any(link.iterdir()):
            return GroupResult(link.name, target_name, "冲突", f"{link} 里还有搬不动的残留，未建链")
        link.rmdir()
        kind = make_link(link, target)
        return GroupResult(link.name, target_name, "已迁移", f"{moved} 项搬入 {target}，原目录改作链接（{kind}）")

    # 什么都没有：直接建
    target.mkdir(parents=True, exist_ok=True)
    kind = make_link(link, target)
    return GroupResult(link.name, target_name, "新建", f"指向 {target}（{kind}）")


def print_report(results: list[GroupResult], wb: Path, ws: Path) -> int:
    width = max(len(r.link_name) for r in results) if results else 4
    for r in results:
        print(f"  {r.link_name.ljust(width)}  [{r.state}] {r.detail}")
    failed = [r for r in results if r.state in ("失败", "冲突")]
    print()
    print(f"  workbuddy 侧：{wb}")
    print(f"  工作区侧    ：{ws}")
    if failed:
        print(f"\n  有 {len(failed)} 组没弄好，见上面的 [失败]／[冲突]。")
        return 1
    return 0
