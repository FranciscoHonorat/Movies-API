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

// InitTracing wires this process's spans to an OTLP/gRPC collector —
// Jaeger, in this project (docker-compose.yml, JAEGER_OTLP_ENDPOINT). If
// otlpEndpoint is empty, tracing is skipped entirely: no TracerProvider
// gets registered, and every otel.Tracer(...).Start call anywhere in the
// codebase quietly becomes a no-op (that's the OTel API's documented
// default until SetTracerProvider is called) — so nothing else in the
// code needs an "is tracing on?" check, only this one place.
//
// Sampling is AlwaysSample, a deliberate choice for a study project where
// seeing every request in Jaeger matters more than reducing collector
// load — see docs/adr/0007-*.md for why this isn't what you'd want in
// production (a probabilistic or rate-limiting sampler instead).
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
