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

// levelName 级别名居中补齐到 7 列（对位旧 loguru "{level: ^7}"）。
func levelName(l zapcore.Level) string {
	var name string
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
	left := (7 - len(name)) / 2
	right := 7 - len(name) - left
	pad := func(n int) string {
		s := ""
		for i := 0; i < n; i++ {
			s += " "
		}
		return s
	}
	return pad(left) + name + pad(right)
}

// fileCore 只向文件写一条主干消息（本工具的全部日志都是纯文本消息，不带结构化字段）。
type fileCore struct{ ws zapcore.WriteSyncer }

func (c *fileCore) Enabled(zapcore.Level) bool { return true }

func (c *fileCore) With(_ []zapcore.Field) zapcore.Core { return c }

func (c *fileCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return ce.AddCore(ent, c)
}

func (c *fileCore) Write(ent zapcore.Entry, _ []zapcore.Field) error {
	_, err := fmt.Fprintf(c.ws, "%s - [%s] - %s\n",
		ent.Time.Format("2006-01-02 15:04:05.000"), levelName(ent.Level), ent.Message)
	return err
}

func (c *fileCore) Sync() error { return c.ws.Sync() }

// Logger 是全工具唯一的日志入口；由 main 注入给需要的环节。
type Logger struct{ l *zap.SugaredLogger }

func (lg *Logger) Debug(args ...any)   { lg.l.Debug(args...) }
func (lg *Logger) Info(args ...any)    { lg.l.Info(args...) }
func (lg *Logger) Success(args ...any) { lg.l.Log(LevelSuccess, args...) }
func (lg *Logger) Warn(args ...any)    { lg.l.Warn(args...) }
func (lg *Logger) Error(args ...any)   { lg.l.Error(args...) }

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
	return &Logger{l: zap.New(core).Sugar()}, nil
}
