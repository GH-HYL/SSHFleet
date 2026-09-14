#!/usr/bin/env bash
# SSHFleet 构建脚本（Linux / 自动化用；置于工作区根）
#   编译 linux/amd64 + windows/amd64，产物落工作区根 build/
#   带 release 参数：额外组装发布目录并打 tar.gz（release/SSHFleet_<版本>_linux.tar.gz）
set -euo pipefail

# 工作区根按脚本自身位置定位：曾用相对路径 `../build`，在 cd 之后实际解析成
# modules/build，产物落深一层（2026-09-14 修）
ROOT="$(cd "$(dirname "$0")" && pwd)"

# Git Bash（MSYS）下 pwd 给的是 /d/… 形式，而 go.exe 是原生 Windows 程序，会把 /d/…
# 当成「当前盘根下的 d 目录」写成 D:\d\…（M1 踩过一次，改脚本时又踩一次）。
# 有 cygpath 就转成 D:/… 混合形式交给 go；真实 Linux 上没有 cygpath，直接用 ROOT。
if command -v cygpath >/dev/null 2>&1; then
    OUT_ROOT="$(cygpath -m "$ROOT")"
else
    OUT_ROOT="$ROOT"
fi

cd "$ROOT/modules/SSHFleet_Go"
OUT="$OUT_ROOT/build"
mkdir -p "$OUT"

echo "[1/2] 编译 linux/amd64 ..."
GOOS=linux GOARCH=amd64 go build -o "$OUT/SSHFleet" .

echo "[2/2] 交叉编译 windows/amd64 ..."
GOOS=windows GOARCH=amd64 go build -o "$OUT/SSHFleet.exe" .

echo "构建完成：$OUT/"
ls -la "$OUT"

# ---- release 模式：组发布目录 + 打 tar.gz ----------------------------------
# Windows 的包由 build-windows.bat release 负责（zip）；各平台的包在各自平台上打，
# 权限与容器格式都最可靠（Windows 的 bsdtar 打不出带执行权限的 tar.gz）。
if [ "${1:-}" != "release" ]; then
    exit 0
fi

# 版本号取自 CHANGELOG.md 顶部第一个正式版本号（形如 `## [5.0.0]`）；取不到用当天日期
VER="$(grep -m1 -oE '^## \[[0-9]+\.[0-9]+\.[0-9]+\]' "$ROOT/CHANGELOG.md" 2>/dev/null | tr -d '[]# ' || true)"
if [ -z "$VER" ]; then
    VER="$(date +%Y-%m-%d)"
    echo "未从 CHANGELOG.md 取到正式版本号，改用日期：$VER"
fi

NAME="SSHFleet_${VER}_linux"
PKGDIR="$ROOT/release/$NAME"
rm -rf "$PKGDIR"
mkdir -p "$PKGDIR/config"

echo "[发布] 组装 release/$NAME ..."
cp "$OUT/SSHFleet" "$PKGDIR/"
cp "$ROOT/README.md" "$PKGDIR/"
cp "$ROOT/CHANGELOG.md" "$PKGDIR/"
cp "$ROOT/modules/SSHFleet_Go/config/SSHFleet.conf" "$PKGDIR/config/"
cp "$ROOT/modules/SSHFleet_Go/config/dangerous_keywords.toml" "$PKGDIR/config/"
cp "$ROOT/modules/SSHFleet_Go/config/error_keywords.toml" "$PKGDIR/config/"

# 权限：真实 Linux 上 tar 天然保留 0755；Git Bash（MSYS）下 chmod 对 NTFS 无效，
# 用 GNU tar 的 --mode 兜底（发布树只有可执行文件 + README/CHANGELOG + config，
# 统一 755 对目录是必需的、对文本文件无害）。非 GNU tar 不支持该选项，故先探测。
TAR_MODE=()
if tar --help 2>&1 | grep -q -- '--mode'; then
    TAR_MODE=(--mode=755)
fi
tar "${TAR_MODE[@]}" -czf "$ROOT/release/${NAME}.tar.gz" -C "$ROOT/release" "$NAME"

echo "[发布] 完成：release/${NAME}.tar.gz"
tar -tzvf "$ROOT/release/${NAME}.tar.gz"
