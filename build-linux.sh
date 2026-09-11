#!/usr/bin/env bash
# SSHFleet 构建脚本（Linux / 自动化用；置于工作区根）
# 编译 linux/amd64 + windows/amd64，产物落工作区根 build/
set -euo pipefail
cd "$(dirname "$0")/modules/SSHFleet_Go"
OUT=../build
mkdir -p "$OUT"

echo "[1/2] 编译 linux/amd64 ..."
GOOS=linux GOARCH=amd64 go build -o "$OUT/SSHFleet" .

echo "[2/2] 交叉编译 windows/amd64 ..."
GOOS=windows GOARCH=amd64 go build -o "$OUT/SSHFleet.exe" .

echo "构建完成：$OUT/"
ls -la "$OUT"
