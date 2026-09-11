# M1-03 internal/log：zap 日志 + SUCCESS 级别

Type: task
Status: resolved
Resolved: 2026-09-11
Blocked by: 01

## 范围

`internal/log`：

- zap 初始化，含自定义 SUCCESS 级别
- 落盘位置 / 初始化时机的具体行为以旧代码为准（到 `modules/SSHFleet_bak/` 按需查阅）
- 归档目录内两个日志文件按用途命名（tool / exec）属 M5（D36），不在本工单

## 验证

- SUCCESS 级别可输出
- 初始化失败按「致命」处理，走 main 统一出口

## Comments

- 2026-09-11 完成。自实现 fileCore（zap 自定义级别整型放不进 INFO 与 WARN 之间，故自管级别序 DEBUG/INFO/SUCCESS/WARNING/ERROR），格式与旧版逐字对齐 `YYYY-MM-DD HH:mm:ss.SSS - [ LEVEL ] - message`，SUCCESS 居中 7 列；lumberjack 50MB 轮转。冒烟验证：日志落 historys/SSHFleetTools.log，格式比对一致。