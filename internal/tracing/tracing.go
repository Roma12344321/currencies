package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/martynov-roman/currencies/internal/config"
)

func InitTracer(ctx context.Context, cfg *config.Config) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithEndpoint(cfg.Tracing.OtelEndpoint),
	)
	if err != nil {
		return nil, fmt.Errorf("otlptracehttp.New: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.Tracing.ServiceName),
		)),
	)
	otel.SetTracerProvider(tracerProvider)

	return tracerProvider, nil
}

func AddErrorToSpan(span trace.Span, err error) error {
	if err == nil || span == nil {
		return err
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())

	return err
}
