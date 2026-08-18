# ADR-0001 — read_nodes_infos 函数级拆分

- 日期：2026-08-17
- 状态：Accepted

## 背景

`modules/SSHFleet_Py/src/input/csv.py` 的 `read_nodes_infos`（约 320 行）单函数承载"读 CSV → 凭据校验 → 逐节点解析"主干流程，一路顺下来，函数过长导致阅读不便。经访谈确认：拆分的动机是**结构清晰、便于阅读**，而非行数本身。

## 决策

1. **不拆文件**，仅函数级拆分（按功能提取模块级私有函数）；`csv.py` 保持单一文件。
2. **`validate_csv_credentials`（约 200 行）不拆**——结构单一（一个循环 + 分状态检查）、无明确拆分点，属"大但好读"。
3. **拆分标准为结构清晰**，不设行数目标。
4. **单节点循环体提取为 `_parse_node`**，返回 `(node_info, error_msg)` 二选一，由主干分发；不引入异常流。
5. **端口/用户名/密码三段字段补全分别提取** `_resolve_port` / `_resolve_user` / `_resolve_password`；**不抽象通用字段函数**——三段差异真实存在（端口校验整数、密码用 getpass 且敏感），硬抽象会参数爆炸。
6. **跨节点"输入记忆"状态打包为 `FieldMemory` dataclass**（port/user/password 各一组 use_input + input_value），随循环传入并原地更新。
7. **读 CSV 阶段（inline/文件读行、判空、去表头）提取为 `_read_csv_rows`**。
8. **凭据准备阶段（全局 passphrase、universal 密钥/口令交互）留在主干**。

## 后果

- 主干收缩为：初始化 → 读行 → 凭据准备 → 循环调 `_parse_node` → 收尾。
- 行为零变化：对外接口 `read_nodes_infos` 签名与返回不变；错误消息格式、交互提示文案、getpass 延迟导入、KeyboardInterrupt/EOFError 处理均保留。
- 原 IP 重复检查（`debug=True` 恒关闭的死代码分支）随重构移除，以注释标注。
