package log

import (
	"fmt"
	"os"
	"path"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Zlog 全局日志
var Zlog *customLogger // 基于 zap.Logger 定制化的结构化日志（有时间等级，输出到文件）

// InitLogger Zlog配置初始化：输出到文件，有时间等级
func InitLogger(logPath string, File string) error {

	zFile := path.Join(logPath, File)

	// 创建日志文件
	file, err := os.OpenFile(zFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Println(err)
		return err
	}

	// 日志文件启动输出空白行
	if _, err := file.WriteString("\n\n\n"); err != nil {
		return err
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:  "time",  // 时间字段的键名
		LevelKey: "level", // 日志等级字段的键名
		//CallerKey:     "caller",  // 调用者位置字段的键名
		NameKey:       "logger",  // 日志器名称字段（可选）
		MessageKey:    "message", // 日志消息字段的键名
		StacktraceKey: "stacktrace",
		EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
		}, // 自定义时间格式
		EncodeLevel:      zapcore.CapitalLevelEncoder,   // 大写等级：INFO, ERROR
		EncodeDuration:   zapcore.StringDurationEncoder, // 持续时间格式
		ConsoleSeparator: " ",                           // 控制台分隔符
	}
	// 自定义等级格式（比如 [INFO]）
	encoderConfig.EncodeLevel = func(level zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
		// 特殊处理 SUCCESS 级别
		if level == zapcore.InfoLevel+1 {
			enc.AppendString("[ SUCCESS ]")
		} else {
			enc.AppendString("[ " + level.CapitalString() + " ]")
		}
	}
	// 文本格式编码器（非JSON）
	encoder := zapcore.NewConsoleEncoder(encoderConfig)

	core := zapcore.NewCore(encoder, zapcore.AddSync(file), zapcore.DebugLevel)

	// 创建自定义 Logger
	baseLogger := zap.New(core, zap.AddStacktrace(zapcore.ErrorLevel))
	Zlog = &customLogger{Logger: baseLogger}

	return nil
}
