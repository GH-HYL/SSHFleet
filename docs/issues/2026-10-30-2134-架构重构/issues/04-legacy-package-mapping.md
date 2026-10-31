# 04: 老工程 13 个包怎么摊进五站——尤其 output 那 2249 行怎么劈

**Type**：grilling
**Status**：open
**Blocked by**：无，可立即开工

## Question

老工程 `modules/SSHFleet_Go/internal/` 现有 13 个包，非测试代码合计 13147 行，按行数排：`ssh` 3589、`output` 2249、`cli` 1739、`credential` 1180、`batch` 698、`common` 694、`nodelist` 628、`dangercheck` 597、`config` 577、`confirm` 463、`verdict` 452、`result` 159、`log` 122。加上根目录 `main.go` 512 行（十步主干）。另有 51 个 `_test.go` 文件散在 12 个包里。

初步摊法（要在这条工单里核实）：

| 老位置 | 新落点 |
| --- | --- |
| `config` + `cli`（解析部分）+ `nodelist` | `ingest` |
| `dangercheck` + `confirm` + `cli`（校验部分） | `gate` |
| `ssh` + `batch` | `execute` |
| `verdict` + `result` | `appraise` |
| `output` | `present` + `archive`（横切） |
| `common` | 拆散，按「产不产业务数据」分流 |
| `log` | `log`（横切） |
| `credential` | `credential`（独立垂直切片） |

注意这张表没有一处是干净的：`cli` 要劈成解析与校验两半、`result` 与 `verdict` 要合、`common` 要拆散。表上每一行都是一次判断，不是一次搬运。

**最大的单点风险是 `output` 那 2249 行**：呈现、报告、xlsx、归档全在里面，而新分法要把它劈成 `present`（五站之一，产结论给人看）与 `archive`（横切，只发路径字符串）。这一刀切在哪，决定呈现站的接口宽窄。

要定的：

1. 上面这张表对不对，有没有哪一块摊错了或摊不进去。
2. `output` 怎么劈：按「终端输出 / 文件输出 / xlsx / 归档」分，还是按「呈现 vs 落盘」分？`internal/common` 里那些无状态小工具（字节格式化、颜色、CSV 文本）归谁——呈现站，还是横切。
3. 有没有哪一块**在五站里找不到家**——那说明五站这个切法在这个工程上不成立，这是最坏的情况，得早发现。
4. **每一格的那一句话。** 迁移地图每行要能写出「这个包只干哪一件」，一句话，写不出来、或者得用「和」把三件事串起来的，那一行的摊法就还没想清（03 号工单的判据同一条）。这句话建包时原样成为包的头注释，后面会接一句「判据见 ADR-0032 第四条」——见 07 号工单第二笔。

答案要能落成一张完整的迁移地图，一行一块，一行一句，搬完能对账。
