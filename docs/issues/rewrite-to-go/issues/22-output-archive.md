# M5-22 output：归档目录 / 资源备份 / latest_history

Type: task
Status: resolved
Resolved: 2026-09-14
Blocked by: 21

## 范围

- **归档目录**：`historys/<YYYY-MM-DD_HH-MM-SS>_<模式>[_备注]/`（对位旧命名；无备注时不追加下划线）
- **资源备份** `assets/`：只备份清单与脚本，**不备份上传文件**（spec D38）；内联清单（`-f` 为文本）无文件可备份时跳过
- **latest_history 软链接**：POSIX 下建软链接指向最新归档目录（重名符号链接先删；同名普通文件告警跳过）；**Windows 跳过并提示**（需开发者模式/管理员权限）——用户要求留到收尾阶段再尝试 Windows 等价方案
- 归档只放执行日志，工具日志保持 `historys/<paths.tool>` 单一滚动文件（用户 2026-09-14 裁定）

## 验证

- 真机冒烟：归档目录 `2026-09-14_08-19-22_command_echo/`（含备注 `_m5upload` 的上传用例同样正确）内含 `SSHFleetExec.log` / `assets/` / `output.txt` / `output.xlsx` / `report.txt` / `results.xlsx`
- Windows 下打印跳过提示，不报错

## Comments

- 2026-09-14 完成。Windows 软链接等价方案（如 junction 或快捷方式）列为本轮收尾待试项。
