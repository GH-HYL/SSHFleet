#!/usr/bin/env python3
"""造最小 xlsx 供口径自检：mkbook.py <dest> <工作表名> <格>=<类型>:<值> …

类型 `n` 数值进 `<v>`，`s` 文本进 sharedStrings 并带 `t="s"`。
只写 cmp.py 会读的三个部件（workbook／sheet1／sharedStrings），不是可用 Excel 文件，
只用来证明判据真在看哪几样——它可以坏、可以假，但不能被当成真产物。

例：mkbook.py out/x/old.history/results.xlsx Results G1=s:连接耗时 G2=n:0.209061886
"""
import sys
import zipfile

HEAD = '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n'
NS = 'xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"'
WORKBOOK = HEAD + '<workbook ' + NS + '><sheets><sheet name="%s" sheetId="1"/></sheets></workbook>'
STRINGS = HEAD + '<sst ' + NS + ' count="%d" uniqueCount="%d">%s</sst>'
SST_ITEM = '<si><t>%s</t></si>'


def esc(s):
    return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def main(dest, sheet_name, specs):
    strings, cells = [], []
    for spec in specs:
        ref, _, rest = spec.partition("=")
        kind, _, value = rest.partition(":")
        if not ref or not value:
            raise SystemExit("格说明写坏了：%r（要 格=类型:值）" % spec)
        if kind == "s":
            cells.append('<c r="%s" t="s"><v>%d</v></c>' % (ref, len(strings)))
            strings.append(SST_ITEM % esc(value))
        else:
            cells.append('<c r="%s"><v>%s</v></c>' % (ref, esc(value)))
    rows = {}
    for c in cells:
        ref = c.split('"')[1]
        rows.setdefault(int("".join(ch for ch in ref if ch.isdigit())), []).append(c)
    body = "".join('<row r="%d">%s</row>' % (r, "".join(rows[r])) for r in sorted(rows))
    sheet = HEAD + '<worksheet ' + NS + '><sheetData>' + body + '</sheetData></worksheet>'
    with zipfile.ZipFile(dest, "w", zipfile.ZIP_DEFLATED) as z:
        z.writestr("xl/workbook.xml", WORKBOOK % sheet_name)
        z.writestr("xl/worksheets/sheet1.xml", sheet)
        z.writestr("xl/sharedStrings.xml", STRINGS % (len(strings), len(strings), "".join(strings)))


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2], sys.argv[3:])
