# ADR-0005: 用 `BannerCallback` 接住服务端提示，作为认证阶段故障的原因来源

- **状态**：已接受
- **日期**：2026-09-16

## 背景

账号过期（`chage -E`）、`/etc/nologin` 存在这两类故障，拒绝发生在**认证阶段**（PAM account 预认证）。sshd 在这里**刻意不发原因**——服务端日志写的是

```
pam_unix(sshd:account): account sftest2 has expired (account expired)
Failed password for sftest2 from 172.28.112.1 port 2096 ssh2
fatal: Access denied for user sftest2 by PAM account configuration [preauth]
```

统一话术 + `[preauth]` 是防账号探测的设计。客户端侧只能拿到 `ssh: handshake failed: EOF`，于是被 `error_keywords.toml` 里的 `ssh: handshake failed: eof` 判成「SSH协议错误」——一个会把人引去查网络与协议版本的**错误结论**。

但服务端其实留了一个口子：**sshd 把 PAM 的话当作 `SSH_MSG_USERAUTH_BANNER` 发出来**。`x/crypto/ssh` 会解析这个消息，而 `internal/ssh/client.go` 的 `ClientConfig` 没有配 `BannerCallback`，`handleBannerResponse` 在回调为空时**直接丢弃**整包。也就是说原因一直在包里，只是没接。

2026-09-16 用 Go 客户端挂上 `BannerCallback` 实测：

| 场景 | 连接结果 | banner 原文 |
| --- | --- | --- |
| 密码过期（密码登录） | 成功 | `You are required to change your password immediately (administrator enforced).` |
| 密码过期（密钥登录） | 成功 | 同上 |
| 账号过期 | 失败（EOF） | `Your account has expired; please contact your system administrator.` |
| `/etc/nologin` 存在 | 失败（EOF） | **该文件的内容原文** |
| 密码真错 | 失败 | 空 |

## 决定

`ClientConfig` 挂上 `BannerCallback`，把服务端提示原文**追加进 `Result.Error`**（用户 2026-09-16 裁定）—— 不新增字段，原有文本处理逻辑一行不改。

- `error` 为空时直接作为其内容；非空时换行追加到尾部，原有报错原文保留在前。
- 于是它天然进入现有分类链路：第一块匹配 `error` 时即可命中「账号过期」等判据；传输模式（连接成功、SFTP 失败）也靠它把原因带上来。

## 理由

- 这是客户端拿到认证阶段原因的**唯一正规机制**：不 fork 第三方库、不解析裸包、不加探测命令。
- **对真认证失败无副作用**：密码错误时服务端无话可说，banner 为空（实测），不会污染原有分类。
- banner 在失败之前就已到达（实测账号过期时 banner 与 EOF 同时拿到），失败路径上不会错过。
- 与模式无关：传输模式（上传/下载）同样受益——实测密码过期时连接是成功的、banner 已到手，而 SFTP 报错原文里没有任何原因线索。
- 并入 `error` 而非另立字段：分类逻辑、统计口径、明细列都不用动，改动面最小。

## 影响

- `error` 字段自此同时承载「报错原文」与「服务端提示」，属**用户可见变化**（`CHANGELOG` 记「问题修复」）。
- 服务端提示可能是 MOTD、法务声明这类无害内容（sshd 配了 `Banner` 的站点，每行都带一段公告）：分类不受影响（退出码为 `0` 时退出码优先），但明细的 `error` 列会出现这段文本。
- 关键词表需新增认证阶段的判据文案（如 `Your account has expired`、`You are required to change your password immediately`），并**取"最长整句"**——精度全部来自关键词长度。
- 账号过期与 `/etc/nologin` 从"物理上无法分类"变为可分类（`/etc/nologin` 的提示是文件内容、不可预判，落回原文）；`error_keywords.toml` 里的 `ssh: handshake failed: eof` 应当摘掉（EOF 是万金油症状，不是原因）。
- 已有 `AuthFailure`（D44）是"SSH 层直接给短分类文案"的既有先例；本决策走的是"原文入结果 + 关键词外置"，两者不冲突。

## 备选方案

| 方案 | 为何未采用 |
| --- | --- |
| 不处理，继续标「SSH协议错误」 | 是错误结论，会把排查方向引偏；而原因其实就在被丢弃的包里 |
| 新增独立字段 + 明细加一列 | 表现力更好，但要把新字段一路串到统计、终端、报告与 xlsx；用户 2026-09-16 裁定并入 `error` 即可 |
| 只在关键词未命中时把 `eof` 换成"握手被断开"这类中性词 | 只改措辞、不拿信息；banner 一路本来能拿到具体原因 |
| 登录后探测（读远端 `shadow`、`chage -l`、日志） | 认证阶段就被拒的账号根本登不进去；且与"最小侵入"的取向冲突 |
| 自己 fork / patch `x/crypto/ssh` 以暴露 banner | 库本来就提供了 `BannerCallback`，无需 fork |
