# M1-01 工程骨架：go.mod + main.go 主干十步 + internal/ 12 目录

Type: task
Status: resolved
Resolved: 2026-09-11

## 范围

创建 `modules/SSHFleet_Go/`：

- `go.mod`（module 名拟用 `sshfleet`，Go 版本按本机工具链）
- `main.go`（工程根）：承载「初始化 → 运行 → 退出」主干十步，每步一次函数调用，结果由 main 统一回收；**错误统一由 main 打印，main 独占退出权**
- `internal/` 12 个功能目录全部就位：config · log · cli · credential · dangercheck · nodelist · confirm · ssh · batch · result · output · common。本期未实现的目录先放一个 doc.go 占位（归属声明 + 一句话职责），后续里程碑填充
- 工程根 `config/` 目录就位（模板文件在 M1-02 落地）

依赖（spec 第三节）：`golang.org/x/crypto/ssh`、`github.com/pkg/sftp`、`github.com/BurntSushi/toml`、`spf13/pflag`、`go.uber.org/zap`、`charmbracelet/lipgloss`、`github.com/xuri/excelize/v2`。一次声明还是按期引入，属实现层，动工时定，不影响骨架。

## 依据

- `spec.md` 第六节（主干十步与承载目录）、D12、D21、D22、D39
- `个人开发规范.md` §二（入口承载主干全流程、internal 只建功能目录、必须含 log）
- 框架冻结（D21）：本工单完成即骨架定稿，此后所有改动都在框架内调整

## 实现层处理（非用户裁定项）

- 尚未实现的环节，主干调用先落在对应 internal 目录的占位函数上，统一返回「未实现」错误，保证骨架始终可编译；不留"先跑通一条链路"的临时形态

## 验证

- `go build ./...` 通过
- 运行可走到退出：未实现环节以「未实现」信息退出，不 panic

## Comments

- 2026-09-11 完成。`go build ./...` 与 `go vet` 零告警；依赖按期引入（M1 仅 toml/pflag/zap/lumberjack/lipgloss）。依赖追加：`gopkg.in/natefinch/lumberjack.v2`（zap 无自带轮转，用它保住旧版 50MB 轮转行为，spec 依赖清单之外的实现层补充）。stub 函数落各 internal 目录、统一返回「未实现」错误，骨架全程可编译。