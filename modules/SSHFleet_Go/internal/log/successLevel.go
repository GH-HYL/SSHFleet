package log

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// 自定义 Logger 类型，嵌入 *zap.Logger
type customLogger struct {
	*zap.Logger
}

// Succ 方法记录 SUCCESS 级别的日志
func (l *customLogger) Succ(msg string, fields ...zap.Field) {
	// 使用自定义的 SUCCESS 级别（比 Info 高一级）
	l.Log(zapcore.InfoLevel+1, msg, fields...)
}
