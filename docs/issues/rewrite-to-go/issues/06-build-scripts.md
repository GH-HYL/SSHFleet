# M1-06 构建脚本与双平台交叉编译验证

Type: task
Status: resolved
Resolved: 2026-09-11
Blocked by: 01

## 范围

工作区根（**工作区级**目录，不在工程内）：

- `tools/build-windows.bat`（双击即可跑）
- `tools/build-linux.sh`（供 Linux / 自动化使用）
- 目标平台：`windows/amd64` + `linux/amd64`；编译输出工作区根 `build/`；产物名 `SSHFleet`（Windows 为 `SSHFleet.exe`）
- 发布目录组装与压缩属 M6，本工单只交付「编译 → 落位」——M1 收尾即验证双平台交叉编译通不通，不拖到 M6
- 白名单：`tools/build-*` 已在 `.gitignore` 放行范围（D27），无需改动

## 验证

- 两个脚本各自产出双平台可执行文件且骨架版可运行

## Comments

- 2026-09-11 完成。**位置调整（用户指定）**：脚本放工作区根 `build-windows.bat` / `build-linux.sh`，不放 tools/；D27 与 M6 展开已同步改写，白名单改为放行根目录两脚本。
- bat 参考旧 `SSHFleet_Go_build.bat`：UTF-8 带 BOM + CRLF + `chcp 65001`（无 BOM 时 cmd 按 GBK 解析中文会碎行，已踩过并修复）、失败 pause、结尾 dir 产物。
- 验证：双脚本各自产出 windows/amd64（PE32+）与 linux/amd64（ELF x86-64）双产物并落工作区根 build\，交叉编译通过。