package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var (
	logger Logger
	tracer trace.Tracer
	meter  metric.Meter
)

const (
	serviceName = "service.box"
	serviceHost = "localhost:8080"
	logFileName = "service.box.log"
)

type CancelFunc func(ctx context.Context) error

func Init(ctx context.Context) (CancelFunc, error) {
	otelShutdown, err := setupOTelSDK(ctx)
	if err != nil {
		return otelShutdown, err
	}
	logger = newZapLogger(serviceName, serviceHost, logFileName)
	tracer = otel.Tracer(serviceName, trace.WithInstrumentationAttributes(
		attribute.String("service.host", serviceHost)),
	)
	meter = otel.Meter(serviceName, metric.WithInstrumentationAttributes(
		attribute.String("service.host", serviceHost)),
	)

	return otelShutdown, nil
}

func Log() Logger {
	return logger
}

func Trace() trace.Tracer {
	return tracer
}

func Metric() metric.Meter {
	return meter
}
