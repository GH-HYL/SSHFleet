# 01: 纸上补齐五种模式的格子，凭据包交出解密码的入口

**要做的**：做完之后，三张纸能装下另外四种模式要的东西——改密的新密码、上传的采集结果、代填表、脚本与代填的路径、身份开关给没给、建议并发、没配到解释器的后缀、登录方式、中间步骤；报告要印的那几件也进得了密文那一段。同时凭据包能按配置的加密开关，把 `--change-password` 的值解成明文。

**Blocked by**：无，可立即开工。

**Status**：ready-for-agent

- [ ] `contract` 补齐：`Task.Passwd.NewPassword`、`Task.Upload.Files`／`Skipped`／`TopEntries`、命令与脚本载荷的 `Answers`、`Resources.Script`、`RunParams.SudoGiven`／`MatchRegex`／`MatchCaseSensitive`／`ConcurrencyAdvice`／`FallbackExt`、`NodeResult.AuthMethod`／`Steps`、`Extra.Transfer`／`Extra.Answer`——每个字段一句注释说清谁填、谁读
- [ ] 随行的两个新类型（可传文件条目、中间步骤）落契约，站里不另造一份
- [ ] 运行上下文（密文）多装报告要印的那七样；封与解两处固定密文样本重录，`parse`／`report` 两处 `runmeta_test.go` 跟着过
- [ ] `credential` 交出一个入口：给「值 + 加密开关」，返回明文或一份问题清单；明文与密文两份 `testdata`、固定主密钥，**不读真环境变量**
- [ ] 改密值贴的是密文内容而不是路径时，两种开关各给一句能照做的话（逐字照老版）
- [ ] `gofmt -l .` 空、`go build`／`go vet`／`go test ./...` 绿；真工程仍 17 包零违规、红线 39 条未动

**不在这单**：不改任何站的行为；不写新红线；不动红线名单。
