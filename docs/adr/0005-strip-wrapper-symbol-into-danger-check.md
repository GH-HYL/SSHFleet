# ADR-0005: 命令边界符号剥壳下沉至危险检测

状态：已接受（2026-08-18）

## 背景

`remove_command_fist_last_same_symbol` 原实现位于 `src/command/builder.py`，在 main 流程中于危险检测之前调用，剥除命令首尾相同的包裹符号（引号、反引号等），并修改 `args.c`；被剥符号通过 `remove_symbol` 参数传给确认阶段展示提示「系统检测并移除了命令的边界符号」。

该设计存在三个问题：

1. **职责错位**：命令构建模块承担了"命令清洗"职责，但剥壳的真实消费者是危险检测——锚定规则（`^` 开头）对带引号包裹的命令（如 `'rm -rf /'`）会漏检，剥壳后才能命中；而 bash 执行时引号被 shell 消费，漏检即危险命令真实执行。
2. **修改执行命令**：main 中剥壳直接改写 `args.c`，最终执行用的也是剥壳后命令，命令构建与危险检测耦合。
3. **展示冗余**：`remove_symbol` 提示随确认界面展示，属于历史遗留的界面信息。

另外，原逻辑只对命令行 `-c` 剥壳，脚本模式（`-s`）的逐行检测不剥壳，脚本内引号包裹的危险命令同样漏检。

## 决策

**剥壳逻辑下沉至危险检测模块，只服务于检测；main 与确认阶段的关联全部删除。**

### 1. 函数迁移

- `remove_command_first_last_same_symbol`（顺带修正 `fist` → `first` 拼写）迁移至 `src/check/dangerous.py`，与 `_split_by_separators`、`_strip_command_prefixes` 组成检测前清洗链。
- `src/command/builder.py` 删除原函数；`src/command/__init__.py` 导出列表同步移除。

### 2. 检测流程调整

`check_dangerous_patterns` 预处理循环新增"第零步"：对每一行（命令 `-c` 与脚本 `-s` 统一处理）先剥壳，再按分隔符切分、去前缀。由此：

- 命令行引号包裹 → 命中（行为与原设计一致，防绕过能力保留）
- **脚本行引号包裹 → 也能命中（新增覆盖，原逻辑漏检）**

### 3. 删除展示链路

- `sshfleet.py`：删除 `remove_symbol` 变量与剥壳调用、`arguments_confirm` 的 `remove_symbol` 实参。
- `src/input/confirm.py`：`arguments_confirm` 签名删除 `remove_symbol` 参数，删除「系统检测并移除了命令的边界符号」提示块。

## 后果

- **正面**：职责归位（清洗属于检测侧）；`args.c` 不再被预处理改写，命令构建与执行链路保持原样；脚本模式引号包裹漏检被修复；确认界面少一行冗余提示。
- **负面**：无功能回退；剥壳能力从"主流程共享"变为"检测专用"，不影响其他环节（其他环节本就不需要剥壳）。
- **改动面**：`dangerous.py`（+函数 +1 行调用）、`builder.py`（-函数）、`command/__init__.py`、`sshfleet.py`、`confirm.py`、CHANGELOG。
