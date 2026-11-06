# 01: 凭据包拿到主密钥那一摊

**要做的**：做完之后，凭据包能说出主密钥的五种状态（正常／不一致／已保存但当前终端没读到／未持久化／未设置）、能报两处来源各是哪一把（只报八位指纹，不报密钥本身）、能把一段明文封成密文、能生成一把新主密钥，并能把密钥写成本机持久值（POSIX 收敛进当前 shell 的 rc 文件末尾，Windows 走 `setx`）。持久值那一摊收成一个可注入的值：读环境变量、读本机持久值、写本机持久值三件事各是一个函数，默认实现按平台，测试注入假件。

**Blocked by**：无，可立即开工。

**Status**：done

- [x] 五种状态各判一次（假件注入）：两处都有且不同→不一致；两处都有且相同→正常；只剩持久值→已保存但当前终端没读到；只剩环境变量→未持久化；都空→未设置（另加「两侧 rc 互相不同」也算不一致）
- [x] 指纹对已知字面量：`sha256("abc")` 前 4 字节 = `ba7816bf`；空密钥报「（未设置）」
- [x] 封一段明文再解回来是同一段字（字面量往返）；换一把密钥解不开
- [x] 生成的密钥 48 字符，两次生成不相同
- [x] rc 文件里原本有多条主密钥行与本工具的分组注释：写完只剩末尾一条；另一侧 rc 文件存在且已有密钥行时跟着改成同一把
- [x] 测试全程用注入的假件或临时 HOME：`go test ./...` 不写本机 rc 文件、不碰注册表

## Comments

**一个口径按「另一侧」读**：spec 第六节「文件不存在时不新建」实现成「另一侧 rc 不新建」，当前 shell 那一侧照老版新建——空主目录上 `--gen-key` 总得有个地方落，不建就等于这条命令没写成。用例 `TestPersistKeyDoesNotCreateOtherSide` 把两半都钉住了。

**形状**：`KeySource` 是那一格可注入的值（`EnvLookup`／`PersistedLookup`／`Persist`／`RCFile`／`Host`），平台实现在 `persist_posix.go` 与 `persist_windows.go`；rc 文件的文本原语不挂平台标签，两侧单测都能跑。`Fingerprint`／`GenerateMasterKey`／`Encrypt` 与 `Decrypt` 同住 `masterkey.go`：密文格式的两面不拆包。

**一处顺手改名**：`envName` 改成导出的 `EnvName`——密钥命令的错误屏里要点名那一格变量，句子在 `message`，名字得从凭据包递过去，不能各写一份字面量。

**交付后审计（2026-11-06 下午）**：命中六条，本轮都不处置，等 09 之前统一收。五条属「同一形式多处拼」：`rcfile.go:21`／`:53`／`:87` 三处各拼 `export SSHFLEET_KEY=` 前缀；`persist_posix.go:22` 的 `rcFiles` 与 `:36`／`:38` 的字面量是两套写法；`masterkey.go:57`／`:61`／`:65`／`:69` 四个 `ErrBadCipher` 包装与 `credential.go` 里七个同形（老版有 `cipherErr` 构造点，这次没搬）；`masterkey.go:59-66` 与 `credential.go:124-131` 那八行逐字相同；`persist_posix.go:87` 重算了一次当前 rc 路径，`rcFilePath` 就是干这个的。第六条是 1.2：「建不出密码器」「建不出 GCM」「取不到随机数」三句中文从本包经 `err.Error()` 上屏（`keytool/run.go:105` 收走），与「中文句子一律住 `message`」不符。
