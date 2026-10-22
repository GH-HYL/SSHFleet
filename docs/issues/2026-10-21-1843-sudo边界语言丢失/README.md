# 议题入口（交接用）

> **本文件是交接用的临时入口，不是长期文档。**
> 它只为一件事存在：让下一个会话不用问任何人，就能知道这是什么、从哪开始、什么算做完。
> **议题完成时删除本文件**（判据见文末「收工清单」）。不要把它写进 CHANGELOG 或手册，也不要让它变成规格的一部分。

---

## 一、这是什么

开发者报：`-a` 代填运行时脚本输出的中英混排，中文触发词匹配不上；另在外部终端看中文文件名是方块。

已经查清：**工具没丢配置**，语言丢在 `sudo` 边界（`export` 拼在 `sudo` 之前，`sudo` 默认 `env_reset`）。ADR-0024 已定：**`[env]` 出厂值的 `LANG` / `LC_ALL` 改成 `C`，拼接形态不变**。

**代码一行未动**（三份 `SSHFleet*.conf` 仍是 `en_US.UTF-8`）。

## 二、先读哪几份（按这个顺序）

| # | 文件 | 读它做什么 |
| --- | --- | --- |
| 1 | `docs/adr/0024-env穿过sudo边界.md` | 决策本体。重点：五、决定；六、理由；七、影响；十二、复审 |
| 2 | `audit-run-2026-10-22/report.md` | 2026-10-22 回顾审计的结构合并：35 条硬伤、30 条判断题、10 个冲突点、6 处空白。**只列关系，不排优先级、不判谁对** |
| 3 | `audit-run-2026-10-22/raw/*.md` | 六份原始回执。要证据时按编号查（如 `03:H5`） |
| 4 | `spec.md` | **旧规格，与 ADR 冲突**：仍写「`export` 过不了 sudo」、推荐 `sudo env`。重写 spec 时按 ADR 来，不要照它抄 |
| 5 | `findings-2026-10-21.md` | 排查底本（证据原件，被 ADR 第十一节引用） |

审计的**设计与过程**另有 `audit-run-2026-10-22/` 下的 `prep.md`（摸底）、`dispatch.md`（第一轮分发审）、`prompts-v3.md`（实际派发的提示词包），只在需要复核审计本身时才读。

## 三、做到哪了

| 阶段 | 状态 |
| --- | --- |
| ADR | **已接受**。2026-10-22 按审计修订 16 处（事实与落点），决定未改 |
| 上游 ADR | 0015 的更新记录已收窄成纯指向；0016 **新建了**「更新记录」节 |
| spec | **未重写**，旧 spec 与 ADR 逐条相反 |
| 工单 | **未拆** |
| 代码 | **未动** |
| 手册 | 未动。仓库里有 11 处反向表述（`docs/3-进阶配置手册.md:288`、`docs/4-进阶知识手册.md:40/46/47/67/76/78/82/86`、`README.md:361`、`CHANGELOG.md:134` 已封口不可回改） |

## 四、下一步（顺序固定）

1. **补两项证据**（见第五节「未决」第 1、2 条）
2. **按 ADR 重写 spec**（`/to-spec`），落点 `docs/issues/2026-10-21-1843-sudo边界语言丢失/spec.md`
3. **拆工单**（`/to-tickets`，竖切，一条一个文件）
4. **实现**（红线 1 授权、红线 5 复现）

## 五、未决（需要开发者定，不许自己拍）

1. **取值是否维持 `C`**。卡在「`LANG` 能不能穿过 sudo」的实测：本会话环境不允许换 uid（见第六节），一次真 sudo 都没跑成。你镜像上的 sudoers **没有任何 `env_keep`**，所以 `LANG` 到底穿不穿，取决于 sudo-rs 的默认行为——必须实跑。
2. **3.8 的三个百分数怎么处置**（0% / 85%(170/200) / 12 行）。本机复算得 0% / 100% / 16-of-16，且固定 200 组、只改「每组几条」，比例从 30.5% 漂到 100%——85% 编码的是一个没写出来的抽样规则。三个选项（补生成器 / 降为定性 / 删整节）见 `report.md` 与 `03:J1`。
3. **存量机群怎么交代**。ADR 第七节只说「不提供自动迁移、是否改随使用者」，没说存量怎么办；而议题最初的现象就发生在存量机器上。开发者已表态「不用考虑兼容性」，但 spec 里仍要落一句「存量不处理 / 给一条手工改法」。

## 六、环境与复现（**最容易白费一轮的地方**）

### 本会话的沙盒限制（换环境后可能不一样，先试一次）

- `/run` 只读、`/var/lib/containers` 不可写、**`setresuid` / `setgroups` 被挡**。
- 后果：`sudo` 起不来——`sudo: PERM_SUDOERS: setresuid(-1, 1, -1): Invalid argument`；`chroot --userspec` 也直接 `cannot set groups`。
- podman 5.8.6 在，但要 `--root <工作区内路径> --runroot=/tmp/xxx --tmpdir=/tmp` 才能起；直接用它自己的存储会报 `database run root ... does not match`。`docker.io` 被挡（502），`quay.io` 通。

### 开发者提供的镜像（已摊成 rootfs 用过）

镜像 = `ubuntu:26.04` + systemd + libpam + sshd + 用户 `hyl`（`hyl`/`sudo`/`admin` 组），`CMD ["/sbin/init"]`，在 `~/.local/share/containers/storage` 里（rootless 存储，11 层）。

**摊平方法**（`build/` 会按天清理，`build/env1/rootfs` 迟早消失，需要时照这个重建）：

1. 在 `overlay-images/<id>/manifest` 里找 config 的 `history`，含 `useradd ... hyl` 的那条即目标镜像（本会话是 `c3a0ef41...`）；
2. 用 `overlay/<layerdigest>/diff` 逐层 `cp -a` 到目标目录，顺序按各层 `overlay/<layerid>/lower` 链（顶层 depth 最大）；
3. 摊完 `chroot <rootfs> /bin/bash` 即可用。

### 该镜像的实测事实（来自它的 rootfs，不是猜的）

| 项 | 值 |
| --- | --- |
| 发行版 | Ubuntu 26.04.1 LTS |
| coreutils | **uutils 0.10.0**（ADR 记的是 0.8.0） |
| awk / perl | mawk 1.3.4 / perl 5.40.1 |
| python3 | **没装** |
| `locale -a` | 只有 `C` / `C.utf8` / `POSIX`（**连 `zh_CN` 都没生成**） |
| sudo | **sudo-rs**（`/etc/alternatives/sudo → /usr/bin/sudo.ws`） |
| sudoers 生效项 | 只有 `Defaults env_reset` 与 `secure_path="…:/snap/bin"`，**无 `env_keep`** |

宿主（本机）另有一套：Ubuntu 26.10、uutils 0.12.0、mawk 1.3.4、sudo-rs 0.2.14，`locale -a` 同样没有 `en_US.UTF-8`。

## 七、不能踩的坑

1. **别照 ADR 第七节只改一份 conf**。出厂值在**三份** `SSHFleet*.conf` 里；两个打包脚本把 `SSHFleet-for-{linux,windows}.conf` 改名成包内的 `config/SSHFleet.conf`，工作区那份**不随包发布**。只改它会「工作区验收通过、包内还是旧值」。
2. **别用 `go test` 验「拼接形态没变」**。现有测试在「把 `export` 挪到 `sudo` 之后」这个改动下**全绿**（`04:J1`）；要逐字断言 `BuildCommand` 的全串 + 反例禁用词。
3. **0016 原来没有「更新记录」节**（本会话已补）。A3 那句「各加一行」在此之前无处落笔。
4. **第十二节 12.2 是「未证实清单」，不是待办**。列名叫「判定条件」，不要往里塞排期。
5. **`/dev/shm` 落文件是另一件事**（`audit-run-2026-10-22/raw/07-shm-delivery.md`）。它不解决 locale，只影响投递形态；且有「可预测文件名 + 符号链接 → 覆盖目标机任意文件」的风险（已复现）。**不要顺手并进本议题。**

## 八、收工清单（议题完成时逐条打勾）

- [ ] spec 按 ADR 重写，五个待定项逐条关闭
- [ ] 工单全部交付，状态改到与实际相符
- [ ] 手册 11 处反向表述改完（含 `4-进阶知识手册.md:82` 那段「为什么钉死成 en_US.UTF-8」的选型理由）
- [ ] CHANGELOG 按档位记一条（`A1` 改动是否算不兼容，见 `05:问4`）
- [ ] **删除本 README**
