#!/usr/bin/env bash
# SSHFleet 构建脚本（Ubuntu / Linux，跑一次出齐两个平台的发布包）
#
# ⚠️ 本脚本连同打包、发布流程（含 release/ 目录）都是**开发者本人手动使用**的，
#    与 AI coding、测试无关：改代码、跑测试、验证功能**都不涉及**这里，
#    除非开发者在本轮明确要求打包或发布，否则不要跑本脚本、不要动 release/、
#    也不要因为「产物没更新」而去重跑它。
#
#   ./build-linux.sh
#   不带参数直接走完全部流程：编译 windows/amd64 + linux/amd64（产物落工作区根 build/）
#   → 用同一个时间戳出两套包：release/SSHFleet_<时间戳>_windows.zip 与
#     release/SSHFleet_<时间戳>_linux.tar.gz；打包前先清空整个 release/，
#     目录里永远只有最新一次的产物。
#
# 出厂配置按平台取：config/SSHFleet-for-linux.conf / SSHFleet-for-windows.conf
#   各自复制成包内的 config/SSHFleet.conf。工作区里那份 config/SSHFleet.conf 是
#   作者本机在用的配置，不随包发布。
#
# 两处与 build-windows.bat 不同，都是 Linux 侧的地面事实逼出来的：
#   - zip 交给 python3 的 zipfile：Info-ZIP 的 zip 不置 UTF-8 标志位（general purpose
#     bit 11），中文手册名在 Windows 解压时会按本地代码页解成乱码。zipfile 会置位，
#     与 bat 侧 .NET ZipFile 的行为一致（2026-10-12 用 zipfile 核对 flag_bits 实测确认：
#     zip -UN=UTF8 读到 0x0000，zipfile 读到 0x0800）。
#   - tar.gz 直接用 GNU tar 的 --mode=755，不必像 bat 那样绕道 Git 的 bash。
#
# 构建的临时目录指到 build/tmp（工作区内）。Go 的编译缓存仍走默认位置——它可复用、
# 不是产物，不受「构建产物不出工作区」约束。

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MOD="$ROOT/modules/SSHFleet_Go"
CFG="$MOD/config"
DOCS_SRC="$ROOT/docs"
OUT="$ROOT/build"
REL="$ROOT/release"

die()  { printf '[失败] %s\n' "$*" >&2; exit 1; }
step() { printf '%s\n' "$*"; }

# ---- 前置检查：缺什么当场点名，别拖到后面只报一句「构建失败」----------------

pick_go() {
  local c
  for c in "$(command -v go 2>/dev/null || true)" \
           /snap/go/current/bin/go /usr/local/go/bin/go /usr/lib/go/bin/go; do
    if [ -n "$c" ] && [ -x "$c" ] && "$c" version >/dev/null 2>&1; then
      printf '%s' "$c"; return 0
    fi
  done
  return 1
}

GO="$(pick_go)" || die "未找到可用的 go，请先安装 Go 并加入 PATH"
command -v python3 >/dev/null 2>&1 || die "未找到 python3（打 zip 要靠它写 UTF-8 文件名）"
[ -d "$MOD" ] || die "找不到子模块目录：$MOD"
[ -d "$DOCS_SRC" ] || die "找不到手册目录：$DOCS_SRC"
[ -f "$CFG/SSHFleet-for-linux.conf" ]   || die "找不到出厂配置模板：$CFG/SSHFleet-for-linux.conf"
[ -f "$CFG/SSHFleet-for-windows.conf" ] || die "找不到出厂配置模板：$CFG/SSHFleet-for-windows.conf"
ls "$DOCS_SRC"/[0-9]-*.md >/dev/null 2>&1 || die "手册目录里没有「数字-名称.md」形态的文件：$DOCS_SRC"

# ---- 临时目录落在工作区内 ----------------------------------------------------

mkdir -p "$OUT/tmp"
export TMPDIR="$OUT/tmp" GOTMPDIR="$OUT/tmp"

# ---- 编译 --------------------------------------------------------------------

step "[1/2] 编译 windows/amd64 ..."
(cd "$MOD" && env GOOS=windows GOARCH=amd64 "$GO" build -o "$OUT/SSHFleet.exe" .) \
  || die "windows/amd64 构建失败"
echo "[完成] build/SSHFleet.exe"

step "[2/2] 交叉编译 linux/amd64 ..."
(cd "$MOD" && env GOOS=linux GOARCH=amd64 "$GO" build -o "$OUT/SSHFleet" .) \
  || die "linux/amd64 构建失败"
echo "[完成] build/SSHFleet"

# ---- 组发布目录 --------------------------------------------------------------

TS="$(date +%Y-%m-%d_%H-%M-%S)"
WNAME="SSHFleet_${TS}_windows"
LNAME="SSHFleet_${TS}_linux"
WPKG="$REL/$WNAME"
LPKG="$REL/$LNAME"

case "$REL" in "$ROOT"/*) ;; *) die "release 目录不在工作区内，拒绝清空：$REL" ;; esac
rm -rf "$REL"
mkdir -p "$WPKG/config" "$WPKG/docs" "$LPKG/config" "$LPKG/docs"

# 两个平台共同的随包内容：README / CHANGELOG / LICENSE / 三份关键词规则 / 六册手册
for d in "$WPKG" "$LPKG"; do
  cp -f "$ROOT/README.md" "$ROOT/CHANGELOG.md" "$ROOT/LICENSE" "$d/"
  cp -f "$CFG/keywords_dangerous.conf" "$CFG/keywords_error.conf" \
        "$CFG/keywords_passwd.conf" "$d/config/"
  cp -f "$DOCS_SRC"/[0-9]-*.md "$d/docs/"
done

# 可执行文件与出厂配置按平台各取各的
cp -f "$OUT/SSHFleet.exe" "$WPKG/"
cp -f "$CFG/SSHFleet-for-windows.conf" "$WPKG/config/SSHFleet.conf"
cp -f "$OUT/SSHFleet" "$LPKG/"
cp -f "$CFG/SSHFleet-for-linux.conf" "$LPKG/config/SSHFleet.conf"

# ---- zip（Windows 包）-------------------------------------------------------

step "[发布] 打包 release/$WNAME.zip ..."
python3 - "$WPKG" "$REL/$WNAME.zip" <<'PY'
import os, sys, zipfile

src, dst = sys.argv[1], sys.argv[2]
root = os.path.dirname(os.path.abspath(src))
base = os.path.basename(os.path.abspath(src))

count = 0
with zipfile.ZipFile(dst, "w", zipfile.ZIP_DEFLATED) as z:
    for dirpath, dirnames, filenames in os.walk(src):
        dirnames.sort()
        filenames.sort()
        arc = os.path.relpath(dirpath, root).replace(os.sep, "/")
        z.write(dirpath, arc + "/")
        for fn in filenames:
            z.write(os.path.join(dirpath, fn), arc + "/" + fn)
            count += 1
print(f"  归档 {count} 个文件（顶层目录 {base}/）")
PY
[ "$(stat -c %s "$REL/$WNAME.zip")" -gt 1000 ] || die "生成 zip 失败"
echo "[发布] 完成：release/$WNAME.zip"

# ---- tar.gz（Linux 包）------------------------------------------------------

step "[发布] 打包 release/$LNAME.tar.gz ..."
if tar --version 2>/dev/null | grep -q GNU; then
  # 显式固定 755：归档内要保证可执行权限，不依赖源文件当前的权限状态。
  # 文本文件跟着 755 无害（与 bat 侧口径一致）。
  tar --mode=755 -czf "$REL/$LNAME.tar.gz" -C "$REL" "$LNAME"
else
  tar -czf "$REL/$LNAME.tar.gz" -C "$REL" "$LNAME"
  echo "[提示] 非 GNU tar，未固定归档内的权限位"
fi
[ "$(stat -c %s "$REL/$LNAME.tar.gz")" -gt 1000 ] || die "生成 tar.gz 失败"
echo "[发布] 完成：release/$LNAME.tar.gz"

# ---- 收尾 --------------------------------------------------------------------

echo
echo "全部完成，发布产物（release/）："
ls -lh --time-style='+%Y-%m-%d %H:%M' "$REL" | tail -n +2

# 双击运行或从文件管理器启动时，终端窗口会随脚本结束立刻关闭，看不到上面的清单。
# 非交互式调用（CI、管道）不阻塞，避免卡住自动化流程。
if [ -t 0 ]; then
  echo
  printf '按任意键退出...'
  read -rsn1 || true
  echo
fi
