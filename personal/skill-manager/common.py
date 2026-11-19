"""skill 管理工具的共用部分：分目录清单、工作区定位、链接的建／查／换。

三个脚本共用这一份清单，免得链接脚本与更新脚本各写一套、名字还对不上。

方向固定：工作区 ``skills/<name>/`` 是实体，``~/.workbuddy/skills/<name>`` 是指过去的链接，
两侧同名。这一向照既有先例定的（``person`` 早先就链在工作区的技能目录上），Ubuntu 侧同理。

**链接形态按平台走**：Ubuntu 上是真符号链接；Windows 上优先符号链接，建不成退成目录联接
（junction，不需要提权，客户端扫描时同样当普通目录读）。判成没成一律看解析结果——
本机上见过 ``os.symlink`` 不抛错、却只落下一个空目录的情形，认返回值会误判成建好了。
"""

from __future__ import annotations

import hashlib
import os
import shutil
import subprocess
from dataclasses import dataclass
from pathlib import Path

# 三组分目录，两侧同名。要加一组就往这里加一行
GROUPS: tuple[str, ...] = ("person", "engineering", "productivity")

# 要从上游 mattpocock/skills 取的那两组；person 是自己写的，不从上游拿
UPSTREAM_GROUPS: tuple[str, ...] = ("engineering", "productivity")

# os.path.isjunction 是 3.12 才有的；老 Python 上退成「不认联接」，而不是直接崩
_is_junction = getattr(os.path, "isjunction", lambda _path: False)


def workspace_root() -> Path:
    """``tools/skill-manager/common.py`` 上溯三级即工作区根。"""
    return Path(__file__).resolve().parents[2]


def skills_root(workspace: Path | None = None) -> Path:
    return (workspace or workspace_root()) / "skills"


def workbuddy_skills_dir() -> Path:
    return Path.home() / ".workbuddy" / "skills"


def is_link(path: Path) -> bool:
    """链接有两种：符号链接，以及 Windows 上的目录联接。"""
    return path.is_symlink() or (os.name == "nt" and _is_junction(path))


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
    if os.name == "nt" and _is_junction(path):
        os.rmdir(path)
    elif path.is_symlink():
        os.unlink(path)
    elif path.is_dir():
        os.rmdir(path)
    else:
        os.unlink(path)


def drop_path(path: Path) -> None:
    """删一个目录、链接或文件，不管它是哪种形态。"""
    if is_link(path):
        remove_any(path)
    elif path.is_dir():
        shutil.rmtree(path)
    elif path.exists():
        path.unlink()


def _tree_signature(root: Path) -> dict[str, str]:
    return {
        str(p.relative_to(root)).replace("\\", "/"): hashlib.sha1(p.read_bytes()).hexdigest()
        for p in root.rglob("*")
        if p.is_file()
    }


def _same_tree(a: Path, b: Path) -> bool:
    """两处内容是否完全一致——用来判断「这个副本是不是上次已经搬过去了」。"""
    if a.is_file() or b.is_file():
        return a.is_file() and b.is_file() and a.read_bytes() == b.read_bytes()
    try:
        return _tree_signature(a) == _tree_signature(b)
    except OSError:
        return False


def make_link(link: Path, target: Path) -> str:
    """建目录链接，返回实际用的形态：``symlink`` 或 ``junction``。"""
    if os.name != "nt":
        os.symlink(target, link, target_is_directory=True)
        if not points_at(link, target):
            raise RuntimeError(f"符号链接建好了却没指到 {target}；确认目标目录存在后重跑本脚本")
        return "symlink"

    try:
        os.symlink(target, link, target_is_directory=True)
    except OSError:
        pass
    if points_at(link, target):
        return "symlink"

    # 符号链接没成（需提权，或建出来是个空壳）：清掉重来，改用目录联接
    remove_any(link)
    exe = subprocess.run(
        ["cmd", "/c", "mklink", "/J", str(link), str(target)],
        capture_output=True,
        text=True,
    )
    if exe.returncode != 0 or not points_at(link, target):
        raise RuntimeError(
            "建立链接失败（符号链接与目录联接都试过）："
            + (exe.stdout + exe.stderr).strip()
            + f"；确认 {target} 所在盘可写后重跑本脚本"
        )
    return "junction"


@dataclass
class GroupResult:
    group: str
    state: str
    detail: str


def apply_links(workspace: Path | None = None) -> list[GroupResult]:
    """按 GROUPS 逐个对齐；单组出错不影响其余组，最后统一回报。"""
    ws = skills_root(workspace)
    wb = workbuddy_skills_dir()
    wb.mkdir(parents=True, exist_ok=True)

    results: list[GroupResult] = []
    for name in GROUPS:
        try:
            results.append(_apply_one(wb / name, ws / name, name))
        except (OSError, RuntimeError) as exc:
            results.append(GroupResult(name, "失败", str(exc)))
    return results


def _apply_one(link: Path, target: Path, name: str) -> GroupResult:
    # 已经链对了：工作区那边还在就什么都不用做
    if points_at(link, target):
        if target.is_dir():
            return GroupResult(name, "跳过", f"已链到 {target}，未改动")
        target.mkdir(parents=True, exist_ok=True)
        return GroupResult(name, "补建", f"链接已在，目标目录缺失，已建出 {target}")

    # 链到了别处（含指丢了的）：改指过去，原链接本身没有内容，不丢东西
    if is_link(link):
        old = os.path.realpath(link)
        remove_any(link)
        target.mkdir(parents=True, exist_ok=True)
        kind = make_link(link, target)
        return GroupResult(name, "改指", f"{old} → {target}（{kind}）")

    # workbuddy 侧还是实体目录：把内容并进工作区再建链，避免技能凭空消失
    if link.is_dir():
        target.mkdir(parents=True, exist_ok=True)
        merged = 0
        clashes = []
        for entry in sorted(link.iterdir()):
            dest = target / entry.name
            if not dest.exists():
                shutil.move(str(entry), str(dest))
                merged += 1
            elif _same_tree(entry, dest):
                # 上次搬到一半断掉留下的副本：工作区已有一样的，删掉源这一份接着走
                drop_path(entry)
                merged += 1
            else:
                clashes.append(entry.name)
        if clashes:
            return GroupResult(
                name,
                "冲突",
                "、".join(clashes) + " 两侧内容不同，这几项没搬；其余已就位。处理完重跑即可，搬过的会跳过",
            )
        detail = f"原目录是空的，直接换成指向 {target} 的链接" if merged == 0 else f"{merged} 项并入 {target}，原目录改作链接"
        link.rmdir()
        kind = make_link(link, target)
        return GroupResult(name, "已迁移", f"{detail}（{kind}）")

    # 什么都没有：直接建
    target.mkdir(parents=True, exist_ok=True)
    kind = make_link(link, target)
    return GroupResult(name, "新建", f"指向 {target}（{kind}）")


def print_report(results: list[GroupResult], wb: Path, ws: Path) -> int:
    width = max(len(r.group) for r in results) if results else 4
    for r in results:
        print(f"  {r.group.ljust(width)}  [{r.state}] {r.detail}")
    failed = [r for r in results if r.state in ("失败", "冲突")]
    print()
    print(f"  workbuddy 侧：{wb}")
    print(f"  工作区侧    ：{ws}")
    if failed:
        print(f"\n  有 {len(failed)} 组没弄好，见上面的说明；按每条给出的办法处理后重跑，弄好的会跳过。")
        return 1
    return 0
