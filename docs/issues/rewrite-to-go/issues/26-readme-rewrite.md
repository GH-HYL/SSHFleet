# M6-26 README 重写（面向全新安装）

Type: task
Status: resolved
Resolved: 2026-09-14
Blocked by: 25

## 范围

仓库根 `README.md`（此前不存在）。

用户 2026-09-14 裁定：**面向全新安装写一份，不考虑升级/旧版本内容**——重构带来的变化由 `CHANGELOG.md` 承担，README 不写迁移章节。

- 结构与旧版 README 对齐（九章 + 附录）：① 这是什么 ② 安装 ③ 快速上手 ④ 核心概念 ⑤ 四种模式与参数 ⑥ 进阶用法 ⑦ 结果与历史 ⑧ 技术架构 ⑨ FAQ
- 内容全部按 5.0.0 的实现重写，事实来源：`cli/args.go` 的 `Usage`（选项与提示文案）、`config/SSHFleet.conf`（配置全解）、spec 决策表（行为差异）
- 覆盖的新差异：单可执行文件（不再需要 Python 与引擎程序）、配置改 TOML 且**全字段必填**、规则文件 TOML、`-k` 三态与私钥/口令同源、上传远端目录须已存在（工具不建目录）、目录上传按文件名平铺、与密码均失败的新分类、常见退出码提示、归档里执行日志名为 `SSHFleetExec.log`、`assets/` 不含上传文件、`latest_history` 两平台可用
- FAQ 按新版重写：删掉「找不到 Go 引擎」这类已不存在的问题，补上「配置缺字段/取值非法」「提示配置文件缺失（工作目录不对）」「缺少主密钥（等级 3 首次使用要先 --gen-key）」「上传报远程目标路径不存在」「目录上传后子目录不见了」

## 验证

- 帮助文本与 README 的参数表逐项比对一致（选项名、默认值来源、说明）
- 配置全解段落与随包 `config/SSHFleet.conf` 逐项核对
- 发布包里的 README 与仓库根同一份（打包时复制）
- 示例命令均为可直接执行的形态（`SSHFleet -f nodes.csv -c "uptime"` 等），并注明 Windows 下写 `SSHFleet.exe`

## Comments

- 2026-09-14 完成。发布包里的 `config/SSHFleet.conf` 按用户裁定**沿用现有那份**（含作者环境的默认值），故 README 的「安装」一节特意提示：配置里 `password_security = 3` 时首次使用前要先 `--gen-key`。
- 同一段还强调两个「必须」：配置必须与可执行文件同目录（工具从当前工作目录读 `./config/SSHFleet.conf`）、所有字段必须写出。
