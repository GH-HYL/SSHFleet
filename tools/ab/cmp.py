#!/usr/bin/env python3
"""比对口径（议题 2026-11-10-1615）：把一场两侧的屏幕、退出码、归档产出归一化后**按字节**比。

归一化做过的每一件事都列在这里；多一样，必须在议题 `audit.md` 的登记表里有对应一行：
  1. 时间戳的两种写法（`2026-11-09 09:15:52`、`2026-11-09_09-15-52`）→ `<TS>`
  2. 耗时的两种写法（`3s`、`3 秒`）→ `<DUR>`
  3. 沙盒绝对路径：两侧目录名 → `<SIDE>`、两侧各自 HOME → `<HOME>`、沙盒根 → `<SB>`
  4. 归档目录名里的秒级时间戳：由第 1 条覆盖，没有独立规则
  5. 耗时列（表头写「连接耗时」「执行耗时」那几列）的数字折成整秒桶 `<DUR:<n>s>`：
     类型必须是数值、值必须是数字；亚秒抖动因此不报，类型变、量级变、非数字、缺格都报。

除此之外一个字节都不动：文本件与屏幕在 bytes 上比，不做 UTF-8 有损解码（09 那把尺子的 E4）。

不做归一化、但参与比较的：一张表的工作表名与顺序；一格的类型（数值／文本）。
格的身份是「列行 + 类型 + 值」；样式索引、列宽、行高、zip 条目时间仍不比，理由进结论登记表。

尚未动的面（各有工单）：
  - `SSHFleetTools.log` 不比 → 05 号单
  - 屏幕面没有 TTY → 06 号单

04 号单加的一条：每一场该不该有归档，由场景表第 10 列声明（`hist`＝该有，`-`＝该没有）。
声明该有而没有、或两侧归档都是空的，报「缺归档」；不再拿「两侧都没有」当一致（09 那把尺子的 E7／E8）。
声明该无的（例如 `c1` 被拒不许建归档目录）仍然允许空对空。

用法：cmp.py <场景名>        结果打一行；有差异退出码 1。
"""
import hashlib
import html
import os
import re
import sys
import zipfile

# 脚本住在 tools/ab，现场（两侧目录、out/、scenes.tsv 之外的产物）由 SSHFLEET_AB_SB 指定；
# 不给就当场跑。scenes.tsv 与脚本同住，跟着 CODE 走。
CODE = os.path.dirname(os.path.abspath(__file__))
SB = os.environ.get("SSHFLEET_AB_SB") or CODE
TEXT_ARTIFACTS = ("report.txt", "terminal-output.txt", "SSHFleetExec.log")
XLSX = ("results.xlsx", "terminal-output.xlsx")
TOOL_LOG = ("old.tools.log", "new.tools.log")
# 登记为「新版不记」的那一类行：老版写了、新版压根不写。两类都属**维护者可见面**，
# 逐条有出处，不是为了让结果变绿：
#
# 1. `原始选项/参数`（09 的 E12）：老版每条命令都写一遍原始命令行，新版不写。
# 2. 各站自报成功那一族（2026-11-11 定的「登记不回改」，见
#    `docs/issues/2026-11-11-0915-拒绝路径留痕/spec.md` 与 07 号单的 `audit.md`）：
#    新版把日志句柄收到塔尖一处，各站不再写日志；要复刻就得拆 ADR-0032 的分工，
#    或让塔尖凭「没报错」替站宣布成功——后者是让日志写给没有事实的地方。
#    塔尖**自己做的动作**四条（生成归档目录／轮转／切回／已退出）已补，不在此列。
OLD_ONLY_MARKS = tuple(m.encode("utf-8") for m in (
    "原始选项/参数",
    "选项/参数解析成功",
    "输入的选项/参数合规性检查通过",
    "危险关键词内容检查通过",
    "读取节点信息成功",
    "计算统计结果信息成功",
    "格式化统计结果信息输出到报告文件成功",
    "保存执行资源文件成功",
    "生成 terminal-output.xlsx 成功",
    "生成 results.xlsx 成功",
))

# 登记为「新版多记」的那一类（4.5 的 04 号单，**有意新增**）：`--yes` 那一行留痕。
# 只出现在 `SSHFleetExec.log`，两侧一起去掉；去掉几行同样打在结果行尾。
NEW_ONLY_MARKS = tuple(m.encode("utf-8") for m in (
    "非交互模式（--yes）跳过执行选项/参数确认",
))

TS = re.compile(rb"\d{4}-\d{2}-\d{2}[_ ]\d{2}[:-]\d{2}[:-]\d{2}(?:[.]\d+)?")
DUR_S = re.compile(rb"\d+(?:\.\d+)?s")
DUR_CN = re.compile(rb"\d+(?:\.\d+)? ?" + "秒".encode("utf-8"))
# 进度面板里那格已耗时（`0:00:02`，形如 H:MM:SS）。它是「这一次跑了几秒」，不是任何一方的行为：
# 两侧同一个二进制也会差，且采样三次三次都差（2026-11-11 实测，b1／b10 各三次）。
# 只认**三段冒号**这种形态；命令输出里偶尔出现的时间码两侧本来就是同一个值，抹掉不影响判据。
CLOCK = re.compile(rb"(?<![\d:])\d{1,2}:\d{2}:\d{2}(?![\d:])")


def norm(raw):
    """bytes 进、bytes 出：只做上面登记过的替换。"""
    raw = TS.sub(b"<TS>", raw)
    raw = DUR_CN.sub(b"<DUR>", raw)
    raw = DUR_S.sub(b"<DUR>", raw)
    raw = CLOCK.sub(b"<CLOCK>", raw)
    for side in ("old", "new"):
        raw = raw.replace(os.path.join(SB, side).encode("utf-8"), b"<SIDE>")
        raw = raw.replace(os.path.join(SB, "home_" + side).encode("utf-8"), b"<HOME>")
    raw = raw.replace(SB.encode("utf-8"), b"<SB>")
    return raw


def norm_cell(value):
    """表格格值来自 UTF-8 的 XML，走同一条规则；surrogateescape 保证往返不丢字节。"""
    return norm(value.encode("utf-8", "surrogateescape")).decode("utf-8", "surrogateescape")


def read(path):
    """读原始字节；文件不在返回 None。不在这里解码——按字节比。"""
    try:
        with open(path, "rb") as f:
            raw = f.read()
    except FileNotFoundError:
        return None
    return norm(raw)


def sha(path):
    with open(path, "rb") as f:
        return hashlib.sha256(f.read()).hexdigest()[:16]


def listing(root):
    out = {}
    if not os.path.isdir(root):
        return out
    for base, _, files in os.walk(root):
        for name in files:
            p = os.path.join(base, name)
            out[os.path.relpath(p, root)] = sha(p)
    return out


def load_strings(z):
    """把 sharedStrings 解成下标 → 文本。两侧各自的去重结果不同（耗时有相同的就并成一条），
    所以不能比这份 XML 本身，只能比各格最终落到哪个值。"""
    if "xl/sharedStrings.xml" not in z.namelist():
        return []
    xml = z.read("xl/sharedStrings.xml").decode("utf-8")
    return [html.unescape(re.sub(r"<[^>]+>", "", si)) for si in re.findall(r"<si>(.*?)</si>", xml, re.S)]


CELL = re.compile(r'<c r="([A-Z]+)(\d+)"([^>]*?)(?:/>|>(.*?)</c>)', re.S)


def sheet_cells(xml, strings):
    """一格 → (类型, 归一后的值)。字符串格按下标查回文本，数字格留原文；类型参与比较。"""
    cells = {}
    for col, row, attrs, inner in CELL.findall(xml):
        value = ""
        if inner:
            m = re.search(r"<v>(.*?)</v>", inner, re.S)
            if m:
                value = m.group(1)
            else:  # 内联字符串 <is><t>…</t></is>
                value = "".join(re.findall(r"<t[^>]*>(.*?)</t>", inner, re.S))
        is_text = 't="s"' in attrs or 't="str"' in attrs or "<is>" in (inner or "")
        if 't="s"' in attrs and value.isdigit():
            value = strings[int(value)] if int(value) < len(strings) else "<丢失>"
        cells[col + row] = ("文本" if is_text else "数值", norm_cell(html.unescape(value)))
    return cells


def workbook_sheets(z):
    """工作表名与顺序。交接清单把「照老版保留的工作表名」写成等价要求，判据里就得有它。"""
    if "xl/workbook.xml" not in z.namelist():
        return []
    xml = z.read("xl/workbook.xml").decode("utf-8")
    return re.findall(r'<sheet\b[^>]*?name="([^"]*)"', xml)


DUR_HEADERS = ("连接耗时", "执行耗时")
NUM = re.compile(r"^\d+(?:\.\d+)?$")
CELL_REF = re.compile(r"^([A-Z]+)(\d+)$")


def duration_columns(cells):
    """表头那一行里哪几列是耗时列。按表头文字认，不按列名认——列位置挪了也跟得上。"""
    cols = set()
    for ref, (_kind, value) in cells.items():
        m = CELL_REF.match(ref)
        if m and m.group(2) == "1" and value in DUR_HEADERS:
            cols.add(m.group(1))
    return cols


def duration_norm(cells):
    """耗时列按语义收：必须数值、必须数字，再折成整秒桶；其余三种情形原样留着让它们比出差异。"""
    cols = duration_columns(cells)
    out = {}
    for ref, (kind, value) in cells.items():
        m = CELL_REF.match(ref)
        if m and m.group(1) in cols and m.group(2) != "1":
            if kind != "数值" or not NUM.match(value):
                out[ref] = ("耗时列非数值", value)
            else:
                out[ref] = (kind, "<DUR:%ds>" % int(float(value)))
        else:
            out[ref] = (kind, value)
    return out


def compare_xlsx(a, b):
    """逐格比两张表：格身份＝列行＋类型＋值；耗时列折整秒桶；工作表名与顺序入比对面。"""
    with zipfile.ZipFile(a) as za, zipfile.ZipFile(b) as zb:
        if workbook_sheets(za) != workbook_sheets(zb):
            return False
        names = [n for n in za.namelist() if n.startswith("xl/worksheets/sheet")]
        if sorted(names) != sorted(n for n in zb.namelist() if n.startswith("xl/worksheets/sheet")):
            return False
        sa, sb = load_strings(za), load_strings(zb)
        for n in names:
            ca = duration_norm(sheet_cells(za.read(n).decode("utf-8"), sa))
            cb = duration_norm(sheet_cells(zb.read(n).decode("utf-8"), sb))
            if ca != cb:
                return False
        return True


def drop_marked(raw, marks):
    """按行去掉登记过的行：返回（保留的字节, 去掉的行数）。

    marks 是 bytes 元组。判的是「这一行里有这个标记」，不认位置——日志行的前缀是时间戳与级别，
    位置会随时间变，标记不会。
    """
    lines = raw.split(b"\n")
    kept = [ln for ln in lines if not any(m in ln for m in marks)]
    return b"\n".join(kept), len(lines) - len(kept)


def tool_lines(path):
    """工具日志按行读；登记为「新版不记」的那类行两侧一起去掉，返回（保留的行, 去掉的行数）。

    两份内容都不在归档目录里，由 ab.sh 每场抄成 `<side>.tools.log`；抄不到时 ab.sh 留空文件，
    所以「两侧都空」是真没内容，不是没采到——采不到会表现为缺文件。
    """
    try:
        with open(path, "rb") as f:
            raw = f.read()
    except FileNotFoundError:
        return None
    kept, dropped = drop_marked(raw, OLD_ONLY_MARKS)
    return kept.split(b"\n"), dropped


def collapse_panels(raw):
    """面板帧一律抽出、只留**最后一帧**、挪到末尾；其余行按原顺序比。

    进度面板是**覆盖重画**：每 100ms 往同一块屏幕区域重画一次，画几帧取决于调度——同一个
    二进制连跑 5 次，b10 有 3 次比对方多或少一帧（2026-11-11 采样）。帧数与「第几帧插在
    哪一行中间」都不是任何一方的行为；**最后一帧的内容**才是：百分比、已完成数、成功/失败
    计数、条色彩带都得对。

    所以：帧数不判、位置不判（它本来就是覆盖重画，位置没有信息），内容照判。
    认出面板帧的判据是「这一行同时有『已完成 』和百分号」——节点结果行带不了这两样。
    """
    panels = []
    rest = []
    for ln in raw.split(b"\n"):
        if _is_panel(ln):
            panels.append(ln)
            continue
        rest.append(ln)
    tail = [panels[-1]] if panels else []
    return b"\n".join(rest + tail)


def _is_panel(ln):
    return "已完成 ".encode("utf-8") in ln and b"%" in ln


def expected_tty(name):
    """本场要不要 TTY 屏幕：先查场景表第 11 列；查不到再看 out/<场景>/tty（只给自检用）。"""
    tab = os.path.join(CODE, "scenes.tsv")
    if os.path.exists(tab):
        with open(tab, encoding="utf-8") as f:
            for line in f:
                if not line.strip() or line.startswith("#"):
                    continue
                cols = line.rstrip("\n").split("\x1f")
                if cols and cols[0] == name and len(cols) >= 11:
                    return cols[10].strip()
    p = os.path.join(SB, "out", name, "tty")
    if os.path.exists(p):
        with open(p, encoding="utf-8") as f:
            return f.read().strip()
    return ""


def expected(name):
    """本场该不该有归档：先查场景表第 10 列；查不到再看 out/<场景>/expect（只给自检用，真场景不许这么写）。"""
    tab = os.path.join(CODE, "scenes.tsv")
    if os.path.exists(tab):
        with open(tab, encoding="utf-8") as f:
            for line in f:
                if not line.strip() or line.startswith("#"):
                    continue
                cols = line.rstrip("\n").split("\x1f")
                if cols and cols[0] == name and len(cols) >= 10:
                    return cols[9].strip()
    p = os.path.join(SB, "out", name, "expect")
    if os.path.exists(p):
        with open(p, encoding="utf-8") as f:
            return f.read().strip()
    return ""


def main(name):
    out = os.path.join(SB, "out", name)
    bad = []

    code_o = open(os.path.join(out, "old.code"), "rb").read().strip()
    code_n = open(os.path.join(out, "new.code"), "rb").read().strip()
    if code_o != code_n:
        bad.append("退出码 old=%s new=%s" % (code_o.decode("utf-8", "replace"), code_n.decode("utf-8", "replace")))

    screen_o = read(os.path.join(out, "old.screen")) or b""
    screen_n = read(os.path.join(out, "new.screen")) or b""
    if screen_o != screen_n:
        bad.append("屏幕")

    added = 0
    for art in TEXT_ARTIFACTS:
        o = read(os.path.join(out, "old.history", art))
        n = read(os.path.join(out, "new.history", art))
        if o is None and n is None:
            continue
        if o is None or n is None:
            bad.append(art + ":缺文件")
            continue
        # 执行日志里有一行「有意新增」的留痕（`--yes` 跳过确认，4.5 的 04 号单）：
        # 两侧一起去掉再比，去掉几行打在结果行尾——它是登记过的差异，不是噪声。
        if art == "SSHFleetExec.log":
            o, co = drop_marked(o, NEW_ONLY_MARKS)
            n, cn = drop_marked(n, NEW_ONLY_MARKS)
            added = max(co, cn)
        if o != n:
            bad.append(art)

    for art in XLSX:
        o = os.path.join(out, "old.history", art)
        n = os.path.join(out, "new.history", art)
        if not os.path.exists(o) and not os.path.exists(n):
            continue
        if os.path.exists(o) != os.path.exists(n):
            bad.append(art + ":缺文件")
        elif not compare_xlsx(o, n):
            bad.append(art)

    ao = listing(os.path.join(out, "old.history", "assets"))
    an = listing(os.path.join(out, "new.history", "assets"))
    if ao != an:
        bad.append("assets")

    # 工具日志：09 判作「不比、也不该当差异」，可已知差异第一条的判法恰好要看它（E12）。
    to = tool_lines(os.path.join(out, TOOL_LOG[0]))
    tn = tool_lines(os.path.join(out, TOOL_LOG[1]))
    dropped = 0
    if to is None and tn is None:
        bad.append("工具日志:缺文件（两侧都没抄到）")
    elif to is None or tn is None:
        bad.append("工具日志:缺文件")
    else:
        dropped = max(to[1], tn[1])
        if norm(b"\n".join(to[0])) != norm(b"\n".join(tn[0])):
            bad.append("工具日志")

    # TTY 屏幕面（06 号单）：声明了 tty 的场次，两份伪终端屏幕都得在，且逐字节同。
    if expected_tty(name) == "tty":
        so = read(os.path.join(out, "old.tty"))
        sn = read(os.path.join(out, "new.tty"))
        if so is None and sn is None:
            bad.append("TTY 屏幕:缺文件（两侧都没采到）")
        elif so is None or sn is None:
            bad.append("TTY 屏幕:缺文件")
        elif collapse_panels(so) != collapse_panels(sn):
            bad.append("TTY 屏幕")

    lo = norm(open(os.path.join(out, "old.link"), "rb").read().strip().replace(b"old", b"<SIDE>"))
    ln = norm(open(os.path.join(out, "new.link"), "rb").read().strip().replace(b"new", b"<SIDE>"))
    if lo != ln:
        bad.append("latest_history")

    # 归档面：本场该不该有，按场景表的声明判，不按「两侧都没有就算一致」。
    exp = expected(name)
    if exp == "hist":
        for side in ("old", "new"):
            d = os.path.join(out, side + ".history")
            if not os.path.isdir(d):
                bad.append("缺归档：" + side + "（本场声明该有）")
            elif not os.listdir(d):
                bad.append("归档为空：" + side + "（本场声明该有）")
    elif exp == "-":
        # 声明该无的（a 组参数层、c1 被拒、c4／c5 起不来、d 组密钥命令）：两侧任一建了非空归档就报。
        # c1「被拒不许建归档目录」靠的就是这一条，否则两侧一起建了也「一致」。
        for side in ("old", "new"):
            d = os.path.join(out, side + ".history")
            if os.path.isdir(d) and os.listdir(d):
                bad.append("不该有归档：" + side + "（本场声明该无）")

    notes = []
    if dropped:
        notes.append("工具日志去掉 %d 行「新版不记」" % dropped)
    if added:
        notes.append("执行日志去掉 %d 行「新版多记」" % added)
    tail = "（%s）" % "；".join(notes) if notes else ""
    if bad:
        print("%s 差异：%s%s" % (name.ljust(24), "、".join(bad), tail))
        return 1
    print("%s 一致%s" % (name.ljust(24), tail))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1]))
