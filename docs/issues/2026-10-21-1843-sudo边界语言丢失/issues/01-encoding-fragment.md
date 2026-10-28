# 01: 下发行自带编码判断

**要做的**：目标机 locale 不是 UTF-8 时，工具让这条命令按字符算——`cut -c` 切中文不再切坏；本来就是 UTF-8 的机器上，这段判断运行时一个变量都不设。命令模式、脚本模式、代填三种下发形态同时生效，因为它们共用同一个环境段。

**Blocked by**：无，可立即开工

Status: done

- [ ] 片段生成接缝：编码判断整串逐字断言（含「不是 UTF-8 才动手」的四步形态），并断言整串不含单引号
- [ ] 缺省与边界四种情形各一条断言：`locale` 不存在、输出为空、只有 `C.utf8`、只有 `en_US.UTF-8` —— 前两种不设任何变量
- [ ] 下发行整串断言：命令模式、脚本模式、sudo 模式、代填各一条，断言到 `sh -c '<片段>; [sudo ]解释器 …'` 的完整形态
- [ ] 反例禁用词：任何下发行里都不出现 `sudo env`、`sudo -E`、`bash -c 'export`
- [ ] 日志里的「命令行」与实发串逐字相同（既有单点性质不破）
- [ ] 手工一条：非 UTF-8 机器上 `cut -c1-2` 出完整汉字；`--no-shell` 下发行与改前一致
- [ ] 两平台 `go test ./...` 全绿

## Comments

- 2026-10-28：按 ADR-0030 建立。测试接缝即规格「测试接缝」一节的第一个口子，不新增别处。
- 2026-10-28：实施。片段落在 `internal/ssh`，拼在 `[env]` 的 export 段之后、解释器之前——判断的是最终生效的 locale。
- 2026-10-28：现场。rootless podman 在本会话起不来（`newuidmap: write to uid_map failed`，沙盒不给嵌套 user namespace，`unshare -Ur` 同样被拒）。改用 rootful 引擎指回用户镜像库能起：`sudo podman --root ~/.local/share/containers/storage --runroot /run/user/1000/containers`。代价是镜像里 `/etc`、`/usr/bin` 属主是 1000，容器内 sudo 与 CentOS 的 sshd 因此起不来，chown 回 0:0 后恢复——rootful 走用户库的副产物，不是镜像问题。
- 2026-10-28：边界情形先在 `build/atest_2026-10-28_13-45-00/` 用假 `locale` 命令验：UTF-8 机器不设、只有 `C.utf8` 设它、只有 `en_US.UTF-8` 设它、`locale -a` 空输出不设、无 `locale` 命令不设且无告警。
- 2026-10-28：两台容器上端到端（sshd 以无 `LANG` 的环境启动，登录后 `charmap` = `ANSI_X3.4-1968`，即失败现场）。Ubuntu 26.04：带片段 `cut -c1-2` 出 `344 270 255 346 226 207`（完整「中文」），`LC_CTYPE=C.utf8`；同一台加 `--no-shell` 只剩 `344 270` 两个字节（半个字）。CentOS 7.9 没有 `C.utf8`、只有 `en_US.utf8`，片段挑到它，同样出完整「中文」；`--no-shell` 同样出半个字。
- 2026-10-28：顺带在容器里验了 `LC_CTYPE` 穿过 sudo（`export LC_CTYPE=C.utf8; sudo sh -c …` 内 charmap=UTF-8、`cut` 出完整汉字），与 ADR-0024 3.1 的结论一致。
- 2026-10-28：实测。`LC_ALL=C` 下 `cut -c1-2` 出 3 字节（半个汉字），只设 `LC_CTYPE=C.utf8` 才是完整两个汉字——这就是「永不设 LC_ALL」那条不变式的现场依据。
- 2026-10-28：片段 162 字节（ADR 与规格原写 160，已就地改成实测值）。
