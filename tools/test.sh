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
	# Windows 侧：交给 WSL 里同一个脚本跑。
	#
	# 不做路径换算——`wsl --cd` 直接收 Windows 形状的路径（`C:/…` 或 `C:\…` 都行），
	# 进去就是仓库根。以前拿 sed 自己换，`/c/…` 与 `C:/…` 只认一种，认错时报的是
	# 「bash: …: No such file or directory」，看着像文件没了。
	#
	# 关掉 MSYS 的路径改写：不关的话，`--cd` 后面那个路径会被改成 `C:/Program Files/Git/…`。
	export MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL='*'
	exec wsl.exe -d "${SSHFLEET_WSL_DISTRO:-Ubuntu-26.04}" --cd "$(cygpath -w "$root")" -- bash tools/test.sh "$@"
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
