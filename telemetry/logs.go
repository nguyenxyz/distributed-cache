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

type zapLogger struct {
	logger *zap.SugaredLogger
}

func newZapLogger(serviceName, serviceID, logFileName string) *zapLogger {
	config := zap.NewProductionEncoderConfig()
	config.TimeKey = "@timestamp"
	config.MessageKey = "message"
	config.LevelKey = "log.level"
	config.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(config)
	consoleEncoder := zapcore.NewConsoleEncoder(config)
	logFile, _ := os.OpenFile(logFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defaultLogLevel := zapcore.DebugLevel
	logFields := zap.Fields(
		zap.String("service.name", serviceName),
		zap.String("service.id", serviceID),
	)
	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, zapcore.AddSync(logFile), defaultLogLevel),
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), defaultLogLevel),
	)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel), logFields)
	return &zapLogger{
		logger: logger.Sugar(),
	}
}

func (zl *zapLogger) Debugf(tmp string, args ...interface{}) {
	zl.logger.Debugf(tmp, args...)
}

func (zl *zapLogger) Infof(tmp string, args ...interface{}) {
	zl.logger.Infof(tmp, args...)
}

func (zl *zapLogger) Warnf(tmp string, args ...interface{}) {
	zl.logger.Warnf(tmp, args...)
}

func (zl *zapLogger) Errorf(tmp string, args ...interface{}) {
	zl.logger.Errorf(tmp, args...)
}

func (zl *zapLogger) Fatalf(tmp string, args ...interface{}) {
	zl.logger.Fatalf(tmp, args...)
}

func (zl *zapLogger) Panicf(tmp string, args ...interface{}) {
	zl.logger.Panicf(tmp, args...)
}
