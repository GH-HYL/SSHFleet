# M5-21 output：执行日志 / output.txt / report.txt / 两个 xlsx

Type: task
Status: resolved
Resolved: 2026-09-14
Blocked by: 20

## 范围

- **执行日志**（`<归档目录>/<paths.exec>`）：本次运行的节点级明细（与终端明细同形态），危险命令放行留痕也写这里（spec D46）
- **output.txt**：各模式的结果明细（`<归档目录>/<paths.output>`）
- **report.txt**：统计报告；**补齐下载模式的执行参数段**（spec D37，旧完全没有下载分支）
- **output.xlsx**（开关 `enable.output_to_xlsx`）：3 列（IP地址 / 事件类型 / 内容详情），一条结果展开为「连接 / 执行(上传|下载) / 分类」若干行；表头样式 + 列宽 + 自动筛选
- **results.xlsx**（开关 `enable.results_to_xlsx`）：结果逐条一行，含退出码、耗时、传输计数、分类、错误、输出
- 库：`github.com/xuri/excelize/v2`（spec 依赖清单已定，对位旧 openpyxl）

## 验证

- 真机冒烟（命令模式 + 上传模式）：归档目录内 5 类产物齐全；`output.txt` / 执行日志内容与终端一致；`report.txt` 参数段两种模式都对（含下载模式的空值保护）；两个 xlsx 生成成功（命令模式 6.5KB / 上传模式同）
- 生成失败只告警不中断（对位旧行为：xlsx 失败不影响执行结果）

## Comments

- 2026-09-14 完成。
