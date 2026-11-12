import re, sys

# 09 号单：把现场配置改成连本机 sshd、明文口令、可关加密。
# 用法：prep.py <conf> key=value [key=value ...]
path = sys.argv[1]
pairs = [a.split("=", 1) for a in sys.argv[2:]]

text = open(path, encoding="utf-8").read()
for key, value in pairs:
    text, n = re.subn(r"(?m)^([ \t]*" + re.escape(key) + r"[ \t]*=[ \t]*)\S.*$",
                      lambda m: m.group(1) + value, text, count=1)
    if n != 1:
        sys.exit("没找到这一格：" + key)
open(path, "w", encoding="utf-8").write(text)
