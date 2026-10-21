# 06: medium 族重写

Status: done
Blocked by: 01, 02, 03

**要做的**：把 medium 档从 13 条收成 6 条。做完之后，会「让这一批机器同时变样」的动作照样提醒一次，而日常的包管理、改属主、调内核参数不再每次都弹。

---

## 这一档的定位

medium 与 high 的处置**完全一样**（打印警告 → 问一次 → `--yes` 不阻断），区别只在措辞与颜色。所以这一档的取舍标准不是「要不要拦」，而是**「值不值得占用使用者一次注意力」**。

判据：**批量下发后，会不会让一整批机器同时变样、且不易回滚？**

## 六条规则

### 1. 删除关键路径文件

**四合一**：原「删除日志文件」+「删除系统文件」+「强制删除配置或日志文件」+「强制删除」。

只认关键路径：`/etc`、`/var/log`、`/boot` 之下的删除。必须覆盖 `rm -f /etc/hosts`、`rm -f /var/log/app.log`、`rm -rf /etc/nginx`、`rm -f /boot/*`。

**不认**：`rm -f /tmp/x`、`rm -rf /tmp/build_cache`、`rm -f ./a.txt`。

> **为什么原「强制删除」要并进来而不是单独留**：它的正则 `\brm -f\s` 太宽——批量清 `/tmp` 也在里面，是纯噪音。并入后靠「只认关键路径」收窄。

### 2. 停止·禁用服务（**名单要重写**）

**二合一**：原「强制停止服务」+「禁用系统服务」。

**现名单是反的**：正则只认 `(?:mysql|nginx|apache)`——列上了业务服务，**漏掉了会切断自己管理通道的 `sshd` / `network`**（`audit.md` 8.2 实测：`systemctl stop sshd` 不响，`systemctl stop mysql` 反而响）。

新名单以「停掉后可能再也连不上这批机器」为准：`sshd`、`network`、`NetworkManager`、`systemd-networkd`、`firewalld`。

必须覆盖：`systemctl stop sshd`、`systemctl disable sshd`、`systemctl stop network`、`systemctl disable firewalld`、**`systemctl mask firewalld`**。

**不认**：`systemctl stop nginx`、`systemctl restart nginx`（业务服务的启停是日常）。

> **与 05 的边界（必须对齐）**：`firewalld` 的 `mask` 与 `disable` **都在本条**（medium）——两者是同一件事的两种强度，不能一个 forbidden 一个 medium。`sshd` / `network` 则按严重程度分：`mask` 归 05（high），`stop` / `disable` 归本条（medium）。

### 3. 删除账户·用户组

**二合一**：原「删除用户账户」+「删除用户组」。覆盖 `userdel`、`userdel -r`、`groupdel`。

### 4. 权限修改

原「权限修改」，保留。认 **others 可写**（`chmod 777`、`chmod -R 777`、`chmod o+w`）。

> **注意**：现正则 `\bchmod\s+[0-7]?[0-7][0-7]7\b` **不接受中间夹 `-R`**——`chmod -R 777 /var/www` 实测不响（`audit.md` 8.4）。重写时把它吃进来。
>
> **不要放宽到 755**：`chmod 755` 是每日例行，放宽就成了最大的噪音源。

### 5. SUID·SGID

**二合一**：原「设置SUID权限」+「设置SGID权限」。覆盖 `chmod u+s`、`chmod g+s`、`chmod 4755`、`chmod 2755`。

### 6. 清空系统日志

原「清空系统日志」。覆盖 `truncate -s 0 /var/log/messages`、`> /var/log/messages`。

## 验收

- [x] 第 1 条：4 种关键路径写法命中；`/tmp` 下三种**不命中**
- [x] 第 2 条：5 种写法命中（**含 `systemctl mask firewalld`**）；**`systemctl stop nginx` 不命中**（这是原规则误伤、新规则必须放掉的）
- [x] 第 3 条：3 种写法命中
- [x] 第 4 条：`chmod 777`、**`chmod -R 777`**、`chmod o+w` 命中；**`chmod 755`、`chmod 775` 不命中**
- [x] 第 5 条：4 种写法命中（含数字形式）
- [x] 第 6 条：2 种写法命中
- [x] `medium` 档共 **7 条**（2026-10-20 按开发者三点五调整：原判 6 条，`强制删包` 由 high 降入）
- [x] **`firewalld` 的 `mask` 与 `disable` 落在同一档**（都是 medium），不会一个 forbidden 一个 medium
- [x] 语料测试通过；**不通过的用例先分辨是规则错还是期望错**，拿不准的交开发者定，不许改期望凑绿

## Comments

- 2026-10-19：建立。第 2 条的名单反转是实测发现的真缺陷（`audit.md` 8.2），重写时必须顺手修正。
- 2026-10-19：**已落地**。关键路径文件只认 `/etc/`、`/var/log/`、`/boot/` 之下的删除；`/etc/nginx`、`/var/log/app.log` 归本条，`/tmp` 下不认。`firewalld` 的 `mask` 与 `disable` 同在本条，`sshd`/`network` 的 `mask` 在 05。`chmod -R 777`、`o+w`、`a+rwx` 与数字形式都覆盖，755/775 不认。
- 2026-10-20：**验收逐条复核通过**。关键路径 4 种命中、`/tmp` 三种不命中；`systemctl mask firewalld` 与 `disable firewalld` 同落 medium，`stop nginx` 不命中；`chmod 777`/`-R 777`/`o+w` 命中而 `755`/`775` 不命中；账户删除 3 种、SUID 4 种、日志 2 种均命中。条数按三点五调整为 7 条。
