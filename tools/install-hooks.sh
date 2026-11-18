#!/usr/bin/env bash
# 把 tools/hooks/ 下的两个钩子装进 .git/hooks/。换一台机器 clone 下来跑一次即可。
#
# Git for Windows 不看可执行位，文件在场就执行；chmod 只是给 Linux 那边留的。

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/.." && pwd)"
dest="$root/.git/hooks"

for name in pre-commit commit-msg; do
	cp "$here/hooks/$name" "$dest/$name"
	chmod +x "$dest/$name"
	echo "装好：.git/hooks/$name"
done
