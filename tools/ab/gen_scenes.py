#!/usr/bin/env python3
"""生成 scenes.tsv（`\\x1f` 分隔，九列）。表格里不许用制表符，改这张表就改这里再跑一遍。

列：name, machine(1=要机器), stdin, conf(本场改配置一格), pre(本侧开跑前先跑，或 ticket:NN),
    reset(远端现场复位), env(本侧这一次跑注入的环境变量，空格分隔多组), kill(到点给整组发 SIGINT 的秒数), argv
"""
import os

R = "sudo rm -rf /tmp/fleet09-up && sudo mkdir -p /tmp/fleet09-up && sudo chmod 777 /tmp/fleet09-up"
UP = R
COLLIDE = R + " && echo old-one | sudo tee /tmp/fleet09-up/one.txt >/dev/null"
DOWN = ("sudo rm -rf /tmp/fleet09-down && sudo mkdir -p /tmp/fleet09-down"
        " && sudo sh -c 'printf \"alpha\\n\" > /tmp/fleet09-down/a.txt; printf \"beta\\n\" > /tmp/fleet09-down/b.txt;"
        " ln -s a.txt /tmp/fleet09-down/link.txt; chmod -R a+rX /tmp/fleet09-down'")
T1EX = "echo t1:OldPass#2026 | sudo chpasswd && sudo chage -d 0 t1"
T1OK = "echo t1:OldPass#2026 | sudo chpasswd && sudo chage -d 2026-11-06 t1"
# 密钥那几场：一把写死的密钥，一边种进 rc（KEYRC），一边塞进环境（KEY）。
KEY = "SSHFLEET_KEY=FIXEDKEY-AB-0001"
KEYRC = "bash ../fixtures/seedkey.sh FIXEDKEY-AB-0001"
# 加密开着时清单第 4 列要填凭据文件的绝对路径，配置里那一格也得是；两处都用这个共享路径，
# 两侧读的是同一个文件（只读，不写），屏上那行路径才一致。
PW = "/tmp/fleet09-pw"
# 引号不能省：老版用 BurntSushi/toml，裸值即语法错、当场起不来（新工程自解析，认裸值——
# 这条口径差另记在 audit「未归入的观察」里，不混进本组的比对结论）。
CONF_ENC = 'encrypt=true password="' + PW + '"'

rows = [
    # A 哨兵：不碰机器，逐字比
    ("a1-help", 0, "", "", "", "", "-h"),
    ("a2-noargs", 0, "", "", "", "", ""),
    ("a3-unknown-option", 0, "", "", "", "", "--nope"),
    ("a4-missing-value", 0, "", "", "", "", "-c"),
    ("a5-two-keycmds", 0, "", "", "", "", "--key-status --gen-key"),
    ("a6-key-extra", 0, "", "", "", "", "--key-status extra"),
    ("a7-key-with-batch", 0, "", "", "", "", '--key-status -c uptime -f u1.csv'),

    # B 五种模式的成与败
    ("b1-command", 1, "", "", "", "", '-f u1.csv -c "echo hello" --yes'),
    ("b2-command-fail", 1, "", "", "", "", '-f u1.csv -c "exit 3" --yes'),
    ("b3-unreachable", 1, "", "", "", "", '-f bad.csv -c "uptime" --yes'),
    ("b4-script", 1, "", "", "", "", "-f u1.csv -s hello.sh --yes"),
    ("b5-script-answer", 1, "", "", "", "",
     '-f u1.csv -s ask1.sh -a "y,Continue?,Press enter to go on:" --yes'),
    # 回退非 bash 要答两次：先「继续用 bash 作为解释器吗」，再确认执行（`\n` 由 ab.sh 拆行）。
    ("b6-script-fallback", 1, "y\\ny", "", "", "", "-f u1.csv -s deploy.xyz"),
    ("b7-upload", 1, "", "", "", UP, "-f u1.csv -u ./updir -p /tmp/fleet09-up/ --yes"),
    ("b8-upload-collide", 1, "", "", "", COLLIDE, "-f u1.csv -u ./updir -p /tmp/fleet09-up/ --yes"),
    ("b9-download", 1, "", "", "", DOWN, "-f u1.csv -d /tmp/fleet09-down -p ./down/ --yes"),
    ("b10-passwd-expired", 1, "", "", "", T1EX,
     '-f t1.csv --change-password "Fleet#New2026" --yes'),
    ("b11-passwd-valid", 1, "", "", "", T1OK,
     '-f t1.csv --change-password "Fleet#New2026" --yes'),
    # 内联记录那一场走确认屏（stdin 答 y，不挂 --yes）：已知差异第一条要看的正是
    # 确认屏、report.txt、工具日志三处那条命令行各露多少。
    ("b12-inline-f", 1, "y", "", "", "",
     '-f "127.0.0.1,2201,sshfleet,Sshfleet#2026" -c "whoami"'),
    # 多节点：两台里一台通、一台不通，比统计块与退出码（07 号单单列的一条验收）。
    # 命令用 `whoami` 不用 `uptime`：后者的输出自带墙上时刻，两侧必然差一秒。
    ("b13-two-nodes", 1, "", "", "", "", '-f two.csv -c "whoami" --yes'),

    # C 场面：拒绝、问话全免、中断、起不来
    ("c1-gate-refuse", 1, "n", "", "", "", '-f u1.csv -c "rm -rf /tmp/whatever"'),
    # 问话全免：`--yes` 跳过确认屏。命令用 `whoami`，不用 `uptime`——后者输出自带墙上时刻，
    # 两侧差一秒就成了「差异」（b13 踩过同一脚，这一场原来没跑过）。
    ("c2-yes-all", 1, "", "", "", "", '-f u1.csv -c "whoami" --yes'),
    # 中断：两台、`-n 1` 串行，第一台 0.5 秒内收工，第二台停在 `sleep 30` 里；
    # 3 秒时给整组发 SIGINT（终端 Ctrl+C 的口径）。标记文件让「哪台快哪台慢」可重复：
    # 第一台上没有标记就建标记、立刻返回，第二台上标记已在就睡下去。
    # 标记在目标机上、由 sshfleet 用户创建，/tmp 有粘滞位——`rm` 得带 `sudo`，否则删不掉
    # 且不报错：新侧会从「标记已在」起跑，第一台直接睡下去，比出来的是另一场。
    ("c3-interrupt", 1, "", "",
     "sudo rm -f /tmp/fleet09-int", "",
     '-f int2.csv -n 1 -c \'if [ -f /tmp/fleet09-int ]; then sleep 30; else touch /tmp/fleet09-int; echo node-done; fi\' --yes'),
    ("c4-start-no-config", 1, "", "", "mv config/SSHFleet.conf config/SSHFleet.conf.away", "",
     '-f u1.csv -c "uptime" --yes'),
    # 必须项缺：配置在，删掉 `[account] port` 那一行。
    ("c5-start-missing-key", 1, "", "", "sed -i '/^port /d' config/SSHFleet.conf", "",
     '-f u1.csv -c "uptime" --yes'),

    # D 密钥与语言。KEYRC 那一手是给本侧 HOME 种一条**写死**的主密钥行：指纹是随机密钥的
    # 产物，两侧各生成一把必然不同，种同一把指纹才可比。d2 的 `--gen-key` 照跑（生成那条路
    # 要过），跑完把密钥钉成写死那把，免得屏上那 8 位十六进制成了「差异」。
    ("d1-key-empty", 0, "", "", "", "", "--key-status"),
    ("d2-key-generated", 0, "", "", "./sshfleet --gen-key >/dev/null && " + KEYRC, "", "--key-status"),
    # 两处不一致：rc 里一把（FIXEDKEY-AB-0001）、环境里另一把（FIXEDKEY-AB-0002）。
    ("d3-key-diverged", 0, "", "", KEYRC, "", "--key-status"),
    ("d4-genkey-keep", 0, "n", "", KEYRC, "", "--gen-key"),
    ("d5-genkey-yesflag", 0, "", "", KEYRC, "", "--gen-key --yes"),
    ("d6-convert-to-cipher", 0, "", CONF_ENC, KEYRC, "", "--convert-secret sec.txt"),
    # 「已是目标形态」：本侧先自己转一遍，再让这一场转第二遍。
    ("d7-convert-already", 0, "", CONF_ENC, KEYRC + " && " + KEY + " ./sshfleet --convert-secret sec.txt", "",
     "--convert-secret sec.txt"),
    ("d8-convert-unreadable", 0, "", CONF_ENC, "", "", "--convert-secret nope.txt"),
    # 反向：先把 sec.txt 转成密文，再把开关拨回 false，这一场从密文转回明文。
    # 屏上要比，落盘的明文也要比——`cmp.py` 不认工作目录里的文件，那一笔靠手工核（见 audit）。
    ("d12-convert-to-plain", 0, "", 'encrypt=false password="' + PW + '"',
     "printf 'Sshfleet#2026' > " + PW + " && " + KEYRC
     + " && python3 ../prep.py config/SSHFleet.conf encrypt=true && " + KEY
     + " ./sshfleet --convert-secret sec.txt && python3 ../prep.py config/SSHFleet.conf encrypt=false",
     "", "--convert-secret sec.txt"),
    # 预检查提醒：加密开着 + 两处不一致。`.zshrc` 放「本机已保存」那把（A），`.bashrc` 放
    # 「本次运行」那把（B）——工具以当前 shell 的 rc 为准去解密，所以凭据按 B 转，解得开、
    # 执行照跑。这一场要看的正是「提醒归提醒，结果与退出码不变」。
    ("d9-precheck", 1, "", CONF_ENC,
     "printf 'Sshfleet#2026' > " + PW
     + " && bash ../fixtures/seedkey.sh ANOTHER-KEY-0002 .bashrc"
     + " && SSHFLEET_KEY=ANOTHER-KEY-0002 ./sshfleet --convert-secret " + PW
     + " && bash ../fixtures/seedkey.sh FIXEDKEY-AB-0001 .zshrc",
     "", '-f u1enc.csv -c "whoami" --yes'),
    ("d10-english-false", 1, "", "english=false", "", "", '-f u1.csv -c "whoami" --yes'),
    ("d11-english-true", 1, "", "english=true", "", "", '-f u1.csv -c "whoami" --yes'),
]

# envs 是每场给本侧注入的环境变量（空格分隔多组），没列的场景不注入。
envs = {
    "d3-key-diverged": "SSHFLEET_KEY=FIXEDKEY-AB-0002",
    "d6-convert-to-cipher": KEY,
    "d7-convert-already": KEY,
    "d12-convert-to-plain": KEY,
    "d9-precheck": "SSHFLEET_KEY=ANOTHER-KEY-0002",
}

# kills 是每场到点发 SIGINT 的秒数，没列的场景不发（跑完为止）。
kills = {"c3-interrupt": "3"}

# expects 是第 10 列「本场该不该有归档」：hist＝该有，-＝该没有。
# 09 那把尺子把「两侧都没有归档」当成一致（E7／E8），19 场因此只比了屏幕与退出码；
# 这一列把「本该有却没有」和「本来就不该有」分开。c1 是后者：被拒不许建归档目录。
expects = {n: "hist" for n in (
    "b1-command", "b2-command-fail", "b3-unreachable", "b4-script", "b5-script-answer",
    "b6-script-fallback", "b7-upload", "b8-upload-collide", "b9-download",
    "b10-passwd-expired", "b11-passwd-valid", "b12-inline-f", "b13-two-nodes",
    "c2-yes-all", "c3-interrupt", "d9-precheck", "d10-english-false", "d11-english-true",
)}

# ttys 是第 11 列：本场要不要在伪终端里跑（06 号单）。起步三场——帮助（折行形态）、
# 命令模式（进度面板）、改密（面板与逐行明细）；其余场次仍走重定向采集。
ttys = {"a1-help", "b1-command", "b10-passwd-expired"}

head = ("# 09 号单的固定输入集。字段用 \\x1f 分隔（制表符会被 read 当空白折叠，空字段会丢）：\n"
        "# 场景名 | machine(1=要机器) | stdin | conf | pre(或 ticket:NN) | reset | env | kill(秒) | argv | expect(hist/-) | tty\n")
path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "scenes.tsv")
# newline="\n" 是硬要求：在 Windows 的 Python 下按默认换行会落成 CRLF，`read` 把行尾的 \r
# 粘到最后一个字段上（本轮 tty 列因此永远不等于 "tty"，`--tty` 从没传下去）。
with open(path, "w", encoding="utf-8", newline="\n") as f:
    f.write(head)
    for r in rows:
        assert len(r) == 7 and all("\x1f" not in str(x) for x in r), r
        env = envs.get(r[0], "")
        assert "\x1f" not in env, r
        kill = kills.get(r[0], "")
        tty = "tty" if r[0] in ttys else ""
        f.write("\x1f".join(str(x) for x in list(r[:6]) + [env, kill, r[6], expects.get(r[0], "-"), tty]) + "\n")
print(f"写了 {len(rows)} 场 → {path}")
