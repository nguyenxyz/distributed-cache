package telemetry

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger interface {
	Debugf(tmp string, args ...interface{})
	Infof(tmp string, args ...interface{})
	Warnf(tmp string, args ...interface{})
	Errorf(tmp string, args ...interface{})
	Fatalf(tmp string, args ...interface{})
	Panicf(tmp string, args ...interface{})
}

type ZapLogger struct {
	logger *zap.SugaredLogger
}

type Config struct {
	ServiceName, ServiceHost, LogFileName string
}

func NewZapLogger(cfg Config) *ZapLogger {
	config := zap.NewProductionEncoderConfig()
	config.TimeKey = "@timestamp"
	config.MessageKey = "message"
	config.LevelKey = "log.level"
	config.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(config)
	consoleEncoder := zapcore.NewConsoleEncoder(config)
	logFile, _ := os.OpenFile(cfg.LogFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defaultLogLevel := zapcore.DebugLevel
	logFields := zap.Fields(
		zap.String("service.name", cfg.ServiceName),
		zap.String("service.host", cfg.ServiceHost),
	)
	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, zapcore.AddSync(logFile), defaultLogLevel),
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), defaultLogLevel),
	)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel), logFields)
	return &ZapLogger{
		logger: logger.Sugar(),
	}
}

func (zl *ZapLogger) Debugf(tmp string, args ...interface{}) {
	zl.logger.Debugf(tmp, args...)
}

func (zl *ZapLogger) Infof(tmp string, args ...interface{}) {
	zl.logger.Infof(tmp, args...)
}

func (zl *ZapLogger) Warnf(tmp string, args ...interface{}) {
	zl.logger.Warnf(tmp, args...)
}

func (zl *ZapLogger) Errorf(tmp string, args ...interface{}) {
	zl.logger.Errorf(tmp, args...)
}

func (zl *ZapLogger) Fatalf(tmp string, args ...interface{}) {
	zl.logger.Fatalf(tmp, args...)
}

func (zl *ZapLogger) Panicf(tmp string, args ...interface{}) {
	zl.logger.Panicf(tmp, args...)
}
