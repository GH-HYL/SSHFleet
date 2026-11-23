#!/usr/bin/env bash
# 提交前的闸门。提交那一步由 tools/hooks/pre-commit 转进来（传 --all），也可直接跑。
#
# 五样检查，各有一个开关；不带参数或 --all 就按改动把该跑的都跑：
#   --docs   文档检查，四样字符串级判断，判据见 docs/agents/按需-写文档.md：
#            ① 引用指向不存在的 docs 路径或目录（写路径等于承诺它明天还在）
#            ② 新写的 ADR 有「决策人」段却没有身份标注（证据与要求混排）
#            ③ 写了「共 N 条」这类会被正文证伪的计数
#            ④ 本次新增的行里写了「第 X 节」「详见 <路径>」（给人指路的写法）
#   --fmt    格式：gofmt -l internal
#   --vet    静态检查：go vet ./...
#   --guard  结构总闸：TestProjectIsClean 那条「真工程整跑」，一次把包清单、
#            依赖方向、红线指纹、放行台账全数一遍，是这套约束的总开关。
#
# 中途自查：裸跑 `bash tools/check.sh` 会自己挑档——改动里没有 .go 就只跑文档那档，
# 有 .go 才过 gofmt / vet / 结构检查；只想查一样就 `bash tools/check.sh --docs`（最快）。
# go vet 与结构检查在 /mnt/c 上各要几秒，那不是代码慢，是 WSL 读 Windows 盘的成本。
#
# 全量跑哪几档：提交态看这次提交里有没有 .go；手动跑看工作区里有没有 .go 的改动。
# 没有就只跑文档那档——那几档的结论跟上次过闸时一样。一处改动都没有时按验状态全跑，
# 纯文档提交因此不必等 go vet 那几十秒。显式点名单项时按点的跑，不套这条判断。
#
# 文档查哪些文件：提交时只查本次改动的，手动跑时查全部 tracked。
# 只查改动的，是为了不让历史欠账把新提交堵死。
#
# 两种情形都先过一道跳过名单：skills/、mattpocock-skills/、.dsh/、tools/、personal/、
# docs/agents/ 六个整目录，加上根目录的 AGENTS.md，一律不查——约束类文件指名道姓写路径，
# 不受这四条纪律管。名单只有 doc_scope_filter 那一处，改名单改那里。
#
# ①③ 查整个文件，手动跑也查；②④ 只在提交时做——② 看本次新增的 ADR 文件，
# ④ 只看本次新增的行。存量里「第 X 节」有几十处，按整份文件判会让碰一下就红。
#
# 为什么不放进 tools/test.sh：那是跑测试的入口，参数要原样交给 go test；
# 这里是固定几样、只认下面那几个开关，两件事别混在一个脚本里。

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/.." && pwd)"

sandbox="$root/build/gotmp-$(date +%Y%m%d)"
mkdir -p "$sandbox"
export GOTMPDIR="$sandbox"
export TMPDIR="$sandbox"
export TMP="$sandbox"
export TEMP="$sandbox"

usage() {
	cat <<'EOF'
用法：bash tools/check.sh [开关 ...]

不带开关 / --all   按改动挑档：文档检查照跑，改动里有 .go（或什么都没改）才跑
                   gofmt / go vet / 结构检查（提交钩子走这条）
--docs               只查文档四条
--fmt                只查 gofmt
--vet                只查 go vet
--guard              只查结构检查

开关可以叠加，按固定顺序跑：bash tools/check.sh --docs --vet
EOF
}

want_docs=""
want_fmt=""
want_vet=""
want_guard=""
want_all=""
for arg in "$@"; do
	case "$arg" in
	--docs) want_docs=1 ;;
	--fmt) want_fmt=1 ;;
	--vet) want_vet=1 ;;
	--guard) want_guard=1 ;;
	--all) want_all=1 ;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "闸门：不认识的开关「$arg」；跑 bash tools/check.sh --help 看能传什么" >&2
		exit 2
		;;
	esac
done
if [ -z "$want_docs$want_fmt$want_vet$want_guard" ]; then
	want_all=1
fi

# 暂存区一次问清：哪几个文件动了、动的是文档还是代码、有没有新写的 ADR。
# 原来分三次问 git，一次一百多毫秒，在 /mnt/c 上更贵。
staged_any=""
staged_docs=""
staged_go=""
staged_added_adr=""
while IFS=$'\t' read -r st p1 p2; do
	[ -n "${st:-}" ] || continue
	case "$st" in
	R* | C*) p="$p2" ;;
	*) p="$p1" ;;
	esac
	[ -n "$p" ] || continue
	staged_any+="$p"$'\n'
	case "$st" in
	A* | C* | M* | R*)
		case "$p" in
		*.md | *.txt) staged_docs+="$p"$'\n' ;;
		esac
		case "$p" in
		*.go) staged_go+="$p"$'\n' ;;
		esac
		;;
	esac
	case "$st" in
	A*)
		case "$p" in
		docs/adr/0*.md) staged_added_adr+="$p"$'\n' ;;
		esac
		;;
	esac
done <<<"$(git -C "$root" -c core.quotepath=false diff --cached --name-status || true)"

# 问 git「工作区里还有什么改动」。它要给全仓每个文件做一次 stat，在 /mnt/c 上固定三秒，
# 所以只有手动跑、手里没有「这次提交」可看时才问。删掉 .go 不算改了 .go（照旧）；
# 一条改动都没有时这里留空，那时按验状态跑全档。
touched_any=""
touched_go=""
touched_ask() {
	touched_add() {
		touched_any+="$1"$'\n'
		case "$1" in *.go) touched_go+="$1"$'\n' ;; esac
	}
	local ent xy p ren=""
	while IFS= read -r -d '' ent; do
		# 改名在 -z 里占两条记录，第二条是原路径，跳过。
		if [ -n "$ren" ]; then
			ren=""
			continue
		fi
		xy="${ent:0:2}"
		p="${ent:3}"
		[ -n "$p" ] || continue
		case "$xy" in R* | C*) ren=1 ;; esac
		case "$xy" in
		'??') touched_add "$p" ;;
		*)
			case "${xy:0:1}" in A | C | M | R) touched_add "$p" ;; esac
			case "${xy:1:1}" in A | C | M | R) touched_add "$p" ;; esac
			;;
		esac
	done < <(git -C "$root" -c core.quotepath=false status --porcelain=v1 -z -uall 2>/dev/null || true)
}

doc_scope_empty=""
doc_check() {
	# 谁不进文档检查：技能正文（路径常是举例）、脚本与个人工具（不承载文档）、
	# 以及约束类文件（照 docs/agents/按需-写文档.md，它们指名道姓写路径，不受这些纪律管）。
	doc_scope_filter() {
		grep -vE '^(skills|mattpocock-skills|\.dsh|tools|personal|docs/agents)/|^AGENTS\.md$' || true
	}

	local scope
	if [ -n "$staged_any" ]; then
		scope="$(printf '%s' "$staged_docs" | doc_scope_filter)"
	else
		scope="$(git -C "$root" -c core.quotepath=false ls-files '*.md' '*.txt' | doc_scope_filter)"
	fi
	scope="$(printf '%s\n' "$scope" | grep -v '^$' || true)"
	if [ -z "$scope" ]; then
		doc_scope_empty=1
		return 0
	fi

	local -a files=()
	local f
	while IFS= read -r f; do
		[ -n "$f" ] || continue
		[ -f "$root/$f" ] || continue
		files+=("$root/$f")
	done <<<"$scope"

	local problems="" line p rest n
	if [ "${#files[@]}" -gt 0 ]; then
		# ① 引用：一次 grep 扫完全部文件，不再一个文件开三个进程。
		while IFS= read -r line; do
			[ -n "$line" ] || continue
			f="${line%%:*}"
			f="${f#"$root"/}"
			p="${line#*:}"
			case "$p" in *'<'* | *'>'* | *'*'*) continue ;; esac
			[ -e "$root/$p" ] || problems+="引用了不存在的路径：$f → $p"$'\n'
		done <<<"$(printf '%s\0' "${files[@]}" | xargs -0 grep -oH -E 'docs/[^[:space:]`"'"'"'）)]+\.md' 2>/dev/null || true)"

		# 目录级引用单独捞一遍：正则以 .md 结尾，指整份议题目录的写法全漏。
		while IFS= read -r line; do
			[ -n "$line" ] || continue
			f="${line%%:*}"
			f="${f#"$root"/}"
			p="${line#*:}"
			case "$p" in *'<'* | *'>'* | *'*'*) continue ;; esac
			[ -d "$root/${p%/}" ] || problems+="引用了不存在的目录：$f → $p"$'\n'
		done <<<"$(printf '%s\0' "${files[@]}" | xargs -0 grep -oH -E 'docs/[^[:space:]`"'"'"'）)，。；、]+/' 2>/dev/null || true)"

		# ③ 计数。
		while IFS= read -r line; do
			[ -n "$line" ] || continue
			f="${line%%:*}"
			f="${f#"$root"/}"
			rest="${line#*:}"
			n="${rest%%:*}"
			problems+="写了会被正文证伪的计数：$f:$n"$'\n'
		done <<<"$(printf '%s\0' "${files[@]}" | xargs -0 grep -nH -E '共[[:space:]]*[0-9]+[[:space:]]*条|共[[:space:]]*[一二三四五六七八九十]+[[:space:]]*条' 2>/dev/null || true)"
	fi

	# ④ 只判本次新增的行：给人指路的两种写法。手动跑没有新增行，这段不跑。
	if [ -n "$staged_any" ]; then
		local added newpos
		while IFS= read -r f; do
			[ -n "$f" ] || continue
			[ -f "$root/$f" ] || continue
			added="$(git -C "$root" -c core.quotepath=false diff --cached -U0 -- "$f" 2>/dev/null |
				grep -E '^\+' | grep -vE '^\+\+\+' || true)"
			[ -n "$added" ] || continue
			newpos="$(printf '%s\n' "$added" | grep -E '第[[:space:]]*[0-9一二三四五六七八九十]+[[:space:]]*节|见上表第[[:space:]]*[0-9几]+[[:space:]]*行|(详见|依据见)[^。；]{0,20}(docs/|[A-Za-z0-9_./-]+\.md|`[^`]+`)' || true)"
			while IFS= read -r line; do
				[ -n "$line" ] || continue
				problems+="新增的行按位置/路径指路：$f：${line#+}"$'\n'
			done <<<"$newpos"
		done <<<"$scope"
	fi

	# ② 身份标注只查新写的 ADR；存量按碰到再补，不追溯。
	while IFS= read -r f; do
		[ -n "$f" ] || continue
		[ -f "$root/$f" ] || continue
		if grep -q '决策人' "$root/$f" && ! grep -q '是证据，不是要求' "$root/$f"; then
			problems+="新 ADR 缺身份标注：$f"$'\n'
		fi
	done <<<"$staged_added_adr"

	if [ -n "$problems" ]; then
		printf '%s' "$problems" >&2
		echo "文档检查没过。判据见 docs/agents/按需-写文档.md。" >&2
		return 1
	fi
	return 0
}

run_fmt() {
	cd "$root/modules/SSHFleet2_Go"
	local dirty
	dirty="$(gofmt -l internal)"
	if [ -n "$dirty" ]; then
		echo "闸门没过：这些文件没格式化" >&2
		echo "$dirty" >&2
		return 1
	fi
}

run_vet() {
	cd "$root/modules/SSHFleet2_Go"
	go vet ./... >/dev/null || {
		echo "闸门没过：go vet 有意见（上面就是）" >&2
		return 1
	}
}

run_guard() {
	cd "$root/modules/SSHFleet2_Go"
	# -vet=off：go test 自带的那遍 vet 只覆盖本包一小撮分析器，全量 go vet 已经扫过，
	# 这一遍是重算，白等一到三秒。
	go test -count=1 -vet=off -run TestProjectIsClean ./internal/guard/ >/dev/null || {
		echo "闸门没过：结构检查红（跑 bash tools/test.sh -run TestProjectIsClean ./internal/guard/ -v 看明细）" >&2
		return 1
	}
}

if [ -n "$want_all" ]; then
	if ! doc_check; then
		exit 1
	fi

	# 提交态看这次提交里有没有 .go——工作区里没暂存的 .go 不进这次提交，不归它管，
	# 也就不必花三秒去问。手动跑没有「这次提交」可看，按工作区的实际改动判；
	# 一条改动都没有（新机器 clone 下来验一遍）时不跳，按验状态跑全档。
	skip_go=""
	if [ -n "$staged_any" ]; then
		if [ -z "$staged_go" ]; then
			skip_go=1
		fi
	else
		touched_ask
		if [ -n "$touched_any" ] && [ -z "$touched_go" ]; then
			skip_go=1
		fi
	fi
	if [ -n "$skip_go" ]; then
		if [ -n "$doc_scope_empty" ]; then
			echo "闸门：本次改动没有要查的文档（技能正文、个人工具、约束类文件不在范围内），代码那段跳过"
		else
			echo "闸门：文档检查过了（本次没改 .go，代码那段跳过）"
		fi
		exit 0
	fi

	run_fmt || exit 1
	run_vet || exit 1
	run_guard || exit 1
	echo "闸门：文档检查 / gofmt / go vet / 结构检查 都过了"
	exit 0
fi

# 只跑点名的几档：按固定顺序，谁红就停在谁那儿。
ran=""
if [ -n "$want_docs" ]; then
	if ! doc_check; then
		exit 1
	fi
	ran="文档检查"
fi
if [ -n "$want_fmt" ]; then
	run_fmt || exit 1
	ran="${ran:+$ran / }gofmt"
fi
if [ -n "$want_vet" ]; then
	run_vet || exit 1
	ran="${ran:+$ran / }go vet"
fi
if [ -n "$want_guard" ]; then
	run_guard || exit 1
	ran="${ran:+$ran / }结构检查"
fi
echo "闸门：$ran 过了（这次只跑这几档；提交时跑全量）"
