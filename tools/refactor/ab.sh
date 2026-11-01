#!/usr/bin/env bash
# 参数矩阵 A/B：同一组 argv 分别交给新老两个二进制，比 stdout / stderr / 退出码。
#
# 【这份脚本是给本次重构用的临时追踪文件，重构完成（阶段 10 切换之后）随
#  .gitignore 里那段白名单一起删掉。】
#
# 用法：把本脚本拷进一个沙盒根目录（`build/<用途>_<时间戳>/`），沙盒里要有
#   - 老版二进制、新版二进制；
#   - `run/` 目录：放一份按本机形态写好的 `config/`（随包那份是 Linux 形态，在 Windows
#     上老版也会拦；见总图交接那一节）。
# 然后 `./ab.sh`。二进制文件名按平台不同，用环境变量改名：
#   Linux：AB_OLD=../sshfleet_old AB_NEW=../sshfleet2 ./ab.sh
#
# 归一：程序名、版本行（两版各自的名字与构建标识不同，不是差异）。
# 「已知差异」是阶段 03 认下的几处，见 16 号工单的 Comments——它们照旧列出来，只是不算失败。
# 每认下一处新差异，往 KNOWN_EXPECT 加一行「标签|为什么」，并在那一阶段的议题里记一句来历。
set -u
export MSYS2_ARG_CONV_EXCL='*'   # Git Bash（Windows）下别改写参数里的路径；Linux 上是空动作
export MSYS_NO_PATHCONV=1
cd "$(dirname "$0")/run" || exit 1
OLD=${AB_OLD:-../sshfleet_old.exe}
NEW=${AB_NEW:-../sshfleet2.exe}
pass=0; fail=0; known=0
failed=(); knowns=()

# 认下的差异：标签|为什么
KNOWN_EXPECT=(
  "密钥命令 --gen-key|三条密钥命令随阶段 08 接线（过渡形态）"
  "危险命令 提醒档 --yes|放行之后没有下一站（过渡形态，随 04 消失）"
  "-n 零|同上：老版走到读清单，新版停在「还没接上」"
  "--no-shell 配 -s|分站造成的顺序差：材料自检在参数合规之前"
  "多余位置参数|老版报「出现多余的参数」，本阶段未接这条判据"
)

is_known() {
  local label=$1 k
  for k in "${KNOWN_EXPECT[@]}"; do
    [ "${k%%|*}" = "$label" ] && return 0
  done
  return 1
}

run_one() {
  local bin=$1 out=$2 err=$3
  shift 3
  "$bin" "$@" >"$out" 2>"$err" </dev/null
  echo $?
}

# 归一两样：程序名与版本行。
norm() {
  cat "$1" "$2" | sed -e 's/sshfleet_old\.exe/PROG/g' -e 's/sshfleet2\.exe/PROG/g' \
                      -e 's/^版本: .*/版本: <归一>/'
}

SB=$(cd .. && pwd)
export SB

case_run() {
  local label=$1; shift
  local e1 e2 d
  e1=$(run_one "$OLD" "$SB/o.out" "$SB/o.err" "$@")
  e2=$(run_one "$NEW" "$SB/n.out" "$SB/n.err" "$@")
  d=$(diff <(norm "$SB/o.out" "$SB/o.err") <(norm "$SB/n.out" "$SB/n.err") 2>&1)
  if [ -z "$d" ] && [ "$e1" = "$e2" ]; then
    pass=$((pass+1))
    printf 'SAME  %-32s exit=%s\n' "$label" "$e1"
    return
  fi
  if is_known "$label"; then
    known=$((known+1)); knowns+=("$label")
    printf 'D-KWN %-32s exit=%s/%s\n' "$label" "$e1" "$e2"
    return
  fi
  fail=$((fail+1)); failed+=("$label")
  printf 'DIFF  %-32s exit=%s/%s\n' "$label" "$e1" "$e2"
  printf '%s\n' "$d" | sed 's/^/      /' | head -12
}

case_run "帮助 -h"                     -h
case_run "帮助 空参数"
case_run "帮助 --help"                 --help
case_run "模式零个"                     -f nodes.csv
case_run "模式两个"                     -c "echo hi" -d /etc/passwd
case_run "未知选项"                     --zzz
case_run "缺值 -c"                      -c
case_run "-c 为空"                      -c ""
case_run "-c 只有空格"                  -c " "
case_run "-n 非数字"                    -n abc -c "echo hi"
case_run "-n 零"                        -n 0 -c "echo hi"
case_run "-n 负数"                      -n -5 -c "echo hi"
case_run "-t 非数字"                    -t abc -c "echo hi"
case_run "-T 非数字"                    -T abc -c "echo hi"
case_run "-p 配 -c"                     -c "echo hi" -p /tmp
case_run "--no-shell 配 -s"             --no-shell -s ./x.sh
case_run "--no-shell 配 -u"             --no-shell -u . -p /tmp/
case_run "-u 缺 -p"                     -u .
case_run "-u 的 -p 不是绝对路径"         -u . -p rel
case_run "-u 的 -p 不以斜杠收尾"         -u . -p /tmp
case_run "-d 缺 -p"                     -d /etc/passwd
case_run "-d 远端路径相对"               -d rel -p .
case_run "-d 本机目录不存在"             -d /etc/passwd -p ./nope
case_run "--sudo 与 --no-sudo 同给"      --sudo --no-sudo -c "echo hi"
case_run "改密配 -p"                     --change-password pw -p /tmp
case_run "改密配 -a"                     --change-password pw -a "x"
case_run "改密配 -k"                     --change-password pw -k
case_run "改密为空"                      --change-password ""
case_run "-s 文件不存在"                 -s ./nope.sh
case_run "密钥命令 --gen-key"            --gen-key
case_run "危险命令 禁止档"               -c "rm -rf /"
case_run "危险命令 提醒档 --yes"          -c "rm -rf /opt" --yes
case_run "危险命令 提醒档 读到输入结束"    -c "rm -rf /opt"
case_run "危险命令 路径归一 //"           -c "rm -rf //"
case_run "危险命令 大小写"                -c "RM -RF /etc"
case_run "等号内联 + -h"                 --command=ls -h
case_run "等号内联 + 未知选项"            --command=ls --bogus
case_run "多余位置参数"                   -c "echo hi" extra
case_run "-h 后跟未知选项"                -h --bogus
case_run "未知选项先于 -h"                --bogus -h

echo
echo "SAME=$pass  已知差异=$known  真差异=$fail"
[ ${#knowns[@]} -gt 0 ] && printf '已知差异：%s\n' "${knowns[*]}"
[ ${#failed[@]} -gt 0 ]  && printf '真差异：%s\n' "${failed[*]}"
exit $((fail > 0))
