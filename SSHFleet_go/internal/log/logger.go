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
var Zlog *customLogger

// InitLogger Zlog配置初始化
func InitLogger(logPath string) error {
	var writeSyncer zapcore.WriteSyncer

	if logPath != "" {
		zFile := path.Join(logPath, "SSHFleet.log")

		file, err := os.OpenFile(zFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			fmt.Println(err)
			return err
		}

		if _, err := file.WriteString("\n\n\n"); err != nil {
			return err
		}

		writeSyncer = zapcore.AddSync(file)
	} else {
		writeSyncer = zapcore.AddSync(os.Stderr)
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:       "time",
		LevelKey:      "level",
		NameKey:       "logger",
		MessageKey:    "message",
		StacktraceKey: "stacktrace",
		EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
		},
		EncodeLevel:      zapcore.CapitalLevelEncoder,
		EncodeDuration:   zapcore.StringDurationEncoder,
		ConsoleSeparator: " ",
	}

	encoderConfig.EncodeLevel = func(level zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
		if level == zapcore.InfoLevel+1 {
			enc.AppendString("[ SUCCESS ]")
		} else {
			enc.AppendString("[ " + level.CapitalString() + " ]")
		}
	}

	encoder := zapcore.NewConsoleEncoder(encoderConfig)
	core := zapcore.NewCore(encoder, writeSyncer, zapcore.DebugLevel)
	baseLogger := zap.New(core, zap.AddStacktrace(zapcore.ErrorLevel))
	Zlog = &customLogger{Logger: baseLogger}

	return nil
}
