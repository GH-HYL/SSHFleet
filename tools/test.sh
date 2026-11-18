#!/usr/bin/env bash
# 跑测试的唯一入口。在 Git Bash 里直接敲 `tools/test.sh ./...`，
# 它会转进 WSL 跑——Windows 侧原生跑会多出软链接特权与探针缺 .exe 两类假红，
# 测试二进制还会落进 %TEMP% 被 360 删或拦（见 docs/测试环境使用指南.md 第八节）。
#
# 干的三件事：转到 WSL、把三个临时目录变量指到工作区沙盒、把参数原样交给 go test。
# 参数就是 go test 的参数：`tools/test.sh ./...`、`tools/test.sh -run TestFoo ./internal/execute/`。

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/.." && pwd)"

case "$(uname -s)" in
MINGW* | MSYS* | CYGWIN*)
	# Windows 侧：换成 WSL 的路径形状（/c/… → /mnt/c/…），再交给同一个脚本在 WSL 里跑。
	#
	# 关掉 MSYS 的路径改写：不关的话，`/mnt/c/…` 会被当成 Unix 路径改写成
	# `C:/Program Files/Git/mnt/c/…`，wsl 里找不到文件。
	export MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL='*'
	wsl_root="$(printf '%s' "$root" | sed -E 's|^/([A-Za-z])/|/mnt/\L\1/|')"
	exec wsl.exe -d "${SSHFLEET_WSL_DISTRO:-Ubuntu-26.04}" -- bash "$wsl_root/tools/test.sh" "$@"
	;;
esac

sandbox="$root/build/gotmp-$(date +%Y%m%d)"
mkdir -p "$sandbox"
export GOTMPDIR="$sandbox"
export TMP="$sandbox"
export TEMP="$sandbox"

cd "$root/modules/SSHFleet2_Go"
if [ "$#" -eq 0 ]; then
	set -- ./...
fi
exec go test -count=1 "$@"
