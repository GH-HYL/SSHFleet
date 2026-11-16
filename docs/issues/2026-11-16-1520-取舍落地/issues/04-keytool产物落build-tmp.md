# 04: `keytool` 单测的产物落 `build/tmp`

**要做的**：`internal/keytool/run_confirm_test.go` 的 `buildBinary`（`:21-33`）不再把二进制写进 `build/建站09_AB_2026-11-06_1535`——那是 03 轴的 A/B 取证目录，不是测试沙盒。改用本包已有的 `buildTmp()`（`internal/keytool/tmpdir_test.go`）。同一文件 `:62` 那处认仓库根同办。

**Blocked by**：无，可立即开工。

**Status**：done

- [ ] 两处 `filepath.Abs(filepath.Join("..", "..", "..", ".."))` 消失，改用 `buildTmp()`
- [ ] 产物落 `build/tmp/test/keytool/`，`build/建站09_AB_2026-11-06_1535` 里不再出现新的 `sshfleet2_keytool_test`
- [ ] 取证目录里的旧产物**不动**（那是 03 轴的证据，删了就对不上账）
- [ ] POSIX 侧：`go test -count=1 ./internal/keytool` 全绿；Windows 侧 skip 的理由不变
- [ ] `go vet`、`gofmt -l` 干净

## Comments

**2026-11-16 拆单**：`build/` 不进版本控制，所以这一条不改任何人的历史账，只是别再往别人的取证目录里写东西。落点与 01 号单同一个办法（`buildTmp()`），不新建基建。

**2026-11-16 落地**：`buildBinary` 改用本包 `buildTmp()`，按模块路径编（`go build -o … sshfleet2`，不设 `Dir`）；`genKeyScene` 读随包配置改成 `../../config`（两段相对、不出模块）。`build/建站09_AB_2026-11-06_1535` 里的旧产物一个没动。

**本机验到哪一步**：Windows 上那三条进程级用例按文件开头的规定 skip（`TestGenKeyConfirmYesOverwrites` 等三条），所以本机只到编译与 vet；「产物落 `build/tmp/test/keytool`」要等 POSIX 侧跑一次才算验过。WSL 里没装 Go，这台机器上补不了——已记进 `docs/issues/未收尾清单.md` 的「要复核」。
