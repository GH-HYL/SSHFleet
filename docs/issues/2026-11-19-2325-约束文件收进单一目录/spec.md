# 约束割接收尾：交接文档

- **Status**：ready-for-agent
- **日期**：2026-11-20
- **性质**：一次性交接文档，只为把「还剩什么」写清楚。做完连本目录一起删；`Status` 不更新，随目录消失。上一版规划（四步、21 份清单）已完成并并成八份，全文在 git 历史里（`b7bd142` 之前那一版），要追来由看它。

## 先读这三样

1. `docs/agents/必读-总则.md` 与 `docs/agents/必读-约束.md`——谁的话算数、条款分几级、七条红线。下面所有动作都受它们管。
2. `docs/adr/0064-约束平铺与入口只做索引.md`——为什么是八份、为什么是 `docs/agents/`、为什么上游三份不动。
3. 同一目录下的 `待同步适配.md`——剩下那件事的逐条清单。

## 现状：这些已经做完，别重做

- 约束在 `docs/agents/` 下八份：`必读-总则.md`、`必读-约束.md`、`按需-写代码.md`、`按需-测试环境.md`、`按需-写文档.md`、`按需-交付与提交.md`、`按需-给使用者看的文字.md`、`按需-触发词表.md`。
- `AGENTS.md` 已是入口索引：必读表、按需表，加 `## Agent skills` 段。八份与它对得上（`diff` 过）。
- 旧三份长文已删：`docs/个人开发规范.md`、`docs/agents/doc-guide.md`、`docs/测试环境使用指南.md`。
- 内容已按 `personal/约束文档检查经验/约束文档检查经验.md` 过了一遍，另做过两轮审计（内容归属、语义衔接），形式层的问题都改了。审计产出在 `build/audit-20261120/`，不需要重跑。

## 剩下的一件事：把十五处指针换掉

这些指针现在指的都是已删的旧文。逐条对照表在 `待同步适配.md`，一列是「现在写的」，一列是「换成」。汇总：

- `tools/check.sh` 四处：两处「判据见 doc-guide 零节」的提示、过滤名单里为旧规范留的那一项（删掉，只留 `^AGENTS\.md$`）、第 42 行「照 doc-guide 零节」那处注释。
- `tools/hooks/commit-msg` 两处：提交类型的出处指向 `docs/agents/按需-交付与提交.md`。
- `tools/test.sh` 一处：沙盒与临时目录指向 `docs/agents/按需-测试环境.md`。
- `.gitignore` 三行注释：「个人开发规范」改成「约束在 `docs/agents/` 下」。
- `modules/SSHFleet2_Go` 五处代码注释（`credential_test.go`、`connect_integration_test.go`、`log_session.go`、`run_upload_integration_test.go`、`run_upload_space_integration_test.go`）：路径与节号一起换。只换路径等于没换——`三.3`、`四.4`、`第三节`、`第四节` 这些节号在新体系里不存在了，要落到具体文件名。

## 做完的判据

```bash
git grep -n "个人开发规范\|doc-guide\.md\|测试环境使用指南" -- '*.md' '*.sh' '*.go' '*.yml' '*.json' .gitignore tools/hooks/commit-msg
```

命中只应剩下三处：`docs/adr/` 下的历史文档（当时的判断，不就地改写）、`modules/SSHFleet_Go_bak` 只读区、本议题目录。别处还命中就是没做完。

再让闸门跑一遍，确认没被自己的改动绊倒：

```bash
bash tools/check.sh
```

## 门禁

- 改 `tools/` 下的脚本与代码注释属改代码，动手前要有开发者说出的**「授权开工」四个字**。
- `.gitignore` 属约束类文件，要单独点头。
- 删本目录（含这份文档与 `待同步适配.md`）是最后一笔，单独提交——删不可逆，单成一笔才回滚得动。

## 不要做的

- 不要重新设计约束的形状（八份、两族、`必读-`／`按需-` 前缀）。已定，要改先问。
- 不要动 `AGENTS.md` 的 `## Agent skills` 段，也不要动 `docs/agents/issue-tracker.md`、`triage-labels.md`、`domain.md`：那四个路径被 setup 技能写死，动了它重跑时会判成未配置并原位重造。
- 不要动 `docs/交流方式.md`：它是开发者自己的一份，不在约束体系里，别处也不要引它。
- 不要按旧节号机械替换。旧文里「一.1 第七条」「三.3」「四.3」这类指代，靠关键词对上再落文件名；对不上的先问。

## 一处已知的两存，别顺手统一

议题目录名在 `AGENTS.md` 里写的是 `docs/issues/<yyyy-MM-dd-HHmm-主题>/`，`issue-tracker.md` 写的是 `<feature-slug>`。开发者已定：**他的习惯优先**，这句写在 `docs/agents/按需-写文档.md` 的「起不起规格」末尾。两处保持原样，不要为了「一致」去改任何一边。
