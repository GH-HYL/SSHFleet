# M6-28 打包：两个构建脚本加 release 模式

Type: task
Status: resolved
Resolved: 2026-09-14
Blocked by: 27

## 范围

`build-windows.bat` / `build-linux.sh` 增加 `release` 参数（用户 2026-09-14 裁定：**用现有脚本加参数，不新增文件**，因此不涉及 `.gitignore` 白名单变更）。

- 不带参数：只编译（行为与之前一致）
- 带 `release`：编译 → 组装 `release/SSHFleet_<版本>_<平台>/` → 压缩
- 带任意参数都会跳过结尾暂停（bat 侧），双击（无参数）行为不变

**版本号**：从 `CHANGELOG.md` 顶部取第一个形如 `## [x.y.z]` 的正式版本号（bat 用 `findstr` + `for /f` 按 `[]` 取第二段；sh 用 `grep -oE` + `tr -d`）；取不到则用当天日期。

**发布目录内容**：可执行文件 + `README.md` + `config/`（`SSHFleet.conf` + `dangerous_keywords.toml` + `error_keywords.toml`）。
> spec 的 M6 表原来只写了「可执行文件 + config/SSHFleet.conf 模板 + README.md」——**漏了两个规则文件**，而缺了它们工具起不来（配置里的 `paths.*` 指向它们）。已按实际需求补齐并同步 spec。

**分平台打包（用户裁定：分平台惯例）**：

| 平台 | 容器 | 工具 | 理由 |
| --- | --- | --- | --- |
| Windows | `.zip` | 系统自带的 `tar.exe`（bsdtar，按扩展名自动选格式） | 双击即可解压；实测可用 |
| Linux | `.tar.gz` | GNU tar | 能保住执行权限 |

**各平台的包在各自平台上打**（不交叉打）：实测 Windows 的 bsdtar 打出的 tar.gz 里 Linux 二进制是 `-rw-rw-rw-`——**执行权限丢失**，Linux 用户得自己 `chmod +x`，这不是发布包该有的样子。故 Linux 包由 `build-linux.sh release` 负责。

**权限兜底**：真实 Linux 上 tar 天然保留 0755；Git Bash（MSYS）下 `chmod` 对 NTFS 无效（实测 `chmod +x` 后 `ls` 仍是 `-rw-r--r--`），故 sh 侧检测 GNU tar 后加 `--mode=755` 兜底。代价是归档内文本文件也是 755——对目录是必需的，对文本文件无害，已在脚本注释里写明；非 GNU tar（如 macOS 的 bsdtar）不支持该选项，脚本先探测再决定是否加，探测不到时退回普通打包。

## 验证

| 项 | 结果 |
| --- | --- |
| `./build-linux.sh release` | 产出 `release/SSHFleet_5.0.0_linux/{SSHFleet, README.md, config/×3}` + `release/SSHFleet_5.0.0_linux.tar.gz`（9.2MB） |
| `build-windows.bat release` | 产出 `release/SSHFleet_5.0.0_windows/{SSHFleet.exe, README.md, config/×3}` + `release/SSHFleet_5.0.0_windows.zip`（9.4MB），退出码 0，stderr 空 |
| 版本号提取 | 两侧都取到 `5.0.0`（与 CHANGELOG 一致），未走日期兜底 |
| 归档内容 | `tar -tzvf` 显示 linux 包为 `-rwxr-xr-x`（二进制可执行）；`tar -tf` 显示 zip 内 7 个条目、层次正确 |
| 解压校验 | 两个包分别解压，目录结构与本机 workspace 一致；zip 内 `SSHFleet.exe` 为可执行 |
| **包内程序真能跑** | 解压 `SSHFleet_5.0.0_windows` 后在该目录直接运行 `SSHFleet.exe`（不带参数）：正常打印帮助（选项表里的默认值来自包内自带配置，证明配置加载成功），退出码 0，并在该目录自动建出 `historys/` |
| 回归 | 不带参数时两个脚本行为与之前一致；`md5sum` 比对两脚本产出的二进制仍完全相同 |

## Comments

- 2026-09-14 完成。过程中的两个坑：① 压缩包路径给原生 `tar.exe` 时必须用 Windows 形式（`C:/…`），传 MSYS 的 `/c/…` 会报 `could not chdir`——与之前 `go build -o /d/d/…` 同源；② bat 的 release 分支里 `where tar` 检查是必要的，老系统（Windows 10 1803 之前）没有自带 tar，会以「找不到 tar」明确报出而不是莫名失败。
- `release/` 与 `build/` 均在白名单之外（被 `*` 忽略），打包产物不会进版本库。
