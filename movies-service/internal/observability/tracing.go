// Package observability holds movies-service's tracing bootstrap. There's
// no equivalent metrics.go here yet (movies-service only speaks gRPC, no
// HTTP listener to expose /metrics on — see docs/adr/0003-*.md, "Itens
// descobertos"); this package may grow into a fuller mirror of
// api-gateway/internal/observability if that gap gets closed later.
package observability

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// InitTracing — see api-gateway/internal/observability/tracing.go for the
// full rationale (identical here; duplicated rather than shared because
// this is per-process bootstrap wiring, not a cross-service contract —
// unlike shared.AMQPHeaderCarrier, which is).
func InitTracing(ctx context.Context, serviceName, otlpEndpoint string) (shutdown func(context.Context) error, err error) {
	noop := func(context.Context) error { return nil }
	if otlpEndpoint == "" {
		slog.Info("OTEL_EXPORTER_OTLP_ENDPOINT não definido — tracing desligado", "service", serviceName)
		return noop, nil
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(otlpEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return noop, err
	}

	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(serviceName)))
	if err != nil {
		return noop, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	slog.Info("tracing ligado", "service", serviceName, "otlp_endpoint", otlpEndpoint)
	return tp.Shutdown, nil
}
