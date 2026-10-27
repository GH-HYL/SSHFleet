#!/usr/bin/env python3
"""SSHFleet 打包发布脚本。

⚠️ 本脚本连同 release/ 目录都是**开发者本人手动使用**的，与 AI coding、测试无关：
   改代码、跑测试、验证功能**都不涉及**这里。除非开发者在本轮明确要求发布，
   否则不要跑本脚本、不要动 release/、也不要因为「产物没更新」而去重跑它。

用法：
    python tools/release/release.py            # 交互选档位，回车即发布
    python tools/release/release.py --dry-run  # 只打印将要做什么，不落地

它做的事，按顺序：校验 → 定号 → 封段 → 插新未发布区 → 改入口版本常量
→ 提交 → 打 tag → 两个平台编译打包 → 校验产物非空。

一次出齐两个平台的包：release/SSHFleet_<号>_<时间戳>_windows.zip
与 release/SSHFleet_<号>_<时间戳>_linux.tar.gz。打包前清空整个 release/，
目录里永远只有最新一次的产物。

版本号由脚本从 CHANGELOG 顶部最近一个已发布段读出，再按所选档位加一；
人手不敲号，只选档位。tag 必须打在打包之前，否则包里的构建标识指向的
提交不是 tag 那个，事后对不上。

下面这些坑都是两个旧脚本（build-windows.bat / build-linux.sh）踩出来的，
重写时逐条搬了过来，别删：
  - 编译临时目录必须落在工作区 build/tmp：落 %TEMP% 会被 360 当木马静默删，
    现象是构建莫名失败且不报原因（GOTMPDIR / TMPDIR / TMP / TEMP 四个都指）。
  - zip 必须先写目录项再写文件，且文件名按 UTF-8 置位：zipfile 用 str 路径
    写入时会置 general purpose bit 11（实测读到 0x0800），Windows 解压不会乱码；
    Info-ZIP 的 zip -UN=UTF8 反而只读到 0x0000。
  - tar.gz 里的文件权限固定 755，不依赖源文件当前权限；文本文件跟着 755 无害，
    与旧 bat 侧口径一致。
  - release/ 清空前必须确认它在工作区内，rm -rf 打到工作区外不可逆。
  - 包名时间戳不能用 %DATE% 那类本地化格式：中文 Windows 下形如「2026/10/16 周五」，
    带斜杠会建出多层目录。这里用 datetime 自己格式化。
  - 打包中途失败会留下 0 字节文件，所以打完按大小再校验一遍（>1000 字节）。
  - 找 go 不能只看 PATH：Linux 上还可能在 /snap、/usr/local、/usr/lib。
  - 两个平台的包共用同一个时间戳，取一次就好。
"""

from __future__ import annotations

import argparse
import os
import re
import shutil
import subprocess
import sys
import tarfile
import zipfile
from datetime import datetime
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
MOD = ROOT / "modules" / "SSHFleet_Go"
CFG = MOD / "config"
DOCS_SRC = ROOT / "docs"
OUT = ROOT / "build"
REL = ROOT / "release"
CHANGELOG = ROOT / "CHANGELOG.md"
MAIN_GO = MOD / "main.go"

APP_NAME = "SSHFleet"
PLATFORMS = ("windows", "linux")
DEFAULT_VER = "1.0.0"

RE_HEAD = re.compile(r"^## \[(\d+)\.(\d+)\.(\d+)\]", re.M)
RE_UNDONE = re.compile(r"^## 未发布\s*$", re.M)
RE_NEXT_H2 = re.compile(r"^## ", re.M)
RE_DAY = re.compile(r"^### (\d{4}-\d{2}-\d{2})\s*$", re.M)
RE_ITEM = re.compile(r"^- ", re.M)
RE_APPVER = re.compile(r'^const appVersion = "([^"]*)"', re.M)


def die(msg: str) -> "NoReturn":  # noqa: F821
    print(f"[失败] {msg}", file=sys.stderr)
    sys.exit(1)


def step(msg: str) -> None:
    print(msg)


# 读写一律走这两支，保留文件原有行尾。Path.read_text / write_text 会做通用换行转换：
# 在 Linux 上跑一次就把 CHANGELOG 的 CRLF 整份改成 LF，下次在 Windows 上提交时
# 整个文件都是改动。
def read_text(p: Path) -> str:
    with open(p, encoding="utf-8", newline="") as f:
        return f.read()


def write_text(p: Path, s: str) -> None:
    with open(p, "w", encoding="utf-8", newline="") as f:
        f.write(s)


def eol_of(s: str) -> str:
    return "\r\n" if "\r\n" in s else "\n"


def run(cmd: list[str], cwd: Path | None = None, env: dict | None = None) -> str:
    r = subprocess.run(
        cmd, cwd=str(cwd or ROOT), env=env,
        capture_output=True, text=True, encoding="utf-8", errors="replace",
    )
    if r.returncode != 0:
        die(f"命令失败：{' '.join(cmd)}\n{r.stdout}{r.stderr}")
    return r.stdout.strip()


# ---- 读 CHANGELOG ----------------------------------------------------------


def read_state() -> tuple[tuple[int, int, int], str, list[str]]:
    """返回 (最近发布号, 未发布区正文, 未发布区里的日期小节列表)"""
    text = read_text(CHANGELOG).replace("\r\n", "\n")

    m = RE_HEAD.search(text)
    base = (int(m.group(1)), int(m.group(2)), int(m.group(3))) if m else (
        tuple(int(x) for x in DEFAULT_VER.split("."))  # type: ignore[assignment]
    )

    u = RE_UNDONE.search(text)
    if not u:
        die("CHANGELOG 顶部没有 `## 未发布` 区")
    rest = text[u.end():]
    nxt = RE_NEXT_H2.search(rest)
    undone = rest[: nxt.start()] if nxt else rest

    return base, undone, RE_DAY.findall(undone)


def bump(base: tuple[int, int, int], tier: int) -> tuple[int, int, int]:
    a, b, c = base
    if tier == 1:
        return (a, b, c + 1)
    if tier == 2:
        return (a, b + 1, 0)
    return (a + 1, 0, 0)


# ---- 前置检查 --------------------------------------------------------------


def pick_go() -> str:
    cands = [shutil.which("go"), "/snap/go/current/bin/go",
             "/usr/local/go/bin/go", "/usr/lib/go/bin/go"]
    for c in cands:
        if not c:
            continue
        try:
            subprocess.run([c, "version"], capture_output=True, check=True)
            return c
        except (OSError, subprocess.CalledProcessError):
            continue
    die("未找到可用的 go，请先安装 Go 并加入 PATH")


def precheck() -> None:
    if run(["git", "status", "--porcelain"]):
        die("工作区有未提交的改动，先提交或还原再发布")
    for p in (MOD, DOCS_SRC, CFG / "keywords_dangerous.conf",
              CFG / "SSHFleet-for-windows.conf", CFG / "SSHFleet-for-linux.conf"):
        if not p.exists():
            die(f"找不到：{p}")
    if not list(DOCS_SRC.glob("[0-9]-*.md")):
        die(f"手册目录里没有「数字-名称.md」形态的文件：{DOCS_SRC}")
    if not MAIN_GO.exists():
        die(f"找不到入口文件：{MAIN_GO}")


# ---- 落地：改文件、提交、打 tag --------------------------------------------


def seal_changelog(version: str, today: str) -> None:
    """## 未发布 → ## [x.y.z] - 日期，并在其上插入新的 ## 未发布"""
    raw = read_text(CHANGELOG)
    eol = eol_of(raw)
    text = raw.replace("\r\n", "\n")
    u = RE_UNDONE.search(text)
    if not u:
        die("CHANGELOG 顶部没有 `## 未发布` 区")
    head = text[: u.start()]
    tail = text[u.end():]
    new = f"## [{version}] - {today}\n"
    write_text(CHANGELOG, (head + "## 未发布\n\n" + new + tail).replace("\n", eol))


def set_app_version(version: str) -> None:
    src = read_text(MAIN_GO)
    if not RE_APPVER.search(src):
        die("main.go 里找不到 appVersion 常量")
    write_text(MAIN_GO, RE_APPVER.sub(f'const appVersion = "{version}"', src, count=1))


def commit_and_tag(version: str) -> None:
    run(["git", "add", str(CHANGELOG.relative_to(ROOT)), str(MAIN_GO.relative_to(ROOT))])
    run(["git", "commit", "-m", f"chore(发布): 定版 {version}"])
    run(["git", "tag", f"v{version}"])


# ---- 编译与打包 ------------------------------------------------------------


def build_bins(go: str) -> None:
    (OUT / "tmp").mkdir(parents=True, exist_ok=True)
    env = dict(os.environ)
    for k in ("GOTMPDIR", "TMPDIR", "TMP", "TEMP"):
        env[k] = str(OUT / "tmp")

    step("[1/2] 编译 windows/amd64 ...")
    e = dict(env, GOOS="windows", GOARCH="amd64")
    run([go, "build", "-o", str(OUT / "SSHFleet.exe"), "."], cwd=MOD, env=e)
    step(f"[完成] {OUT / 'SSHFleet.exe'}")

    step("[2/2] 交叉编译 linux/amd64 ...")
    e = dict(env, GOOS="linux", GOARCH="amd64")
    run([go, "build", "-o", str(OUT / "SSHFleet"), "."], cwd=MOD, env=e)
    step(f"[完成] {OUT / 'SSHFleet'}")


def stage_pkg(pkg: Path, platform: str) -> None:
    (pkg / "config").mkdir(parents=True, exist_ok=True)
    (pkg / "docs").mkdir(parents=True, exist_ok=True)

    for f in ("README.md", "CHANGELOG.md"):
        shutil.copy2(ROOT / f, pkg / f)
    for f in ("keywords_dangerous.conf", "keywords_error.conf", "keywords_passwd.conf"):
        shutil.copy2(CFG / f, pkg / "config" / f)
    for f in DOCS_SRC.glob("[0-9]-*.md"):
        shutil.copy2(f, pkg / "docs" / f.name)

    binname = "SSHFleet.exe" if platform == "windows" else "SSHFleet"
    shutil.copy2(OUT / binname, pkg / binname)
    # 出厂配置按平台取模板；工作区里那份 config/SSHFleet.conf 是作者本机在用的，不随包发布
    shutil.copy2(CFG / f"SSHFleet-for-{platform}.conf", pkg / "config" / "SSHFleet.conf")


def make_zip(src: Path, dst: Path) -> None:
    root = src.parent
    count = 0
    with zipfile.ZipFile(dst, "w", zipfile.ZIP_DEFLATED) as z:
        for dirpath, dirnames, filenames in os.walk(src):
            dirnames.sort()
            filenames.sort()
            arc = os.path.relpath(dirpath, root).replace(os.sep, "/")
            z.write(dirpath, arc + "/")  # 目录项先写，否则部分解压工具建不出空目录
            for fn in filenames:
                z.write(os.path.join(dirpath, fn), arc + "/" + fn)
                count += 1
    step(f"  归档 {count} 个文件（顶层目录 {src.name}/）")


def _add_tree(tar: tarfile.TarFile, path: Path, arcname: str) -> None:
    ti = tar.gettarinfo(str(path), arcname)
    ti.mode = 0o755  # 固定权限，不依赖源文件当前状态；文本文件跟着 755 无害
    if ti.isdir():
        tar.addfile(ti)
        for p in sorted(path.iterdir()):
            _add_tree(tar, p, arcname + "/" + p.name)
    else:
        with open(path, "rb") as f:
            tar.addfile(ti, f)


def make_targz(src: Path, dst: Path) -> None:
    with tarfile.open(dst, "w:gz") as tar:
        _add_tree(tar, src, src.name)


def check_size(*paths: Path) -> None:
    for p in paths:
        if not p.exists() or p.stat().st_size <= 1000:
            die(f"产物异常（不存在或过小）：{p}")


def package(go: str, version: str, ts: str) -> list[Path]:
    # 先编译再清 release：编译失败时上一次的产物还在，不必从零重来
    build_bins(go)

    if REL.exists():
        # 清空前确认 release 在工作区内：rm 一旦作用到工作区外就是不可逆的
        try:
            REL.resolve().relative_to(ROOT.resolve())
        except ValueError:
            die(f"release 目录不在工作区内，拒绝清空：{REL}")
        shutil.rmtree(REL)
    REL.mkdir(parents=True)

    made: list[Path] = []
    for plat in PLATFORMS:
        name = f"{APP_NAME}_{version}_{ts}_{plat}"
        pkg = REL / name
        step(f"[发布] 组装 release/{name} ...")
        stage_pkg(pkg, plat)
        if plat == "windows":
            dst = REL / f"{name}.zip"
            step(f"[发布] 打包 {dst.name} ...")
            make_zip(pkg, dst)
        else:
            dst = REL / f"{name}.tar.gz"
            step(f"[发布] 打包 {dst.name} ...")
            make_targz(pkg, dst)
        check_size(dst)
        made.append(dst)
    return made


# ---- 交互 ------------------------------------------------------------------


# 档位怎么选（原先写在规范「四·2」里，随版本号规则一起搬到这里，规范里不再重复）：
#   只问一句——升级之后，使用者拿着他原来的配置、原来的命令，还能不能照旧跑起来？
#   判的是使用者的东西（配置文件、命令行参数、脚本／清单），改代码内部结构不算。
#   算：删掉或改名一个选项／配置字段；配置里新增必填字段；某字段取值语义变了；
#       默认行为变了、旧命令跑出不同结果；规则文件改名或换了查找位置；
#       输出结构变了而使用者的脚本要解析它；目录／文件名改了而他写进了自己的脚本。
#   不算：内部重构、包结构调整、函数改名；新增选项或新增可选配置字段；
#       只改报错文案、日志措辞、界面排版；修 bug 而修的是错的行为；补文档。
#   拿不准按不兼容算——多抬一次主版本只是号大一点，漏抬一次则使用者的东西坏了没提示。
def choose_tier(base: tuple[int, int, int]) -> int:
    b = ".".join(map(str, base))
    print(f"\n最近发布：v{b}")
    print("这次是什么档位？")
    for i, (label, desc) in enumerate([
        ("修订  ", "只有修正"),
        ("次版本", "有新增"),
        ("主版本", "不兼容：老的配置或命令跑不起来了"),
    ], start=1):
        n = ".".join(map(str, bump(base, i)))
        print(f"  {i}) {label} → {n}   {desc}")
    s = input("> ").strip()
    if s not in ("1", "2", "3"):
        die(f"没看懂这个档位：{s}")
    return int(s)


def main() -> None:
    ap = argparse.ArgumentParser(add_help=False)
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("-h", "--help", action="store_true")
    args = ap.parse_args()
    if args.help:
        print(__doc__)
        return

    try:
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")  # type: ignore[attr-defined]
    except Exception:
        pass

    base, undone, days = read_state()
    items = len(RE_ITEM.findall(undone))
    if items == 0 and not args.dry_run:
        if undone.strip():
            die("`## 未发布` 区里有内容，但没有一条以 `- ` 开头的条目。\n"
                "      写法：`### yyyy-MM-dd` 单独一行写日期，条目一律以 `- ` 开头。")
        die("`## 未发布` 区里没有条目，没东西可发")

    precheck()
    go = pick_go() if not args.dry_run else "go"

    span = f"，覆盖 {len(days)} 天" if days else ""
    print(f"\n未发布区：{items} 条{span}")
    tier = choose_tier(base)

    a, b, c = bump(base, tier)
    version = f"{a}.{b}.{c}"
    today = datetime.now().strftime("%Y-%m-%d")

    print(f"\n将发布 v{version} —— 回车执行，其他任意键取消")
    if input("> ").strip() != "":
        print("已取消，什么都没改。")
        return

    if args.dry_run:
        print("\n[dry-run] 只演不练，以下动作未执行：")
        print(f"  封段：## 未发布 → ## [{version}] - {today}，并在其上插入新的 ## 未发布")
        print(f"  改号：main.go 的 appVersion → {version}")
        print(f"  提交：chore(发布): 定版 {version}")
        print(f"  打 tag：v{version}")
        print(f"  打包：release/{APP_NAME}_{version}_<时间戳>_windows.zip / _linux.tar.gz")
        return

    ts = datetime.now().strftime("%Y-%m-%d_%H-%M-%S")

    seal_changelog(version, today)
    set_app_version(version)
    commit_and_tag(version)
    step(f"\n[完成] 已提交并打 tag v{version}")

    made = package(go, version, ts)

    print(f"\n全部完成，发布产物（{REL}）：")
    for p in made:
        print(f"  {p.name}  {p.stat().st_size / 1024 / 1024:.1f} MB")
    print(f"[完成] 已打 tag v{version}（仅本地，未推送）")


if __name__ == "__main__":
    main()
