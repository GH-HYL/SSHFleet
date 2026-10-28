"""从 GitHub 取 mattpocock/skills 的最新 engineering、productivity 两组技能，替换工作区里的旧版。

用法::

    python tools/skill-manager/update_skills_拉上游更新.py [--repo URL] [--branch main] [--keep]

流程：问一次 API 拿到分支当前提交 → 下载该提交的归档包到 ``build/skill-manager/`` → 两组都在才动手 →
把新的铺到临时位置 → 全部铺好后逐组换入 → 清掉临时文件。

替换是整目录换掉，不合并：本地对这两组做过的改动会被覆盖。要的是跟上游一致，
留着旧文件只会让「这版到底是不是最新的」变得说不清。

只用标准库，不需要装 git——归档走 codeload，比 git 协议在代理后面更容易通。
"""

from __future__ import annotations

import argparse
import json
import shutil
import tarfile
import urllib.error
import urllib.request
from pathlib import Path

import common

DEFAULT_REPO = "https://github.com/mattpocock/skills"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="拉取 mattpocock/skills 最新的 engineering、productivity 两组技能并替换本地旧版。")
    parser.add_argument("--repo", default=DEFAULT_REPO, help=f"上游仓库地址（默认 {DEFAULT_REPO}）")
    parser.add_argument("--branch", default="main", help="上游分支（默认 main）")
    parser.add_argument("--keep", action="store_true", help="跑完不删 build/ 下的下载物，留着看")
    return parser.parse_args()


def repo_slug(repo_url: str) -> str:
    """``https://github.com/owner/repo(.git)`` → ``owner/repo``。"""
    slug = repo_url.rstrip("/")
    if slug.endswith(".git"):
        slug = slug[:-4]
    for marker in ("github.com/", "github.com:"):
        if marker in slug:
            slug = slug.split(marker, 1)[1]
            break
    parts = [p for p in slug.split("/") if p]
    if len(parts) != 2:
        raise ValueError(f"看不懂的仓库地址：{repo_url}（要形如 https://github.com/owner/repo）")
    return "/".join(parts)


def fetch_json(url: str) -> dict:
    req = urllib.request.Request(url, headers={"User-Agent": "sshfleet-skill-manager"})
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read().decode("utf-8"))


def resolve_commit(slug: str, branch: str) -> tuple[str, str]:
    """返回 (下载用的 ref, 给人看的版本说明)。API 取不到就退回分支名，不算致命。"""
    try:
        data = fetch_json(f"https://api.github.com/repos/{slug}/commits/{branch}")
        sha = data["sha"]
        date = data["commit"]["committer"]["date"][:10]
        return sha, f"{sha[:7]}（{date}）"
    except (urllib.error.URLError, KeyError, ValueError, TimeoutError) as exc:
        return branch, f"{branch} 分支（取不到提交号：{exc}）"


def download_tarball(slug: str, ref: str, dest: Path) -> Path:
    url = f"https://codeload.github.com/{slug}/tar.gz/{ref}"
    print(f"  下载 {url}")
    req = urllib.request.Request(url, headers={"User-Agent": "sshfleet-skill-manager"})
    try:
        with urllib.request.urlopen(req, timeout=120) as resp, dest.open("wb") as fh:
            shutil.copyfileobj(resp, fh)
    except urllib.error.URLError as exc:
        raise RuntimeError(f"下载失败，工作区未改动：{exc}；检查网络或代理后重跑") from exc
    return dest


def extract(tarball: Path, dest: Path) -> Path:
    """解到 dest，返回归档里那个顶层目录（GitHub 的包都套一层 ``<repo>-<ref>``）。"""
    if dest.exists():
        shutil.rmtree(dest)
    dest.mkdir(parents=True)
    with tarfile.open(tarball, "r:gz") as tar:
        try:
            tar.extractall(dest, filter="data")
        except TypeError:  # 3.11 及更早没有 filter 参数
            tar.extractall(dest)
    tops = [p for p in dest.iterdir() if p.is_dir()]
    if len(tops) != 1:
        raise RuntimeError(f"归档结构不对，解出来有 {len(tops)} 个顶层目录；删掉 build/skill-manager/ 后重跑")
    return tops[0]


def count_files(root: Path) -> int:
    return sum(1 for p in root.rglob("*") if p.is_file())


def main() -> int:
    args = parse_args()
    skills = common.skills_root()
    staging = common.workspace_root() / "build" / "skill-manager"

    try:
        slug = repo_slug(args.repo)
    except ValueError as exc:
        print(f"  失败：{exc}")
        return 1

    ref, described = resolve_commit(slug, args.branch)
    print(f"  上游 {slug} @ {described}\n")

    tarball = staging / "upstream.tar.gz"
    staging.mkdir(parents=True, exist_ok=True)
    try:
        download_tarball(slug, ref, tarball)
        root = extract(tarball, staging / "upstream")
    except (RuntimeError, tarfile.TarError, OSError) as exc:
        print(f"  失败：{exc}")
        return 1

    # 先验齐再动手：上游少一组就整体退出，不留半套
    missing = [f"skills/{name}" for name in common.UPSTREAM_GROUPS if not (root / "skills" / name).is_dir()]
    if missing:
        print("  失败：上游缺目录 " + "、".join(missing) + "；工作区未改动，先确认仓库结构再重跑。")
        return 1

    # 先铺到临时位置，全铺好了才换入——中途失败不至于新旧各半
    staged = staging / "staged"
    if staged.exists():  # 上次跑到一半留下的残渣会挡住 copytree，先清掉
        shutil.rmtree(staged)
    try:
        for name in common.UPSTREAM_GROUPS:
            shutil.copytree(root / "skills" / name, staged / name)
    except OSError as exc:
        print(f"  失败：铺临时目录出错（{exc}）；工作区未改动，重跑即可。")
        return 1

    skills.mkdir(parents=True, exist_ok=True)
    for name in common.UPSTREAM_GROUPS:
        dst = skills / name
        old = count_files(dst) if dst.is_dir() else 0
        common.drop_path(dst)
        shutil.move(str(staged / name), str(dst))
        print(f"  {name}：{old} 个文件 → {count_files(dst)} 个文件")

    if args.keep:
        print(f"\n  下载物留在 {staging}")
    else:
        shutil.rmtree(staging, ignore_errors=True)

    # 链接断了两边就对不上，提醒一句；这里不替开发者去建链
    wb = common.workbuddy_skills_dir()
    broken = [n for n in common.GROUPS if not (wb / n).is_dir()]
    if broken:
        print(f"\n  注意：workbuddy 侧看不到 {', '.join(broken)}；先跑建链脚本（link_skills_建链_windows.py / link_skills_linux_建链.py）。")

    print(f"\n  完成，工作区技能已更新到 {described}")
    return 0


if __name__ == "__main__":
    import sys

    sys.exit(main())
