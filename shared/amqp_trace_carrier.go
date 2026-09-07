package shared

import amqp "github.com/rabbitmq/amqp091-go"

// AMQPHeaderCarrier adapts amqp.Table (the header map every AMQP message
// carries) to satisfy OpenTelemetry's propagation.TextMapCarrier
// interface, so trace context can ride along on a published
// MoviePublisherMessage and be picked back up when movies-service
// consumes it.
//
// This is the one hop in re-api-books that off-the-shelf instrumentation
// doesn't cover: otelgin/otelgrpc propagate context automatically over
// HTTP and gRPC because both have a natural place to put a header on a
// live request/response. A queued AMQP message has no such round trip —
// whoever publishes and whoever consumes have to agree on where the
// context goes, which is exactly the "contract between the two services"
// this package already exists for (see MoviePublisherMessage).
//
// Usage (see api-gateway/internal/adapters/rabbitmq/publisher.go and
// movies-service/cmd/main.go):
//
//	headers := amqp.Table{}
//	otel.GetTextMapPropagator().Inject(ctx, shared.AMQPHeaderCarrier(headers))
//	// ... publish with amqp.Publishing{Headers: headers, ...}
//
//	ctx = otel.GetTextMapPropagator().Extract(ctx, shared.AMQPHeaderCarrier(msg.Headers))
type AMQPHeaderCarrier amqp.Table

func (c AMQPHeaderCarrier) Get(key string) string {
	if v, ok := c[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (c AMQPHeaderCarrier) Set(key, value string) {
	c[key] = value
}

func (c AMQPHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}
