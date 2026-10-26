# 02: 取文件名与后缀改用与路径归属一致的规则

**要做的**：路径不再被改写之后，「从路径里取一段」的地方必须与路径归属对得上：本机路径按本机规则取，远端路径按远端规则取。涉及三处——自动备注（脚本名、上传名、下载名）、脚本后缀的解释器识别、下载时落到本机的文件名。现在这三处混着用：本机路径按远端规则切，远端路径又按本机规则切。差别只在 Windows 上显形（那一边的规则同时认 `\`），Linux 上两种切法结果一样——所以下面的验收大多写着「Windows 上」。**只负责「名字取对」**：取出来的名字在本机写不下去时，按下载失败报出来，不许改名换地方落盘；怎么拼目录不在本轮（见 Out of Scope）。

Status: done
Blocked by: 无（可立即开工）—— 与 01 是配套关系，不是被它阻塞：01 去掉 `normalizePath` 之后，若不同时落本条，Windows 上 `path.Base` / `path.Ext` 会把整段路径当名字（2026-10-27 订正方向，依据 `audit.md` 5.5 第 6 条）

- [x] Windows 上 `-s D:\a\b.sh` 的自动备注是 `b.sh`（看参数确认屏的「备注」行）
- [x] Windows 上 `-u D:\a\b.txt -p /opt/` 的自动备注是 `b.txt`
- [x] Windows 上 `-s D:\dir.v2\deploy`（没有后缀）：解释器提示里显示的后缀是空，不是 `.v2\deploy`
- [x] Windows 上 `-d /var/log/app.log` 落到本机的文件名是 `app.log`
- [x] Windows 上 `-d /var/log/a\b.log`（远端那个文件的名字里带反斜杠）：明细里显示的文件名是整段 `a\b.log`，不是 `b.log`。**本机写不下去就按「下载失败」报出来，不许悄悄改成 `b.log` 落盘**——`\` 在 Windows 上不是合法文件名字符，这条钉的是「名字不被偷偷换掉」
- [x] Linux 上同样下载 `a\b.log` 仍落成 `a\b.log`（回归：两边本来就一致，不能改坏）
- [x] 全量 `go test ./...` 除开工前就红的那条（清理归 06）外无新增红

## Comments

- 2026-10-26 落地（客户端 Windows，目标机 WSL2 Ubuntu 26.04，账号 `wbverify`）。
- 备注与后缀：`-s D://…//b.sh` → `r='b.sh'`；`-u D://…//b.txt` → `r='b.txt'`；`-s D://…//dir.v2//deploy` → Trace 原文 `脚本后缀 "" 未配解释器`。
- 下载落盘名：`-d /tmp/app.log` 成功，落 `dl\127.0.0.1\app.log`；`-d "/tmp/a\b.log"` 的明细原文是 `a\b.log: 下载失败 - sshfleet.transfer.local_file_failed: name not usable as a local file name: a\b.log`，分类「本机写入失败」，`dl\` 没多出文件。
- Linux 回归（客户端也是 Linux）：同一条命令落成 `dl/127.0.0.1/a\b.log`，成功 1，没改坏。
- 实现注记：只判 `filepath.IsLocal` 不够——Windows 上 `IsLocal("a\b.log")` 是**真**（它只表示「相对且留在子树内」），照原名落盘会平白多一层 `a\` 目录（实测踩到）。判据改成 `filepath.Base(name) != name || !filepath.IsLocal(name)`，前半挡分隔符、后半挡保留设备名。
- 目录下载那支同样覆盖了（复查补的）：`find` 枚举出来的相对路径逐段过 `localRelUsable`。实跑目标机 `/tmp/dltest` 里同时有 `a\b.log` 与 `ok.log`——`a\b.log` 按「本机写入失败」报出，`ok.log` 正常落地，`dl\` 下没多出 `a\` 目录。
- 2026-10-27：订正 `Blocked by`。原文写「01（取值先变原样，Windows 上才看得出切法对不对）」，方向是反的——改前 `normalizePath` 先把 `\` 换成 `/`，`path.Base` 本来就切得对；真正会「整段当名字」的时点是 01 删掉 `normalizePath` **之后**，本条是去堵那个洞的。依据 `audit.md` 5.5 第 6 条（Windows 侧实测：改前 `r=` 已是 `b.sh` / `b.txt`）。两条都已交付，改的只是记录，不影响已发的代码与验收。
