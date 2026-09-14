#!/usr/bin/env bash
# SSHFleet 构建脚本（Linux / 自动化用；置于工作区根）
# 编译 linux/amd64 + windows/amd64，产物落工作区根 build/
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
