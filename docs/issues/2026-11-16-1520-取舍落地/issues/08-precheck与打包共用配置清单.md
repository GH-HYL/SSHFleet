# 08: `precheck` 与打包共用一份配置清单

**要做的**：`tools/release/release.py` 里「包里要放哪几份配置」现在有两处说法——`precheck()`（`:174`）查三份，`stage_pkg()`（`:241,248`）拷五份。抽成一份清单，两处都读它；缺文件时走 `die()` 报 `[失败]`，不再抛 `shutil.copy2` 的 traceback。

**Blocked by**：无，可立即开工。

**Status**：done

- [ ] 一份常量描述包里那五份配置：`keywords_dangerous.conf`、`keywords_error.conf`、`keywords_passwd.conf`、`SSHFleet-for-windows.conf`、`SSHFleet-for-linux.conf`
- [ ] `precheck()` 按这份清单逐份查（现在漏查 `keywords_error.conf`、`keywords_passwd.conf`）
- [ ] `stage_pkg()` 的两处 `shutil.copy2` 前先判存在，缺则 `die(f"找不到：{p}")`；失败时屏上是 `[失败] 找不到：…` 且退 1，不是 Python traceback
- [ ] 顺手核一遍这五份是不是打包真正需要的全集（`config/` 下还有没有别的按平台要拷的）
- [ ] 现有四条前置检查不动：工作区干净、`MOD`、`DOCS_SRC` 与手册 glob、`MAIN_GO`
- [ ] 验法：在 `build/tmp/` 里复制一份仓库、删掉 `config/keywords_passwd.conf`，跑 `precheck()` 应打印 `[失败] 找不到：…` 并退 1

## Comments

**2026-11-16 拆单**：这条只管「少一份怎么报」，不管「要不要少」——五份是否都该随包是另一件事。`python tools/release/release.py` 会真跑一遍发版，所以验法用复制出来的副本，不在本仓直接跑。

**2026-11-16 落地**：加了 `PKG_KEYWORDS`（三份关键词表）与 `PKG_PROFILE`（按平台取的那份出厂配置），`precheck()` 现在按这两样查满五份；`stage_pkg()` 的两处拷贝改走 `copy_or_die()`，缺文件时 `[失败] 找不到：<路径>` 并退 1。

实测（把 `CFG` 指到一份删掉了 `keywords_passwd.conf` 的副本，跑 `precheck()`）：`[失败] 找不到：…\config\keywords_passwd.conf`，退出码 1。改前这份根本不在检查范围——它会在打包时以 `shutil.copy2` 的 traceback 出现。
