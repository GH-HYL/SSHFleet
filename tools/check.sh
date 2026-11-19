#!/usr/bin/env bash
# 提交前的闸门。提交那一步由 tools/hooks/pre-commit 转进来，也可直接跑。
#
# 两段：
#   一、文档检查——三样字符串级判断，判据见 docs/agents/doc-guide.md 的「零」：
#       ① 引用指向不存在的 docs 路径（写路径等于承诺它明天还在）
#       ② 新写的 ADR 有「决策人」段却没有身份标注（证据与要求混排）
#       ③ 写了「共 N 条」这类会被正文证伪的计数
#   二、代码闸门——格式、静态检查、结构总闸。结构总闸就是那条「真工程整跑」，
#       一次把包清单、依赖方向、红线指纹、放行台账全数一遍，是这套约束的总开关。
#
# 跑哪段：暂存区只有 .md 就跑第一段；有 .go、或什么都没暂存（手动跑）就跑两段。
# 纯文档提交因此不必等 go vet 那几十秒。
#
# 文档查哪些文件：提交时只查本次改动的，手动跑时查全部 tracked。
# 只查改动的，是为了不让历史欠账把新提交堵死。
#
# 为什么不放进 tools/test.sh：那是跑测试的入口，参数要原样交给 go test；
# 这里是固定几样、不许带参数，两件事别混在一个脚本里。

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/.." && pwd)"

sandbox="$root/build/gotmp-$(date +%Y%m%d)"
mkdir -p "$sandbox"
export GOTMPDIR="$sandbox"
export TMP="$sandbox"
export TEMP="$sandbox"

staged_all="$(git -C "$root" -c core.quotepath=false diff --cached --name-only)"
staged_md="$(git -C "$root" -c core.quotepath=false diff --cached --name-only --diff-filter=ACM -- '*.md')"

doc_check() {
	# 谁不进文档检查：技能正文（路径常是举例）、脚本与个人工具（不承载文档）、
	# 以及约束类文件（照 doc-guide 零节，它们指名道姓写路径，不受本节管）。
	doc_scope_filter() {
		grep -vE '^(skills|mattpocock-skills|\.dsh|tools|personal|docs/agents)/|^(AGENTS\.md|docs/个人开发规范\.md)$' || true
	}

	local scope
	if [ -n "$staged_all" ]; then
		scope="$(printf '%s\n' "$staged_md" | doc_scope_filter)"
	else
		scope="$(git -C "$root" -c core.quotepath=false ls-files '*.md' | doc_scope_filter)"
	fi
	scope="$(printf '%s\n' "$scope" | grep -v '^$' || true)"
	[ -n "$scope" ] || return 0

	local problems="" f p line refs hits
	while IFS= read -r f; do
		[ -n "$f" ] || continue
		[ -f "$root/$f" ] || continue

		refs="$(grep -oE 'docs/[^[:space:]`"'"'"'）)]+\.md' "$root/$f" 2>/dev/null || true)"
		while IFS= read -r p; do
			[ -n "$p" ] || continue
			case "$p" in *'<'* | *'>'* | *'*'*) continue ;; esac
			[ -e "$root/$p" ] || problems+="引用了不存在的路径：$f → $p"$'\n'
		done <<<"$refs"

		hits="$(grep -nE '共[0-9]+条|共[一二三四五六七八九十]+条' "$root/$f" 2>/dev/null || true)"
		while IFS= read -r line; do
			[ -n "$line" ] || continue
			problems+="写了会被正文证伪的计数：$f:${line%%:*}"$'\n'
		done <<<"$hits"
	done <<<"$scope"

	# 身份标注只查新写的 ADR；存量按碰到再补，不追溯。
	if [ -n "$staged_all" ]; then
		while IFS= read -r f; do
			[ -n "$f" ] || continue
			[ -f "$root/$f" ] || continue
			case "$f" in
			docs/adr/0*.md) ;;
			*) continue ;;
			esac
			if grep -q '决策人' "$root/$f" && ! grep -q '是证据，不是要求' "$root/$f"; then
				problems+="新 ADR 缺身份标注：$f"$'\n'
			fi
		done <<<"$(git -C "$root" -c core.quotepath=false diff --cached --name-only --diff-filter=A -- 'docs/adr/0*.md')"
	fi

	if [ -n "$problems" ]; then
		printf '%s' "$problems" >&2
		echo "文档检查没过。判据见 docs/agents/doc-guide.md 的「零」。" >&2
		return 1
	fi
	return 0
}

if ! doc_check; then
	exit 1
fi

# 只改了文档就到此为止，不进 Go 那一段。删掉 .go 不算改了 .go——那不用过 vet。
if [ -n "$staged_all" ] && [ -z "$(git -C "$root" -c core.quotepath=false diff --cached --name-only --diff-filter=ACMR -- '*.go')" ]; then
	echo "闸门：文档检查过了（本次没改 .go，代码那段跳过）"
	exit 0
fi

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

echo "闸门：文档检查 / gofmt / go vet / 结构检查 都过了"
