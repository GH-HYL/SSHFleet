# M4-17 dangercheck：解析层 + 判定层 + 规则文件 TOML

Type: task
Status: resolved
Resolved: 2026-09-14
Blocked by: 16

## 范围

`internal/dangercheck`（对位旧 `src/check/shell_tokenize.py` + `dangerous.py`）：

- **解析层**换 `mvdan.cc/sh/v3/syntax`（spec D25）：语法树优先，解析失败（脚本有语法错）回退逐行（D33，旧版一律逐行，续行符/heredoc 会被割裂）
- **判定层**自己实现（不依赖库）：包装命令剥离（sudo/time/nohup/setsid/stdbuf/xargs/env/command + 带值选项）、rm 旗标归一（-rf/-r -f/-R/--recursive → 固定次序）、目标路径归一（合并斜杠、去尾斜杠、词法解析 . 与 ..）、间接执行递归（shell -c / eval / ssh 远端，深度上限 5）、命令替换（`$(...)` 与反引号）内部命令单独成段
- **命中聚合**：多命中全列、按风险降序（D34，旧版只报最高一条）
- **规则文件**：`config/dangerous_keywords.toml`（38 条，正则与语义自旧 YAML 一字未改，D11）；加载即校验（空/不足 10 条/字段缺失/risk_level 非法/正则编译失败/未知字段 → 指名报错，对位旧 `check_dangerous_dict`）
- **入口** `Check(args, rules) (*Report, error)` 为纯分析（无打印/交互/退出）；处置在 main：forbidden → 打印警告框后退出 1；非 forbidden → 交互确认，非交互模式放行但写工具日志留痕（D35）

## 验证

- 语料 81 例 **81/81（100%）**——与旧实现识别率完全一致（详见 19-corpus）
- 规则校验 5 类拦截 + 正常文件通过 + 未知字段报错（常驻测试）
- 二进制冒烟：`rm -rf /` → 禁止框 + 退出 1；`rm -rf /opt/x` 非交互 → 放行且日志留痕

## Comments

- 2026-09-14 完成。初版 79/81，两例失败均为命令替换内部命令未收集——补 `CmdSubst` 递归后 100%。
