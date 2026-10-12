# M2-10 nodelist：CSV 读取 + 凭据预检 + 字段补全 + 输入记忆

Type: task
Status: resolved
Resolved: 2026-09-11
Blocked by: 08

## 范围

`internal/nodelist`：

- CSV 读取：`encoding/csv` 必设 `FieldsPerRecord = -1`（变长行容忍）；跳空行 / `#` 注释行；短行补空到 6 列
- 表头识别（D13）：首行首列严格 IPv4（`netip.ParseAddr` + Is4）失败**且该行含逗号** → 判表头移除；否则报错。节点 IP 校验同样严格 IPv4
- 凭据预检（受 -k 三态控制，对位旧 validate_csv_credentials）：错误汇总输出后退出；同时**解码凭据**（读→校验→直接用），解码值进入节点数据，不再二次读盘
- 字段补全三套函数不合并（spec 明确不动）：port/user/password 各自「CSV > config > 输入记忆 > 交互输入」；密码段插入「密钥认证」档；三组输入记忆独立
- 状态3（-k 路径）：私钥读一次、口令问一次；`--disinteractive` 下状态3 明确报错（照旧）
- 全局口令（状态2）读一次、多节点共用
- **交互器落 `internal/common`**（注入 In/Out + 非交互标志）：nodelist 与 confirm 共用；非交互 Confirm 显式返回确认；取消（EOF/Ctrl+C）打印对应文案并经 main 以 1 退出

## 验证

- 冒烟：合法 CSV / 表头行 / 内联节点 / 变长行 / 坏端口 / 凭据错配矩阵抽样

## Comments

- 2026-09-11 完成。两处按用户裁定改写旧行为（均落 spec）：**D41** off 态强制空密钥（旧代码 off 态仍读清单第 5/6 列且绕过 PEM 预检）；**D42** 私钥与口令成对绑定三来源，拆掉旧版「清单口令优先、配置口令兜底」的混搭。
- 交互器落 `internal/common`（nodelist 与 confirm 共用，满足准入标准）；非终端密码输入降级为普通读取（对位 getpass 降级）。
- 临时测试 10 例全过（跑完已删）：表头移除、变长行补空、内联清单、坏端口、域名 IP 报错（D13）、off 态忽略私钥列、default 态私钥缺失报错、等级错配矩阵（level1×base64 / level2×明文 / level2×base64 解码）、凭据文件缺失。
- 待回复项记录在 `open-questions.md`：Q4（密钥节点是否套用配置默认密码——暂按密钥优先置空）、Q5（execution.mode 是否给默认值）、Q6（关键词文件检查与参数校验的顺序）。

