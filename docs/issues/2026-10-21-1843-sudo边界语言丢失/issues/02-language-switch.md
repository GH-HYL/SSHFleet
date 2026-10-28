# 02: 语言开关 `[locale] english`

**要做的**：配置里打开这个开关，目标机上所有程序的提示语统一说英文——失败分类的关键词（全是英文原话）不再因为目标机出中文而整批落「未归类」；关掉则跟随目标节点的语言环境。编码部分不跟着开关走，照旧按机器判断。

**Blocked by**：01

Status: done

- [x] `[locale]` 段被解析，`english` 缺省 `false`；类型写错由既有解析报错，不额外加校验
- [x] 片段生成接缝：开、关两种产出各一条逐字断言——开比关多一句 `export LC_MESSAGES=C`，其余逐字相同
- [x] 片段不含单引号的断言在开关打开时仍成立
- [x] 不设 `LC_ALL`，不碰 `LANG`：开关打开后的下发行里两者零命中
- [x] 日志「命令行」与实发串仍是同一串
- [x] 手工一条：开关打开，非交互下发一条会说话的命令，输出是英文原话
- [x] 两平台 `go test ./...` 全绿（2026-10-28：Windows go1.26.4 与 WSL2 Ubuntu 26.04 go1.26.8 各一次）

## Comments

- 2026-10-28：按 ADR-0030 建立。开关开在 `[locale]` 而不是 `[env]`，理由见 ADR，不在工单重复。
- 2026-10-28：实施。开关进 `[locale]` 段并列入必填表；`english` 从 `Args` 一路传到 `BuildCommand` / `DownlinkSpec` / `DescribeInput`，四种下发形态共用。
- 2026-10-28：与 03 同一批落。配置解析对未知键零容忍、且字段必填，解析先加而模板没加会让程序起不来，所以 02 与 03 不能分开交付。
- 2026-10-28：开关语义在宿主上验过（同一 LANG、只多一句 LC_MESSAGES=C）：`LANG=zh_CN.UTF-8 dpkg -l nosuchpkg` 出「没有找到与 nosuchpkg 相匹配的软件包」，加上 `LC_MESSAGES=C` 后出「no packages found matching nosuchpkg」。容器镜像没有 zh locale 与翻译包，构造不出「本来出中文」的现场，所以这条在宿主上验，不在容器里。
