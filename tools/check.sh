#!/usr/bin/env bash
# 提交前的闸门，在 WSL 里跑。提交那一步由 tools/hooks/pre-commit 转进来。
#
# 三样：格式、静态检查、结构总闸。结构总闸就是那条「真工程整跑」——
# 它一次把包清单、依赖方向、红线指纹、放行台账全数一遍，是这套约束的总开关。
#
# 为什么不放进 tools/test.sh：那是跑测试的入口，参数要原样交给 go test；
# 这里是固定三样、不许带参数，两件事别混在一个脚本里。

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/.." && pwd)"

sandbox="$root/build/gotmp-$(date +%Y%m%d)"
mkdir -p "$sandbox"
export GOTMPDIR="$sandbox"
export TMP="$sandbox"
export TEMP="$sandbox"

cd "$root/modules/SSHFleet2_Go"

dirty="$(gofmt -l internal)"
if [ -n "$dirty" ]; then
	echo "闸门没过：这些文件没格式化" >&2
	echo "$dirty" >&2
	exit 1
fi

go vet ./... >/dev/null || {
	echo "闸门没过：go vet 有意见（上面就是）" >&2
	exit 1
}

go test -count=1 -run TestProjectIsClean ./internal/guard/ >/dev/null || {
	echo "闸门没过：结构检查红（跑 bash tools/test.sh -run TestProjectIsClean ./internal/guard/ -v 看明细）" >&2
	exit 1
}

echo "闸门：gofmt / go vet / 结构检查 都过了"
