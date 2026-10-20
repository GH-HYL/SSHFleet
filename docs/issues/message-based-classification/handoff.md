# 报文分类议题 · 交接说明

> **这份文件给接手的人看**：上一个会话做完了 01–03，这里说清楚「到哪了、下一步怎么做、哪些坑踩过」。
> 议题本身的完整背景读 `README.md`；落点与清单读 `audit.md`；决策理由读 `docs/adr/0021-message-based-classification.md`。
> **本文件在 07 收尾时连同 `README.md` 一起删掉。**

## 一、一句话现状

判定层已经**不再给分类名**了——11 个中文分类常量全部退役，失败原因一律变成 `sshfleet.<环境>.<环节>: <英文>` 报文，经判据表认领。**02 这个最重的框架刀已经过去**，剩下 04–07 是照格式收口 + 核查 + 验收。

## 二、已完成（01–03）

| 工单 | 提交 | 做了什么 |
| --- | --- | --- |
| 01 摸底 | `ec4969a` | 产出 `audit.md`：33 处自编中文、11 个分类常量逐个拆解、23 条关键词逐条对应、出厂报文核准到 36 条 |
| 02 框架 | `45ecaf1` `d54a43d` `1dbe403` | 报文格式与四个导出纯函数；判定层删常量、只留成功规则；`code_defined` 全线下线；判据表改挂报文；执行侧七摊改报文 |
| 03 会话/执行 | `496878c` | 新增 `run.rejected`；两处超时收成同一个环节名 |

**当前测试与构建**：`go build ./...`、`go vet ./...`、`go test ./...` 全绿（13 个包）。分类语料 **42 条全过**。

## 三、下一步：04 传输

打开 `issues/04-transfer-messages.md`。要点先说：

- **配置在 02 已经改好了**（`SFTP失败` 8 条、`本地文件错误` 4 条、`远程路径不存在`、`远端没有文件` 2 条、`远程文件已存在`、`sudo失败` 都改挂了报文）。04 不需要再改关键词，只要**核实逐条对得上**。
- **`transfer.finished` 的追加逻辑 02 已经接好并真机验证过**：下载 3 个文件、1 个不可读 → 错误详情里两行并存（逐文件原文 + `sshfleet.transfer.finished: 1 of 3 files transferred`），分类「部分成功」由配置认领。这个演示可以直接复现，命令见下。
- 04 真正剩下的是：**sudo 路径**（`sftp.UploadFiles` 的 sudo 分支要 `NOPASSWD`，现在没配）与**上传侧**的真机用例。

## 四、可复制的验证命令

```bash
cd /d/Desktop/Code/SSHFleet/modules/SSHFleet_Go
export TMP='D:\Desktop\Code\SSHFleet\build\go-tmp-baseline' TEMP="$TMP"
go build ./... && go vet ./... && go test ./...
go build -o /d/Desktop/Code/SSHFleet/build/e2e-probe/sshfleet.exe .
cp config/keywords_error.conf config/keywords_passwd.conf /d/Desktop/Code/SSHFleet/build/e2e-probe/config/
```

```bash
cd /d/Desktop/Code/SSHFleet/build/e2e-probe
# 连接被拒（本机端口 1）
./sshfleet.exe -f "127.0.0.1,1,sshfleet_ok,x" -c "echo hi" --yes
# 正常连接 + 非零退出
MSYS_NO_PATHCONV=1 ./sshfleet.exe -f "172.28.118.49,22,sshfleet_ok,Probe#2026ok" -c "exit 3" --no-sudo --yes
# 部分成功（先在 WSL 里造：3 个文件、其中一个 chmod 000）
MSYS_NO_PATHCONV=1 ./sshfleet.exe -f "172.28.118.49,22,sshfleet_ok,Probe#2026ok" -d "/tmp/dl" -p ./dl3 --no-sudo --yes
# 改密（未过期账号）
MSYS_NO_PATHCONV=1 ./sshfleet.exe -f "172.28.118.49,22,sshfleet_ok,Probe#2026ok" --change-password "NewProbe#2026x" --yes
```

## 五、必须知道的三个坑

1. **`TMP` / `TEMP` 指到工作区内**。否则 `go test` 报 `Access is denied`——那是 360 删了工作区外的临时可执行文件，不是权限问题。已在上面的命令里带上。
2. **连 WSL 用 IP（`172.28.118.49`），不要用 localhost**。localhost 转发时通时断。IP 变了用 `wsl -d Ubuntu-26.04 -u root -- ip -4 addr show eth0` 查。
3. **Git Bash 会改写 `/tmp/xxx` 参数**，跑下载模式必须加 `MSYS_NO_PATHCONV=1`。

## 六、环境

- **WSL Ubuntu-26.04 长期在后台运行，不要 `wsl --shutdown`。** 重启会换 NAT 地址、打断你手上的东西。
- 账号：`sshfleet_ok` / `Probe#2026ok`（普通）、`sshfleet_expired` / `Probe#2026exp`（密码已过期）、`sshfleet_nologin` / `Probe#2026no`（nologin）。
- 配置里 `sudo = true` 是默认值，但测试账号没有 sudo 权限，**跑任何模式都要加 `--no-sudo`**，否则会得到「执行失败(退出码1)」+ `sudo: I'm sorry`。要给 sudo 场景做验收，得先给 `sshfleet_ok` 配 `NOPASSWD`。
- 你原有的 `repro` 账号与 `/etc/ssh/sshd_config.d/99-probe.conf` **没动过**，用不上就别碰。

## 七、实施中踩过的坑（省得再踩一遍）

| 现象 | 真相 |
| --- | --- |
| 「部分成功」被「权限不足」抢走 | 报文时代没有一级定性先判了，只能靠判据表**次序**——`部分成功` 必须排第一组最前 |
| `代填用尽后超时` 永远认不出来 | 它当时还挂着旧中文关键词 `已全部送出`，且排在 `触发词未命中` 之后，两处都错 |
| `run.rejected` 写了却看不到效果 | 命令通道上 `NewSession` 先失败、被 `session.create_failed` 接住；它只在代填通道 `Start` 被拒时出现 |
| 退出码 0 却判失败时显示「执行失败(退出码0)」 | 自相矛盾，已改为落第一组显示原文 |
| 一度以为 WSL 权限模型不正常 | 是我把 `2>&1` 放在 `$?` 之后，读到的是重定向的退出码。权限模型本身正常 |
| `find` 列不出 `chmod 000` 的文件？ | 列目录只需父目录读权限，不读文件内容——`find` 本来就列得出，枚举阶段不会失败 |

## 八、口径提醒（别改回去）

- **报文只陈述现象，不下结论**：用 `finished` 不用 `partial`，用 `canceled` 不用 `failed`。叫不叫「部分成功」是配置 `name` 的事。
- **判定层不许自己给分类名**。唯一例外是 `执行失败(退出码N)`。
- **报文一律追加、不许覆盖**：`AppendMessage` 是唯一入口。
- **「密码过期」不发报文**：它是远端说的话。它的入场信号与中止词按分类名查表（`keywords.go` 里那个不导出的字面量），删常量时特意留下了。

## 九、还剩什么

| 工单 | 内容 | 备注 |
| --- | --- | --- |
| 04 | 传输一摊核查 + sudo/上传真机用例 | **从这开始** |
| 05 | 改密五档核查 | 主体 02 已接好；重点是「改 name 让守卫报错」那条用例还没写 |
| 06 | 代填两条核查 | 重点复核中止词链路没被改坏 |
| 07 | 残留核对、五种模式端到端、CHANGELOG（抬主版本）、手册（`docs/1-使用手册.md` 与 `docs/4-进阶知识手册.md` 的「判定分两层」整段） | 做完删掉 `README.md` 与本文件 |

**另外有一件悬着的**：判据表里 `本地文件错误` 的 `文件不可读` 关键词对应的文本走的是 `fatal` 而不是 `Result.Error`，本就命中不了。04 要决定撤掉它、还是把采集期错误纳入报文体系（后者要扩 spec）。详见 `audit.md` §七 表下注与 §十四 第 8 条。
