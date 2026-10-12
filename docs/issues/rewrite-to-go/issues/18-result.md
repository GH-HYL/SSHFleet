# M4-18 result：统计 + 错误分类

Type: task
Status: resolved
Resolved: 2026-09-14
Blocked by: 17

## 范围

`internal/result`（对位旧 `src/output/statistics.py` + `src/gotogo/classifier.py`）：

- **错误分类**：退出码三态（0 = 成功；非 0 = 执行失败(退出码N)，退出码即权威、不再匹配关键词；nil = 未执行，靠关键词推断）；传输部分成功优先；关键词自上而下首个命中；未命中回退报错原文（截断 200 字符）；error/output 均空 → 错误未分类；`isFallbackCategory` 对位
- **关键词文件**：`config/error_keywords.toml`（25 分类 / 91 关键词，顺序即优先级，一字未改）
- **AuthFailure 消费（spec D44）**：认证失败分类字段优先于关键词推断
- **统计**：成功/失败计数、结果数与节点数一致性校验、失败分类按数量倒序、按分类收集 IP（数值序排序）、成功分类按模式（执行成功 / 传输成功）、全局起止与耗时

## 验证

- 错误分类语料 24 例 **24/24**（详见 19-corpus）+ 配置归类断言（permission denied 归属）+ 兜底判定 6 例 + AuthFailure 优先级
- 二进制冒烟：正常命令执行到统计（`echo` 用例进度 1/1 成功）

## Comments

- 2026-09-14 完成。统计的终端呈现与报告/归档仍归 M5（`output.Render` 暂为占位）。
