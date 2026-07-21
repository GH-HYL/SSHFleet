package log

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// 自定义 Logger 类型，嵌入 *zap.Logger
type customLogger struct {
	*zap.Logger
}

// 自定义 SugaredLogger 类型，嵌入 *zap.SugaredLogger
type customSugaredLogger struct {
	*zap.SugaredLogger
}

// Sugar 方法返回自定义的 SugaredLogger
func (l *customLogger) Sugar() *customSugaredLogger {
	return &customSugaredLogger{SugaredLogger: l.Logger.Sugar()}
}

// Succ 方法记录 SUCCESS 级别的日志
func (l *customLogger) Succ(msg string, fields ...zap.Field) {
	// 使用自定义的 SUCCESS 级别（比 Info 高一级）
	l.Log(zapcore.InfoLevel+1, msg, fields...)
}

// Succ 方法记录 SUCCESS 级别的日志（SugaredLogger 版本）
func (l *customSugaredLogger) Succ(args ...interface{}) {
	l.Log(zapcore.InfoLevel+1, args...)
}

// Succf 方法记录 SUCCESS 级别的格式化日志
func (l *customSugaredLogger) Succf(template string, args ...interface{}) {
	l.Logf(zapcore.InfoLevel+1, template, args...)
}

// Succw 方法记录 SUCCESS 级别的结构化日志
func (l *customSugaredLogger) Succw(msg string, keysAndValues ...interface{}) {
	l.Logw(zapcore.InfoLevel+1, msg, keysAndValues...)
}
