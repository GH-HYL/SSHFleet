// Package log 承载主干第 2 步：初始化工具日志。
//
// 行为与旧版对齐：日志仅落文件（historys/<paths.tool>），不进终端；
// 级别 DEBUG 起；格式 "YYYY-MM-DD HH:mm:ss.SSS - [ LEVEL ] - message"；
// 50MB 轮转。级别序（对位旧 loguru）：DEBUG < INFO < SUCCESS < WARNING < ERROR。
package log

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// 自定义级别：SUCCESS 夹在 INFO 与 WARNING 之间（zap 标准级别之间的空档用连续整数重排）。
const (
	LevelDebug   = zapcore.Level(-1)
	LevelInfo    = zapcore.Level(0)
	LevelSuccess = zapcore.Level(1)
	LevelWarn    = zapcore.Level(2)
	LevelError   = zapcore.Level(3)
)

// levelName 级别名左对齐补齐到 7 列（[INFO   ] / [SUCCESS]，各行括号与文本列对齐；
// 旧 loguru 的居中写法「 INFO  」视觉上参差，用户 2026-09-14 裁定改左对齐）。
func levelName(l zapcore.Level) string {
	name := ""
	switch l {
	case LevelDebug:
		name = "DEBUG"
	case LevelInfo:
		name = "INFO"
	case LevelSuccess:
		name = "SUCCESS"
	case LevelWarn:
		name = "WARNING"
	case LevelError:
		name = "ERROR"
	default:
		name = l.CapitalString()
	}
	for len(name) < 7 {
		name += " "
	}
	return name
}

// fileCore 只向文件写一条主干消息（本工具的全部日志都是纯文本消息，不带结构化字段）。
type fileCore struct{ ws zapcore.WriteSyncer }

func (c *fileCore) Enabled(zapcore.Level) bool { return true }

func (c *fileCore) With(_ []zapcore.Field) zapcore.Core { return c }

func (c *fileCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return ce.AddCore(ent, c)
}

func (c *fileCore) Write(ent zapcore.Entry, _ []zapcore.Field) error {
	// 分隔符「-」两侧各留两个空格，把时间戳、级别、正文三段分开（用户 2026-09-14 裁定）
	_, err := fmt.Fprintf(c.ws, "%s  -  [%s]  -  %s\n",
		ent.Time.Format("2006-01-02 15:04:05.000"), levelName(ent.Level), ent.Message)
	return err
}

func (c *fileCore) Sync() error { return c.ws.Sync() }

// Logger 是全工具唯一的日志入口；由 main 注入给需要的环节。
type Logger struct {
	l      *zap.SugaredLogger
	rotate *lumberjack.Logger
}

// Close 关闭日志落盘（进程退出前的收尾；幂等）。
func (lg *Logger) Close() error {
	if lg.rotate == nil {
		return nil
	}
	return lg.rotate.Close()
}

// Raw 写入原始内容（不带时间戳与级别前缀）——工具日志的执行分界符专用：
// 经 logger 写会带上时间戳，用户 2026-09-14 裁定分界符直接落盘。
func (lg *Logger) Raw(text string) {
	if lg == nil || lg.rotate == nil {
		return
	}
	_, _ = lg.rotate.Write([]byte(text))
}

// 级别一律经本包自定义常量下发：zap 标准级别的数值与「SUCCESS 夹在 INFO 与 WARNING 之间」
// 的旧口径撞号（zap 的 Warn=1、Error=2 会被渲染成 SUCCESS / WARNING），故不能用
// SugaredLogger 的标准方法，统一走 Log(自定义级别, …)。
func (lg *Logger) Debug(args ...any)   { lg.l.Log(LevelDebug, args...) }
func (lg *Logger) Info(args ...any)    { lg.l.Log(LevelInfo, args...) }
func (lg *Logger) Success(args ...any) { lg.l.Log(LevelSuccess, args...) }
func (lg *Logger) Warn(args ...any)    { lg.l.Log(LevelWarn, args...) }
func (lg *Logger) Error(args ...any)   { lg.l.Log(LevelError, args...) }

// Init 创建 historys 目录并初始化工具日志（50MB 轮转，历史文件全保留）。
func Init(historys, toolFile string) (*Logger, error) {
	if err := os.MkdirAll(historys, 0o755); err != nil {
		return nil, err
	}
	rotate := &lumberjack.Logger{
		Filename: filepath.Join(historys, toolFile),
		MaxSize:  50, // MB
	}
	core := &fileCore{ws: zapcore.AddSync(rotate)}
	return &Logger{l: zap.New(core).Sugar(), rotate: rotate}, nil
}

// InitExec 在归档目录内创建执行期日志（对位旧引擎写入归档目录的 SSHFleet.log：
// 带时间戳与级别的节点级运行明细）。一次执行一个文件，不轮转。
func InitExec(dir, filename string) (*Logger, error) {
	rotate := &lumberjack.Logger{Filename: filepath.Join(dir, filename)}
	core := &fileCore{ws: zapcore.AddSync(rotate)}
	return &Logger{l: zap.New(core).Sugar(), rotate: rotate}, nil
}
