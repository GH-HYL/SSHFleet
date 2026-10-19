# 04: forbidden 族重写——六条必须封住

Status: done
Blocked by: 01, 02, 03

**要做的**：把 `forbidden` 档收成 6 条，每条都**堵住实测确认会漏的写法**。做完之后，真正会毁机器的动作一条都跑不掉，而正当运维动作不会再被整批拒绝。

---

## 为什么这档要单开工单

开发者原话：**「forbidden 族我比较看重，所以单开工单可以，他的内容一定要封堵住」**。

理由也在这里：`forbidden` 是**唯一会让整批作业作废**的档位——命中就打印警告、`os.Exit(1)`，**连 `--yes` 也不放行**（`main.go:302-307`）。所以这一档有两个反方向的硬要求，必须逐条对着实测数据写：

- **不能漏**：漏一条，200 台机器同时执行危险命令。
- **不能误伤**：误伤一条，整个批量作业白跑（实测 `shutdown -r +10`、`init 6` 现在就落在这里）。

判据只有一条（`spec.md` 定案 F）：**这个动作在生产批量作业里有没有正当形态？** 没有才留 forbidden。

## 六条要封住的规则

现 10 条 forbidden 收成 6 条。**每条都要挂多条正则**（定案 A / H：旗标与写法由规则自己列全）。

### 1. 递归删除根目录

吞并原「使用 --no-preserve-root 危险旗标」。

必须封住：`rm -rf /`、`rm -r -f /`、`rm -fr /`、`rm --recursive --force /`、`rm --no-preserve-root -rf /`、`rm -rf //`、`rm -rf /.`、`rm -rf /*`。

### 2. 递归删除系统目录

必须封住：`rm -rf /etc`、`rm -rf /usr`、`rm -rf /var`、`rm -rf /lib`、`rm -rf /bin`、`rm -rf /sbin`、`rm -rf /boot`，以及 `rm -rf /etc/`、`rm -rf /etc/*` 这类「清空整个系统目录」的写法。

**不封**（归 medium 的「删除关键路径文件」）：删系统目录**内部**的某个文件或子目录，如 `/etc/nginx`、`/var/log/app.log`。

### 3. 格式化主盘

原「格式化系统盘」。**不分系统盘与数据盘，凡格式化盘设备一律命中**（`spec.md` 定案 J）。必须封住：`mkfs /dev/sda`、`mkfs.ext4 /dev/sda1`、`mkfs -t xfs /dev/sda`、`mkfs.xfs /dev/nvme0n1`、`mkfs.ext4 /dev/vdb1`。

### 4. 抹盘 dd

原「销毁系统数据」。必须封住：`dd if=/dev/zero of=/dev/sda`、`dd if=/dev/urandom of=/dev/sdb`、`dd if=/dev/zero of=/dev/nvme0n1`。**同样不分主次盘符**。

> **这条现在有个真漏洞**：正则只认 `(?:sd|hd|vd)`，**云主机的 `/dev/nvme0n1` 全漏**（`audit.md` 8.3 实测）。虽然补新命令不在本轮（定案 G），但**把 nvme 补进这条已在的规则不违反定案 G**——它属于「封住既有规则」，不是「新增规则」。**照做。**

### 5. 关机停机

原「系统关机」。现正则 `\bshutdown\b` 把**取消**动作也拦了——实测 `shutdown -c`、`shutdown +0 -c` 判 forbidden，**想撤销误发的重启，工具反而不让撤销**（`audit.md` 8.1）。

必须封住：`shutdown -h now`、`shutdown -P now`、`shutdown now`、`shutdown -h +10`、`poweroff`、`halt`、`init 0`、`telinit 0`。

必须**不封**：`shutdown -c`、`shutdown +0 -c`、`shutdown -h +10 -c`（撤销动作）。

**写法已在沙盒验证过，直接用**（RE2 没有负向断言，靠整行锚定 + 字符类排除）：

```
(?:^|[;&|]\s*)shutdown\s[^\n]*?(?:-h\b|-P\b|\bnow\b)(?:[^-\n]|-(?:[^c\n]|$))*$
```

两个坑（我都踩过）：① **不能省行尾锚定**——只写到 `(?:-h|-P|\bnow\b)` 就早退，后面的 `-c` 看不见，实测漏了 `shutdown -h +10 -c`；② **不能靠「先匹配再过滤」**——`rules.go` 的 `match` 只有 `MatchString`，没有后处理钩子，过滤必须编进正则本身。

### 6. 杀死所有进程

原「杀死所有进程」，保留。必须封住：`kill -9 -1`、`kill -KILL -1`。

> 这条**零语料覆盖**（`audit.md` 第七节），03 工单要先给它补用例。

## 不要做的事

- **不要新增第六条目之外的任何 forbidden 规则**。
- **不要把 `reboot` / `init 6` / `shutdown -r` 留在这里**——它们降 high（见 05）。
- **不要动 high / medium 的规则**。

## 验收

逐条对着上面的「必须封住 / 必须不封」跑：

- [ ] 第 1 条：8 种写法全部命中 forbidden（含 `//`、`/.`、`/*`）
- [ ] 第 2 条：7 个系统目录的「目录本身」与「清空内容」全部命中；`/etc/nginx`、`/var/log/app.log` **不命中**本条
- [ ] 第 3 条：5 种 mkfs 写法（含 `nvme0n1`、`vdb1`）全部命中 forbidden
- [ ] 第 4 条：`/dev/sda`、`/dev/sdb`、**`/dev/nvme0n1`** 全部命中 forbidden（不分主次盘）
- [ ] 第 5 条：8 种真停机写法全部命中；**`shutdown -c`、`shutdown +0 -c`、`shutdown -h +10 -c` 一条都不命中**
- [ ] 第 6 条：`kill -9 -1`、`kill -KILL -1` 全部命中
- [ ] `forbidden` 档总共**恰好 6 条**，不多不少
- [ ] 跑一遍日常运维命令表（`audit.md` 第二节那 31 条），**没有任何一条正当命令落进 forbidden**
- [ ] 语料测试通过；**不通过的用例先分辨是规则错还是期望错**，拿不准的交开发者定，不许改期望凑绿

## Comments

- 2026-10-19：建立。开发者指定单开，理由是「内容一定要封堵住」。验收按「必须封住 / 必须不封」双向列，不只测该拦的。
- 2026-10-19（开发者定）：**盘符不分主次**——`mkfs` / `dd` 一律 forbidden，`/dev/sda` 与 `/dev/nvme0n1` 一视同仁（`spec.md` 定案 J）。连带：原 `high` 的「格式化非主盘」「dd 覆盖非主盘」两条**整条删除**，最终条数由 19 改 17。
- 2026-10-19：**已按边界表落地**。系统目录按「目标是不是目录本身」分段枚举：`/etc`、`/usr`、`/var`、`/var/log`、`/usr/lib`、`/root` 归本条；更深一层（`/etc/nginx`、`/var/log/app.log`）不计。`--no-preserve-root` 单独写出也当删根；`/root/.ssh` 这类管理密钥归本条；反引号里的 `rm -rf /etc` 也认。`shutdown -c` 系列撤销写法按 05 的边界全部放行。边界表 `TestRuleBoundaries` 全绿，语料 162/162。
