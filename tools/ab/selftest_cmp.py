#!/usr/bin/env python3
"""比对口径自检：造样本 → 跑 cmp.py → 断言退出码。

覆盖 01／02／03 三单的档；`--real` 额外拿 09 沙盒的真实产物跑「耗时列收紧后仍一致」回归。
红档必须同时核「样本文件真的在」——造样本失败被吞掉会变成空对空的假绿。

用法：python selftest_cmp.py [--real]
"""
import os
import shutil
import subprocess
import sys

CODE = os.path.dirname(os.path.abspath(__file__))  # 脚本在 tools/ab
SB = os.environ.get("SSHFLEET_AB_SB") or CODE      # 现场：out/ 与样本落这里
PY = sys.executable
MK = os.path.join(CODE, "mkbook.py")
OLD_SB = os.environ.get("SSHFLEET_AB_OLD_SB") or os.path.normpath(
    os.path.join(SB, os.pardir, "建站09_AB_2026-11-06_1535"))
HDR = ["G1=s:连接耗时", "H1=s:执行耗时"]
RESULTS = []
# 子进程与父进程都按 UTF-8：Windows 默认按 GBK 解码子进程输出，中文会当场炸。
ENV = dict(os.environ, PYTHONIOENCODING="utf-8", PYTHONDONTWRITEBYTECODE="1")


def build(scen, side, sheet, cells):
    hist = os.path.join(SB, "out", scen, side + ".history")
    os.makedirs(hist, exist_ok=True)
    dest = os.path.join(hist, "results.xlsx")
    subprocess.run([PY, MK, dest, sheet] + cells, cwd=SB, check=True,
                   env=ENV, capture_output=True, encoding="utf-8", errors="replace")
    if not os.path.exists(dest):
        raise SystemExit("样本没造出来：%s" % dest)
    with open(os.path.join(SB, "out", scen, side + ".code"), "w") as f:
        f.write("0\n")
    for name in (side + ".link", side + ".screen"):
        open(os.path.join(SB, "out", scen, name), "w").close()
    # 05 号单起工具日志入比对面：样本也得有两份，否则会被报「缺文件」——那是采集没做，不是差异。
    open(os.path.join(SB, "out", scen, side + ".tools.log"), "wb").close()


def run(scen):
    p = subprocess.run([PY, os.path.join(CODE, "cmp.py"), scen], cwd=SB, capture_output=True,
                       env=ENV, encoding="utf-8", errors="replace")
    return p.returncode, ((p.stdout or "") + (p.stderr or "")).strip().replace("\r", "")


def check(label, scen, old, new, expect):
    shutil.rmtree(os.path.join(SB, "out", scen), ignore_errors=True)
    build(scen, "old", old[0], old[1])
    build(scen, "new", new[0], new[1])
    code, msg = run(scen)
    RESULTS.append((code == expect, label, expect, code, msg))


def check_bytes(label, old_b, new_b, expect):
    scen = "zz-bytes"
    shutil.rmtree(os.path.join(SB, "out", scen), ignore_errors=True)
    os.makedirs(os.path.join(SB, "out", scen), exist_ok=True)
    for side, raw in (("old", old_b), ("new", new_b)):
        with open(os.path.join(SB, "out", scen, side + ".code"), "wb") as f:
            f.write(b"0\n")
        open(os.path.join(SB, "out", scen, side + ".link"), "wb").close()
        with open(os.path.join(SB, "out", scen, side + ".screen"), "wb") as f:
            f.write(raw)
        open(os.path.join(SB, "out", scen, side + ".tools.log"), "wb").close()
    code, msg = run(scen)
    RESULTS.append((code == expect, label, expect, code, msg))


def check_arch(label, scen, expect, layout, expect_code):
    """归档面：layout 的每个侧 → None（无归档目录）／[]（空目录）／[文件名…]（造这几份，内容同）。"""
    shutil.rmtree(os.path.join(SB, "out", scen), ignore_errors=True)
    base = os.path.join(SB, "out", scen)
    os.makedirs(base, exist_ok=True)
    with open(os.path.join(base, "expect"), "w", encoding="utf-8") as f:
        f.write(expect)
    for side in ("old", "new"):
        with open(os.path.join(base, side + ".code"), "w") as f:
            f.write("0\n")
        open(os.path.join(base, side + ".link"), "w").close()
        open(os.path.join(base, side + ".screen"), "w").close()
        open(os.path.join(base, side + ".tools.log"), "wb").close()
        files = layout.get(side)
        if files is None:
            continue
        hist = os.path.join(base, side + ".history")
        os.makedirs(hist, exist_ok=True)
        for name in files:
            with open(os.path.join(hist, name), "w", encoding="utf-8") as f:
                f.write("同一份\n")
    code, msg = run(scen)
    RESULTS.append((code == expect_code, label, expect_code, code, msg))


def check_tools(label, old_text, new_text, expect_code):
    """工具日志面：两侧各写一份 `<side>.tools.log`，其余产物留空。"""
    scen = "zz-tools"
    shutil.rmtree(os.path.join(SB, "out", scen), ignore_errors=True)
    base = os.path.join(SB, "out", scen)
    os.makedirs(base, exist_ok=True)
    with open(os.path.join(base, "expect"), "w", encoding="utf-8") as f:
        f.write("-")
    for side, text in (("old", old_text), ("new", new_text)):
        with open(os.path.join(base, side + ".code"), "w") as f:
            f.write("0\n")
        open(os.path.join(base, side + ".link"), "w").close()
        open(os.path.join(base, side + ".screen"), "w").close()
        if text is None:
            continue
        with open(os.path.join(base, side + ".tools.log"), "wb") as f:
            f.write(text.encode("utf-8"))
    code, msg = run(scen)
    RESULTS.append((code == expect_code, label, expect_code, code, msg))


def check_tty(label, old_text, new_text, expect_code):
    """TTY 屏幕面：两侧各写一份 `<side>.tty`，并在 `out/<场景>/tty` 里声明本场要 tty。"""
    scen = "zz-tty"
    shutil.rmtree(os.path.join(SB, "out", scen), ignore_errors=True)
    base = os.path.join(SB, "out", scen)
    os.makedirs(base, exist_ok=True)
    with open(os.path.join(base, "expect"), "w", encoding="utf-8") as f:
        f.write("-")
    with open(os.path.join(base, "tty"), "w", encoding="utf-8") as f:
        f.write("tty")
    for side, text in (("old", old_text), ("new", new_text)):
        with open(os.path.join(base, side + ".code"), "w") as f:
            f.write("0\n")
        open(os.path.join(base, side + ".link"), "w").close()
        open(os.path.join(base, side + ".tools.log"), "wb").close()
        if text is None:
            continue
        with open(os.path.join(base, side + ".tty"), "wb") as f:
            f.write(text.encode("utf-8"))
    code, msg = run(scen)
    RESULTS.append((code == expect_code, label, expect_code, code, msg))


def real_regression():
    sys.path.insert(0, CODE)
    import cmp as cmpmod

    for scen in ("b1-command", "b10-passwd-expired", "b13-two-nodes"):
        for art in ("results.xlsx", "terminal-output.xlsx"):
            a = os.path.join(OLD_SB, "out", scen, "old.history", art)
            b = os.path.join(OLD_SB, "out", scen, "new.history", art)
            if not (os.path.exists(a) and os.path.exists(b)):
                continue
            same = cmpmod.compare_xlsx(a, b)
            RESULTS.append((same, "真实回归：%s/%s" % (scen, art), True, same, "compare_xlsx"))


def main():
    check_bytes("01 红：非 UTF-8 字节不同", b"caf\xe9", b"caf\xff", 1)
    check_bytes("01 绿：同一串字节", b"caf\xe9", b"caf\xe9", 0)

    check("02 红：数值 2201 ↔ 文本 2201", "zz-type",
          ("Results", ["A1=n:2201"]), ("Results", ["A1=s:2201"]), 1)
    check("02 绿：两侧同为数值 2201", "zz-type-ok",
          ("Results", ["A1=n:2201"]), ("Results", ["A1=n:2201"]), 0)
    check("02 红：工作表名改一字", "zz-sheet",
          ("Results", ["A1=n:2201"]), ("Results2", ["A1=n:2201"]), 1)
    check("02 绿：工作表同名", "zz-sheet-ok",
          ("Results", ["A1=n:2201"]), ("Results", ["A1=n:2201"]), 0)

    check("03 红：耗时格变文本", "zz-dur-type",
          ("Results", HDR + ["G2=n:0.2"]), ("Results", HDR + ["G2=s:0.2"]), 1)
    check("03 红：耗时格量级变了", "zz-dur-mag",
          ("Results", HDR + ["G2=n:0.2"]), ("Results", HDR + ["G2=n:12.5"]), 1)
    check("03 红：耗时格不是数字", "zz-dur-text",
          ("Results", HDR + ["G2=n:0.2"]), ("Results", HDR + ["G2=s:很快"]), 1)
    check("03 红：耗时格整格缺失", "zz-dur-miss",
          ("Results", HDR + ["G2=n:0.2"]), ("Results", HDR), 1)
    check("03 绿：亚秒抖动", "zz-dur-ok",
          ("Results", HDR + ["G2=n:0.209061886", "H2=n:0.008345099"]),
          ("Results", HDR + ["G2=n:0.117048092", "H2=n:0.0079632"]), 0)
    check("03 绿：整数秒同桶", "zz-dur-bucket",
          ("Results", HDR + ["G2=n:3.2"]), ("Results", HDR + ["G2=n:3.9"]), 0)

    check_arch("04 红：声明该有归档但两侧都没有", "zz-arch-miss", "hist", {}, 1)
    check_arch("04 红：只有一侧有归档", "zz-arch-half", "hist", {"old": []}, 1)
    check_arch("04 红：两侧归档目录都是空的", "zz-arch-empty", "hist", {"old": [], "new": []}, 1)
    check_arch("04 绿：声明该有且两侧都有同一份", "zz-arch-ok", "hist",
               {"old": ["report.txt"], "new": ["report.txt"]}, 0)
    check_arch("04 绿：声明该无（c1 口径）", "zz-arch-none", "-", {}, 0)
    check_arch("04 红：声明该无却两侧都有归档", "zz-arch-forbid", "-",
               {"old": ["report.txt"], "new": ["report.txt"]}, 1)

    LOG = "2026-11-09 09:15:52.126  -  [INFO   ]  -  开始一次运行\n"
    check_tools("05 绿：两侧工具日志逐字节同", LOG, LOG, 0)
    check_tools("05 红：一侧多一行正文", LOG, LOG + "多出来的一行\n", 1)
    # 登记为「新版不记」的那类行：老版有、新版没有，两侧去掉后应一致。
    check_tools("05 绿：老版多「原始选项/参数」行（登记为不记）",
                LOG + "2026-11-09 09:15:52.130  -  [INFO   ]  -  原始选项/参数：./sshfleet -h\n",
                LOG, 0)
    check_tools("05 红：只有一侧有工具日志", LOG, None, 1)

    TTY = "\u8fdb\u5ea6\u9762\u677f 100% |\r\n"
    check_tty("06 绿：两侧 TTY 屏幕逐字节同", TTY, TTY, 0)
    check_tty("06 红：TTY 屏幕多一帧面板", TTY, TTY + "\u8fdb\u5ea6\u9762\u677f 50% |\r\n", 1)
    check_tty("06 红：声明 tty 但两侧都没采到", None, None, 1)

    if "--real" in sys.argv:
        real_regression()

    bad = [r for r in RESULTS if not r[0]]
    for ok, label, expect, code, msg in RESULTS:
        print("%s %-30s 期望 exit=%s 实际 exit=%s  %s" % ("OK  " if ok else "FAIL", label, expect, code, msg))
    print("--- %d 档，失败 %d ---" % (len(RESULTS), len(bad)))
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main())
