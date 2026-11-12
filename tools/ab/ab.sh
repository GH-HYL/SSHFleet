#!/bin/bash
# 09 号单的 A/B：同一份输入，新老各跑一遍，收屏幕、退出码、本月归档目录与各侧 HOME。
#
# 用法：ab.sh <场景名> [--stdin 一行输入] [--kill-after 秒] [--reset '命令'] [--conf 'key=value'] [--env K=V]… [--tty] -- <参数…>
#
# --kill-after 到点给这一条作业的**整个进程组**发 SIGINT（模拟终端上的 Ctrl+C：sshfleet 与它
# 拉起的 ssh 一起收到），然后等它自己收场并入退出码。
#
# --env 只注入本侧这一次跑（`SSHFLEET_KEY` 造「两处不一致」就靠它），另一侧拿不到。
#
# --stdin 里可用 `\n`（`printf %b`）分多行：回退非 bash 那一场要答两次（先「继续用 bash」，
# 再确认执行），只给一行会停在确认屏、两侧都「操作已取消」，比出来是「一致」等于没跑。
#
# --reset 在**每一侧**开跑之前各执行一次（远端现场两边必须一样，否则先跑的那侧会把
# 后跑那侧的现场改掉——上传撞名、改密、下载目录都是这个坑）。
# --conf 在复位之后改本侧配置的一格（每场两侧都会先复位，所以每场都得给）。
#
# --tty（06 号单）：这一场在伪终端里跑，屏幕落 `<侧>.tty` 而不是 `<侧>.screen`——进度面板、
# 折行帮助这些只在 TTY 下出现的排版，重定向采集永远看不到（09 的 E9）。伪终端靠 `script`；
# 本机没有就直接失败，**不许静默退化成非 TTY**（退化了就是又量不出来，还会报「一致」）。
set -u
CODE="$(cd "$(dirname "$0")" && pwd)"
SB="${SSHFLEET_AB_SB:-$CODE}"
name="$1"; shift
stdin_data=""; reset=""; confs=(); envs=(); pre=""; argv=""; kill_after=""; want_tty=0
while [ $# -gt 0 ]; do
  case "${1:-}" in
    --stdin) stdin_data="$2"; shift 2 ;;
    --kill-after) kill_after="$2"; shift 2 ;;
    --reset) reset="$2"; shift 2 ;;
    --conf)  confs+=("$2"); shift 2 ;;
    --env)   envs+=("$2"); shift 2 ;;
    --pre)   pre="$2"; shift 2 ;;
    --tty)   want_tty=1; shift ;;
    --) shift; argv="$*"; break ;;
    *) break ;;
  esac
done

# 本侧要注入的环境变量（`--env K=V`，可给多次）。只作用于这一次跑，不影响另一侧。
envstr=""
for e in ${envs[@]+"${envs[@]}"}; do envstr="$envstr $e"; done

out="$SB/out/$name"
rm -rf "$out"; mkdir -p "$out"

for side in old new; do
  "$CODE/reset.sh" "$side"
  for c in "${confs[@]:-}"; do
    [ -n "$c" ] && python3 "$CODE/prep.py" "$SB/$side/config/SSHFleet.conf" "$c"
  done
  [ -n "$reset" ] && bash -c "$reset" >/dev/null 2>&1
  # HOME 必须 export 而不是写成命令前缀：`--pre` 里常有 `A && B`，前缀只作用于第一条命令，
  # B 会拿真主目录去跑（`seedkey.sh` 曾因此改到开发机自己的 `.bashrc`）。
  if [ -n "$pre" ]; then
    ( cd "$SB/$side" && export HOME="$SB/home_$side"; eval "$pre" ) >"$out/$side.pre" 2>&1
  fi

  # TTY 场次落到 `.tty`，与非 TTY 的 `.screen` 分开存：两种面各比各的，不互相盖掉。
  tty_screen=""
  tty_cmd=""
  if [ "$want_tty" = "1" ]; then
    command -v script >/dev/null 2>&1 || { echo "没有 script：TTY 场次跑不了（$name）" >&2; exit 6; }
    tty_screen="$out/$side.tty"
    # **不许再套一层 eval**：`script -c` 交给它自己的 shell 解析，argv 原样拼进去，
    # 带引号的值（`-c "echo hello"`）在这一层仍是语法。早先写成 `eval \"… $argv\"`，
    # 外层那对双引号会先跟值里的引号配对，`-c "echo hello"` 被拆成两个词——
    # 两侧同时按「参数写错」被拒，比出来是「一致」，等于这一场没跑（2026-11-11 踩）。
    tty_cmd="cd \"$SB/$side\" && export HOME=\"$SB/home_$side\"; $envstr ./sshfleet $argv"
  fi

  if [ -n "$kill_after" ]; then
    # `set -m` 让这条作业自成一个进程组，`kill -INT -$!` 才是终端那一下 Ctrl+C 的口径
    # （只打 sshfleet 会漏掉它拉起的 ssh，收场路径就不是使用者真按出来的那条）。
    set -m
    if [ -n "$tty_screen" ]; then
      script -q -e -c "$tty_cmd" /dev/null < /dev/null >"$tty_screen" 2>&1 &
    else
      ( cd "$SB/$side" && export HOME="$SB/home_$side"; eval "$envstr ./sshfleet $argv" < /dev/null ) \
        >"$out/$side.screen" 2>&1 &
    fi
    job="$!"
    sleep "$kill_after"
    kill -INT -"$job" 2>/dev/null
    wait "$job" 2>/dev/null
    echo $? >"$out/$side.code"
    set +m
  elif [ -n "$stdin_data" ]; then
    if [ -n "$tty_screen" ]; then
      printf '%b\n' "$stdin_data" | script -q -e -c "$tty_cmd" /dev/null >"$tty_screen" 2>&1
    else
      ( cd "$SB/$side" && export HOME="$SB/home_$side"; printf '%b\n' "$stdin_data" | eval "$envstr ./sshfleet $argv" ) \
        >"$out/$side.screen" 2>&1
    fi
    echo $? >"$out/$side.code"
  else
    if [ -n "$tty_screen" ]; then
      script -q -e -c "$tty_cmd" /dev/null < /dev/null >"$tty_screen" 2>&1
    else
      ( cd "$SB/$side" && export HOME="$SB/home_$side"; eval "$envstr ./sshfleet $argv" < /dev/null ) \
        >"$out/$side.screen" 2>&1
    fi
    echo $? >"$out/$side.code"
  fi

  newest="$(ls -d "$SB/$side/history"/*/ 2>/dev/null | tail -1)"
  if [ -n "$newest" ]; then cp -r "$newest" "$out/$side.history"; fi
  readlink "$SB/$side/latest_history" >"$out/$side.link" 2>/dev/null || : >"$out/$side.link"
  # 工具日志（维护者可见面）不在归档目录里，长在 <side>/history/ 下，得单独抄一份（05 号单）。
  # 抄不到就留空文件：缺与空都由 cmp.py 按同一口径判，不靠 shell 的静默成功。
  if [ -f "$SB/$side/history/SSHFleetTools.log" ]; then
    cp "$SB/$side/history/SSHFleetTools.log" "$out/$side.tools.log"
  else
    : >"$out/$side.tools.log"
  fi
done

echo "$name old=$(cat "$out/old.code") new=$(cat "$out/new.code")"
