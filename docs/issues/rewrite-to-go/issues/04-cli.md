# M1-04 internal/cli：平级选项 + 手写互斥 + 自撰提示

Type: task
Status: resolved
Resolved: 2026-09-11
Blocked by: 01

## 范围

`internal/cli`：

- spf13/pflag；保持平级选项、**不引子命令**（D24），不引 cobra（D26）
- 选项全集以旧 argparse 定义为基准（到 `modules/SSHFleet_bak/` 按需查阅），行为差异仅一处：
  - `-k` 三态用 `NoOptDefVal` 表达，去掉 `nargs='?' + const='no_value'` 魔法字符串
- 四种执行模式 + 工具模式的互斥**手写校验**，提示文案自撰（比框架原生报错可读，D24）
- `check_arguments` 的 23 条校验保持原结构，不表格化；校验错误统一由 `main.go` 打印
- 配置字段与 CLI 选项的衔接（默认值、`-f` 内联清单等）属 M2+，本工单只交付解析与校验

## 验证

- 各选项组合的接受 / 拒绝结果与旧版一致（提示文案可不同——自撰）
- `-k` 三种形态（不写 / 裸写 / 带路径）解析结果正确

## Comments

- 2026-09-11 完成。三处与计划不同，均已裁定/留痕：
  1. **Q1（-k 缺陷）**：旧 check_arguments 把裸 -k 的 'no_value' 当路径必报错，default 态走不到。用户裁定由实施者决定、要求三态准确 → 仅 universal 态校验文件存在（D40 落 spec）。
  2. **NoOptDefVal 不可用**：pflag 的 NoOptDefVal 恒优先于消费下一参数，会吞掉 `-k <路径>` 空格形式（与 argparse nargs='?' 不符）。改为解析前把裸 -k 预处理为哨兵 `-k=default`，哨兵只在 KeyMode() 一处解读；spec 依赖清单备注已修正。
  3. **帮助与无参退出顺序**：旧版「无参数→帮助」发生在配置加载之后、默认值从配置插值，据此把 Usage 改为接收 cfg，-h 与无参数统一走 Parse → ErrHelp。
- 帮助对齐用 lipgloss.Width（全角标点不再错位，对位旧 East Asian Width 手写实现）。
- 冒烟验证：互斥报错、-m/-t/-T/-n 校验、-p 消歧义约束、-f 内联 IPv4 预检（D13）、裸 -k 过检 + `-k 坏路径` 报错，全部符合预期。