package telemetry

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.opentelemetry.io/otel"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var (
	logger           Logger
	tracer           trace.Tracer
	meter            metric.Meter
	resource         *sdkresource.Resource
	initResourceOnce sync.Once
)

type ShutdownFunc func(ctx context.Context) error

func initResource() *sdkresource.Resource {
	initResourceOnce.Do(func() {
		addon, _ := sdkresource.New(
			context.Background(),
			sdkresource.WithOS(),
			sdkresource.WithProcess(),
			sdkresource.WithContainer(),
			sdkresource.WithHost(),
		)

		resource, _ = sdkresource.Merge(
			sdkresource.Default(),
			addon,
		)
	})

	return resource
}

func initTraceProvider() (*sdktrace.TracerProvider, error) {
	exporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(time.Second)),
		sdktrace.WithResource(initResource()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp, nil
}

func initMeterProvider() (*sdkmetric.MeterProvider, error) {
	exporter, err := stdoutmetric.New(
		stdoutmetric.WithPrettyPrint(),
	)
	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(initResource()),
	)
	otel.SetMeterProvider(mp)
	return mp, nil
}

func Init(ctx context.Context, serviceName, serviceID, logFileName string) (shutdown ShutdownFunc, err error) {
	tracerProvider, err := initTraceProvider()
	if err != nil {
		return nil, err
	}

	meterProvider, err := initMeterProvider()
	if err != nil {
		return nil, err
	}

	logger = newZapLogger(serviceName, serviceID, logFileName)
	tracer = tracerProvider.Tracer(serviceName, trace.WithInstrumentationAttributes(
		attribute.String("service.id", serviceID),
	))
	meter = meterProvider.Meter(serviceName, metric.WithInstrumentationAttributes(
		attribute.String("service.id", serviceID),
	))

	shutdownFuncs := []ShutdownFunc{tracerProvider.Shutdown, meterProvider.Shutdown}
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		return err
	}
	return shutdown, nil
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
