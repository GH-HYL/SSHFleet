#!/bin/bash
# 09 号单：一条命令跑场景表。默认只跑「不碰机器」的那些，--all 才连要机器的也跑。
# 用法：run.sh [--all] [场景名…]
set -u
CODE="$(cd "$(dirname "$0")" && pwd)"
# 脚本住在 tools/ab，现场（两侧二进制、配置、home、out）另指一处：
#   SSHFLEET_AB_SB=<现场目录> tools/ab/run.sh
# 不给就当场跑，行为与脚本放在现场时一样。
SB="${SSHFLEET_AB_SB:-$CODE}"
ALL=0
wanted=()
for a in "$@"; do
  case "$a" in
    --all) ALL=1 ;;
    *) wanted+=("$a") ;;
  esac
done

pick() {
  [ ${#wanted[@]} -eq 0 ] && return 0
  for w in "${wanted[@]}"; do [ "$w" = "$1" ] && return 0; done
  return 1
}

fail=0
while IFS=$'\x1f' read -r name machine stdin conf pre reset env kill argv expect tty; do
  case "$name" in ""|\#*) continue ;; esac
  pick "$name" || continue

  if [ "$machine" = "1" ] && [ "$ALL" -ne 1 ]; then
    echo "$(printf '%-24s' "$name") 跳过（要机器，加 --all）"
    continue
  fi
  case "$pre" in
    ticket:*) echo "$(printf '%-24s' "$name") 跳过（状态待 ${pre#ticket:} 号单）"; continue ;;
  esac

  args=()
  [ -n "$stdin" ] && args+=(--stdin "$stdin")
  [ -n "$kill" ] && args+=(--kill-after "$kill")
  # conf 一栏可以写多组（空格分开）：加密那几场既要开 encrypt 又要给凭据绝对路径。
  if [ -n "$conf" ]; then
    for one in $conf; do args+=(--conf "$one"); done
  fi
  [ -n "$pre" ] && args+=(--pre "$pre")
  [ -n "$reset" ] && args+=(--reset "$reset")
  if [ -n "$env" ]; then
    for kv in $env; do args+=(--env "$kv"); done
  fi

  # 第 11 列声明本场要不要在伪终端里跑（06 号单）。不传下去的话，cmp.py 会按「声明了 tty
  # 但两侧都没采到」报缺文件——那一报是对的，错在采集这一环没接上。
  #
  # 行尾兜一道：表格若是 CRLF 落的，`read` 会把 \r 粘到最后一个字段上。
  tty="${tty%$'\r'}"
  [ "$tty" = "tty" ] && args+=(--tty)

  "$CODE/ab.sh" "$name" ${args[@]+"${args[@]}"} -- "$argv" >/dev/null 2>&1
  python3 "$CODE/cmp.py" "$name" || fail=1
done < "$CODE/scenes.tsv"

exit $fail
