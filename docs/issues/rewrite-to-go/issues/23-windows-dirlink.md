# M6-23 Windows 上的 latest_history 等价方案：目录联接

Type: task
Status: resolved
Resolved: 2026-09-14
Blocked by: 22

## 范围

D47 留的待试项：给 `latest_history` 找一个 Windows 上「能当目录用」的等价物（旧版直接跳过）。

**结论：目录联接（junction）**，原生 `DeviceIoControl(FSCTL_SET_REPARSE_POINT)` 实现：

- `output/link_windows.go`：`os.Mkdir` 出空目录 → `CreateFile(FILE_FLAG_BACKUP_SEMANTICS|FILE_FLAG_OPEN_REPARSE_POINT)` → 填
  `REPARSE_DATA_BUFFER`（`ReparseTag = IO_REPARSE_TAG_MOUNT_POINT`、替代名 `\??\<绝对路径>`、显示名普通路径）→ `DeviceIoControl`
- `output/link_unix.go`：POSIX 维持 `os.Symlink`
- `archive.go`：删掉 Windows 直接跳过的分支；链接识别改用 `os.Readlink`

**为什么不选另外两条路**：

| 方案 | 否决理由 |
| --- | --- |
| 符号链接（`os.Symlink`） | 需要管理员权限或开发者模式（`SeCreateSymbolicLinkPrivilege`）——正是旧版做不成的根因 |
| `.lnk` 快捷方式 | 只有资源管理器认；命令行 `cd latest_history` 不通，不算等价物 |

## 验证

- 临时测试 4 例（跑完已删）：① 首次创建 + 穿透读文件；② 出现更晚归档目录时链接被替换；③ 只删链接、三个目标目录内容均未受损；④ 被同名普通文件占位时告警跳过、不报错、不动文件
- 真实二进制冒烟：跑一轮命令模式后，`latest_history/` 可直接列目录、可读 `report.txt`
- **外部独立确认**（Python 读原始 reparse tag）：`st_reparse_tag = 0xA0000003` = `IO_REPARSE_TAG_MOUNT_POINT`，确认真联接而非符号链接；`os.readlink` 指向正确归档目录；`os.listdir` 可穿透
- 记录事实：本环境下 `os.Lstat` 对 junction 报 `ModeIrregular`、`IsDir() = false`；Python `os.path.islink` 同样返回 `False`

## Comments

- 2026-09-14 完成。`cmd.exe` 在本环境被沙箱禁用（bash 与另一个命令行工具两侧都拦），故不走 `mklink /J`，改原生 API——顺带避免了子进程、控制台闪窗与中文/空格路径的引号问题。限制：目标须在本地 NTFS 卷。
