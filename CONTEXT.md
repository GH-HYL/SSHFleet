# SSHFleet Domain Context

## Data Flow

```
Go SSE → Python parser → _format_result() → output.txt (实时写入，人看的日志)
                         final_results ───→ output.xlsx (结构化生成，3列格式)
                         final_results ───→ results.xlsx (全字段格式)
                         final_results ───→ report.txt (统计报告)
```

## Glossary

| Term | Definition |
|------|------------|
| **执行模式 (Execution Mode)** | 两种运行模式：`command`（命令模式，`-c`/`-s` 参数）执行远程命令或脚本；`upload`（上传模式，`-u` 参数）上传本地文件到远程节点 |
| **output.txt** | 终端输出捕获文件，实时写入每个节点的执行/上传结果。文件名由配置项 `paths.files.output` 指定，存放于 `exec_log_dir` |
| **exec_log_dir** | 每次执行的历史记录目录，存放 output.txt、report.txt、results.xlsx 等产物 |
| **SSE stream** | Go 进程通过 HTTP Server-Sent Events 推送的消息流。四种消息类型：`result`（节点结果）、`progress`（上传进度）、`init`（上传初始化）、`done`（任务完成） |
| **_format_result()** | Python 端格式化单条结果的函数，根据执行模式自动适配显示"上传"或"执行" |
| **ExecResult / UploadResult** | Go 端定义的结构体，`output` 字段为 base64 编码的执行输出 |
| **错误分类 (Error Classification)** | 根据 `error_keywords.json` 中的关键词对失败结果进行分类，结果写入 output.txt 和 report.txt |
| **nodesinfo** | 节点信息列表，从 CSV 文件读取，包含 ip、port、user、password |
| **上传并发阈值** | 根据文件大小自动约束并发数：< 1MB 不限制，> 10MB 串行，中间最多 10 并发。未指定 `-n` 时自动约束并提示确认；显式指定 `-n` 时仅提示不强制 |
