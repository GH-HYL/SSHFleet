# M2-09 credential：--convert-password

Type: task
Status: resolved
Resolved: 2026-09-11
Blocked by: 08

## 范围

`internal/credential` 转换部分：

- 路径解析**删除旧版两段式**（先原样找、找不到再拼 secret_dir），收敛为 D31 单规则：去空白 → ~ 展开 → 绝对原样 → 相对拼 secret_dir；`secret_dir` 未配置 → 明确报错
- 已裁定的两处行为变化（spec D31 展开）：未配 secret_dir 且文件在当前目录 → 旧命中新报错；两处同名 → 新版一律去 secret_dir
- 转换矩阵（明文/base64/加密 × 1/2/3）、空内容拒绝、解密失败提示、写后回读校验、只输出摘要不回显内容——全部照旧
- 目标等级 3 用新 0x02 加密；输入 0x01 密文可解（只读兼容）
- 读盘防御（NUL=二进制、多行非 base64/加密格）照旧

## 验证

- 冒烟：临时凭据文件 × SSHFLEET_KEY（进程环境变量，不碰注册表），明文→base64→加密→解密还原逐级验证

## Comments

- 2026-09-11 完成。二进制端到端冒烟通过：明文 →（等级3）0x02 加密 → 重复执行判「已是加密格式」→（等级1）还原为明文，密码 `Y>j^nmcjz` 逐字还原；D31 路径规则（相对拼 secret_dir / 绝对直用 / secret_dir 未配置报两条出路 / 找不到文件）、缺主密钥提示均验证。
