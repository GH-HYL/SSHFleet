# 02: 凭据与主密钥那摊的重复收成一处

**要做的**：做完之后，主密钥的读写、指纹、五状态判定、封与解仍跟现在一模一样，但同一个串、同一段逻辑只剩一处成形——改一处不会再漏掉另一处。屏幕一个字不变，rc 文件写出来一样。

**Blocked by**：01（验收要在沙盒里跑，车架没起就验不了）

**Status**：done

- [x] `export` 前缀只剩一处（一个常量或一个成形函数），三处调用都走它
- [x] rc 文件名的两套写法收成一套，不再一半用列表、一半用字面量
- [x] `ErrBadCipher` 的十一次包装收成一个构造点；错误的 `Unwrap` 链与今天相同（拿既有用例判）
- [x] 那两段八行逐字相同的代码只剩一份，另一处调它
- [x] 当前 rc 路径不再各处重算，统一走那个算路径的函数
- [x] `go test ./internal/credential/...` 与 `go test -race ./...` 绿
- [x] 沙盒里 `--key-status`（不一致那一档）与 `--gen-key`（答否）各跑一次，屏与执行期日志与收账前逐字节相同

**不在这单**：那三句中文从本包上屏（04 号单）；覆盖确认那三档的用例（05 号单）。

## Comments

**2026-11-06：五笔全收，屏一字未动。**

- `exportPrefix` 常量：认行（`isKeyExportLine`）、取值（`findExportInFile`）、写行（`writeKeyToRC`）三处都从它拼。
- rc 文件名收成 `rcZsh`／`rcBash` 两个常量：候选列表与「当前 shell 挑哪个」同源；`persistKey` 的路径改走 `rcFilePath()`，不再自己 `filepath.Join` 一次。
- `cipherErr(detail, args…)` 是 `ErrBadCipher` 那一族的唯一成形处，十一次包装全经它；文案与 `%w` 链一字未改（既有四条 `errors.Is` 用例照旧过）。
- `gcmOf(key)` 收掉那两段逐字相同的「建密码器／建 GCM」，封（`Encrypt`）与解（`Decrypt`）各调一次。

**验收**：`gofmt` 干净、`vet`／`build` 过、`go test -race ./...` 十八包全绿（`guard` 的结构检查也在里面）。沙盒 `check_keys.sh` 比了七样，两侧逐字节相同：`--key-status` 未设置档、`--gen-key` 生成、生成后查状态、环境里塞另一把造不一致、已有密钥再 `--gen-key` 读输入结束（按否办）、两次 rc 文件内容（随机那把与八位指纹按 `<KEY>`／`<FP>` 归一）。
