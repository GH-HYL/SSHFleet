package log

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// 级别名与老版同一个写法：左对齐补齐到 7 列，括号与文本列才对得齐。
const (
	levelDebug   = "DEBUG  "
	levelInfo    = "INFO   "
	levelSuccess = "SUCCESS"
	levelWarn    = "WARNING"
	levelError   = "ERROR  "
)

// rotateBytes 是轮转阈值：与老版 lumberjack 的 50MB 一致。
const rotateBytes = 50 * 1024 * 1024

// Logger 是一次运行的工具日志。句柄由调用方持有、由调用方关闭——本包不设包级变量，
// 免得出第二个「当前日志在哪」的来源。
type Logger struct {
	mu sync.Mutex
	f  *os.File
}

// Init 建目录、开日志文件（追加）。建不起来返回错误，本包不造屏、不退出。
//
// 两格落点由归档包发（`archive.ToolLog`）、塔尖转手：本包只把它们合起来打开，不自己算
// 「日志在哪」——ADR-0032 第四条「路径只能从 archive 拿」。
func Init(historyDir, toolFile string) (*Logger, error) {
	if err := os.MkdirAll(historyDir, 0o755); err != nil {
		return nil, err
	}
	return open(filepath.Join(historyDir, toolFile))
}

// InitExec 开执行期日志：本次运行里节点级的明细，落在归档目录里。
//
// 路径由归档包给（`archive.Paths.ExecLog`）——本包不拼它、也不记它。目录由归档包建过，
// 所以这里不再 MkdirAll：建不出来就是上游没给对，不该在这里默默补一个目录。
func InitExec(path string) (*Logger, error) {
	return open(path)
}

// open 是「到点轮转、追加打开」那一段：工具日志与执行期日志共用，只有路径来源不同。
func open(path string) (*Logger, error) {
	rotateIfBig(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &Logger{f: f}, nil
}

// rotateIfBig 到阈值就把旧文件改名留档。
//
// 老版用 lumberjack 轮转（50MB，历史文件全保留）；新工程零第三方依赖，这里只做同一件事的
// 最小版本：超过阈值时改名成 `<名>-<时间>.log`，不删、不压缩。
func rotateIfBig(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < rotateBytes {
		return
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	stamp := time.Now().Format("20060102T150405")
	_ = os.Rename(path, fmt.Sprintf("%s-%s%s", base, stamp, ext))
}

// Close 关闭句柄；幂等。
func (lg *Logger) Close() error {
	if lg == nil || lg.f == nil {
		return nil
	}
	lg.mu.Lock()
	defer lg.mu.Unlock()
	err := lg.f.Close()
	lg.f = nil
	return err
}

// Raw 写原始内容，不带时间戳与级别前缀——工具日志的执行分界符专用。
func (lg *Logger) Raw(text string) {
	lg.write(func(f *os.File) { _, _ = f.WriteString(text) })
}

func (lg *Logger) Debug(args ...any)   { lg.line(levelDebug, fmt.Sprint(args...)) }
func (lg *Logger) Info(args ...any)    { lg.line(levelInfo, fmt.Sprint(args...)) }
func (lg *Logger) Success(args ...any) { lg.line(levelSuccess, fmt.Sprint(args...)) }
func (lg *Logger) Warn(args ...any)    { lg.line(levelWarn, fmt.Sprint(args...)) }
func (lg *Logger) Error(args ...any)   { lg.line(levelError, fmt.Sprint(args...)) }

// Fatal 把一个多行的错误写进日志：每条一行，续行缩进两格、前缀仍带环节标签。
//
// 致命错误同时落日志是老版的做法：终端一关就只剩日志可查。os.Exit 不跑 defer，
// 所以调用方在退出那条路上显式调它。
func (lg *Logger) Fatal(where, text string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	first := true
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if first {
			lg.line(levelError, fmt.Sprintf("[%s] %s", where, line))
			first = false
			continue
		}
		lg.line(levelError, fmt.Sprintf("[%s]   %s", where, line))
	}
}

// line 写一整行：「时间戳  -  [级别]  -  正文」。分隔符两侧各留两个空格，与老版一致。
func (lg *Logger) line(level, msg string) {
	stamp := time.Now().Format("2006-01-02 15:04:05.000")
	lg.write(func(f *os.File) {
		_, _ = fmt.Fprintf(f, "%s  -  [%s]  -  %s\n", stamp, level, msg)
	})
}

func (lg *Logger) write(fn func(*os.File)) {
	if lg == nil {
		return
	}
	lg.mu.Lock()
	defer lg.mu.Unlock()
	if lg.f == nil {
		return
	}
	fn(lg.f)
}

// Startup 写启动段那几行：分隔线、开始执行、工作路径、日志目录与文件名。
//
// 由日志包自己算工作路径——塔尖不许读文件系统、也不许拼字符串（ADR-0032 第五条）。
// 目录与文件名两格由调用方递进来（归档包发、塔尖转手）：本包不记「日志在哪」。
// 「原始选项/参数」那一行本阶段不写：它要先脱敏，而脱敏归 mask（随阶段 07）。
func (lg *Logger) Startup(historyDir, toolFile string) {
	lg.Raw("\n    " + strings.Repeat("─", 50) + "\n\n")
	lg.Info("SSHFleet工具开始执行")
	wd, _ := os.Getwd()
	lg.Info("工作路径：" + wd)
	lg.Info("日志目录：" + historyDir + "，工具日志文件名：" + toolFile)
}
