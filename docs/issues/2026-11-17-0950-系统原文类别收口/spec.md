# 规格：收掉「系统原文」那一类别（ADR-0047 的过渡口子）

**状态**：done
**底本**：ADR-0047「影响」末条；`build/SSHFleet-handoff-2026-11-16-1753.md` 三.2

## Problem Statement

建归档目录失败、报告写不出来这两条路上，塔尖把站交上来的 `error` 原文直接贴上屏。那半句中文是站里自己拼的——`archive` 一句、`report` 三句、运行上下文解不开四句——不受 `message` 管：改一次说法要进站里改，而站按权限表不许依赖契约，连个类别都不许交。

使用者这一面还有更实际的：那几句是 `fmt.Errorf` 带 `%w` 拼的，屏上落成「建归档目录失败（history/…）：mkdir …: Access is denied.」这种半中文半英文，谁也不敢动——动就得连站的签名一起改。

## Solution

这几句改由 `message` 按类别成形，站里只交「哪一类、哪个目录、系统原话」。收完之后「系统原文」类别没有调用者，删掉。

**屏上字节不变**：模板拼出来与今天逐字一致。

## User Stories

1. As a 使用者, I want 建归档目录失败那一屏仍写明是哪个目录、系统怎么说, so that 我知道去查哪儿、去改哪个权限。
2. As a 维护者, I want 这几句改口径只改一处, so that 不用为一个词同时进三个包。
3. As a 维护者, I want 站里没有半句自己的话, so that 「屏上的字都由 `message` 出」不必靠人盯。

## Implementation Decisions

- **归档包造不出纸**：按权限表它只依赖标准库（ADR-0032 第四条），所以失败时交回已算出的目录（`Paths.Dir`、`Paths.Root`）与系统原话，纸由塔尖造。
- **报告站的失败出口从 `error` 换成 `*contract.CannotStart`**，与执行站同形。这一条改红线：`TestRedlineReportScreenAndPath` 钉的就是出参形状。单独一条 ADR-0057（规范一.1 第七条）。
- **运行上下文解不开那几句**改成与解析站封侧同形（交 `contract.CannotStartReason` 加一个「坏没坏」的 bool）；建不出密码器／GCM 复用已有的 `CannotStartRunMetaCipher`／`CannotStartRunMetaGCM`。
- 屏上字节不变，不记 `CHANGELOG`。

## Testing Decisions

- 报告站与归档包的用例改判「交回哪一类、带哪些格」，不比对整句；整句归 `message` 的模板管。
- 红线改判的判据：`go test -count=1 -run TestRedlineReportScreenAndPath ./internal/guard` 绿，且 `guard/redline.go` 那一行登记照报错里印出的照抄。
- 收尾：`go test -count=1 ./...`、`go vet`、`go vet -tags integration`、`gofmt -l` 全干净。

## Out of Scope

- 塔尖其余自拼中文（`execute`／`parse` 那几十处）不迁（ADR-0047「影响」第二条）。
- 不新增任何使用者可见的说法，也不改任何一句现有的说法。
- 不碰报告站的版式（那是 `message` 自定义区的事）。

## 工单

`issues/01`–`03`。
