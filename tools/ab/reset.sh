#!/bin/bash
# 09 号单：把一侧的现场复位。每场开跑前各执行一次——先跑的那侧会改掉后跑那侧的现场。
# 用法：reset.sh old|new
set -eu
SB="${SSHFLEET_AB_SB:-$(cd "$(dirname "$0")" && pwd)}"
side="${1:?用法：reset.sh old|new}"

rm -rf "$SB/$side/config" "$SB/$side/history" "$SB/$side/latest_history" "$SB/$side/down"
rm -rf "$SB/$side"/u1.csv "$SB/$side"/t1.csv "$SB/$side"/two.csv "$SB/$side"/bad.csv
rm -rf "$SB/$side"/hello.sh "$SB/$side"/ask1.sh "$SB/$side"/deploy.xyz "$SB/$side"/updir
rm -rf "$SB/home_$side"
mkdir -p "$SB/home_$side"

cp -r "$SB/pristine/$side" "$SB/$side/config"
cp "$SB"/fixtures/*.csv "$SB"/fixtures/*.sh "$SB"/fixtures/deploy.xyz "$SB"/fixtures/sec.txt "$SB/$side/"
cp -r "$SB/fixtures/updir" "$SB/$side/updir"
mkdir -p "$SB/$side/down"

python3 "$SB/prep.py" "$SB/$side/config/SSHFleet.conf" \
  port=2201 'user="sshfleet"' 'password="Sshfleet#2026"' sudo=false encrypt=false
