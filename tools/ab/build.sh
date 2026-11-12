#!/bin/bash
# 比对口径议题（2026-11-10-1615）：两侧二进制按当前代码重编。两侧同名 sshfleet，省一条归一化。
#
# 沙盒根自己算：09 那把脚本里写死的是 09 的沙盒路径，照抄过来会编到旧沙盒去（本轮踩过一次）。
set -eu
CODE="$(cd "$(dirname "$0")" && pwd)"
SB="${SSHFLEET_AB_SB:-$CODE}"
# 仓库根从脚本位置算，不再写死路径（脚本在 <仓库>/tools/ab）
ROOT="$(cd "$CODE/../.." && pwd)"
export HOME=/home/ubuntu
export GOPROXY=off
export GOFLAGS=-buildvcs=false
export GOTMPDIR="$SB/gotmp"
GO=/home/ubuntu/.local/opt/go/bin/go

mkdir -p "$SB/gotmp" "$SB/old" "$SB/new"

cd "$ROOT/modules/SSHFleet_Go"
"$GO" build -o "$SB/old/sshfleet" .
cd "$ROOT/modules/SSHFleet2_Go"
"$GO" build -o "$SB/new/sshfleet" .

ls -l "$SB/old/sshfleet" "$SB/new/sshfleet"
