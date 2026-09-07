package rabbitmq

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/FranciscoHonorat/movies/shared"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("api-gateway/rabbitmq")

// RabbitMQPublisher reconnects lazily: Publish checks whether its
// connection/channel are still alive and redials before publishing if
// not. Without this, once RabbitMQ drops the connection (a restart, a
// network blip) every future Publish would fail forever, since amqp091-go
// doesn't reconnect on its own and the original version of this adapter
// dialed exactly once at construction time. Measured directly: with the
// old single-dial version, restarting the rabbitmq container left
// api-gateway's circuit breaker permanently open — not because the
// breaker was broken, but because the connection it was retrying against
// really was still dead. See docs/adr/0006-*.md.
type RabbitMQPublisher struct {
	url       string
	queueName string

	mu      sync.Mutex
	conn    *amqp.Connection
	channel *amqp.Channel
}

func NewRabbitMQPublisher(url string, queueName string) (*RabbitMQPublisher, error) {
	p := &RabbitMQPublisher{url: url, queueName: queueName}
	if err := p.connect(); err != nil {
		return nil, err
	}
	return p, nil
}

// connect dials and declares the queue. Caller must hold p.mu.
func (p *RabbitMQPublisher) connect() error {
	conn, err := amqp.Dial(p.url)
	if err != nil {
		return err
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return err
	}

	if _, err := channel.QueueDeclare(p.queueName, true, false, false, false, nil); err != nil {
		channel.Close()
		conn.Close()
		return err
	}

	p.conn = conn
	p.channel = channel
	return nil
}

// ensureConnected reconnects if the current connection/channel died since
// the last call. Cheap when already healthy: IsClosed() is a non-blocking
// read of a flag amqp091-go maintains internally, not a network call.
func (p *RabbitMQPublisher) ensureConnected() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn != nil && !p.conn.IsClosed() && p.channel != nil && !p.channel.IsClosed() {
		return nil
	}
	if p.conn != nil {
		p.conn.Close() // best-effort; already dead in every case that gets here
	}
	return p.connect()
}

func (p *RabbitMQPublisher) Publish(ctx context.Context, message shared.MoviePublisherMessage) error {
	ctx, span := tracer.Start(ctx, "movies_queue publish", trace.WithSpanKind(trace.SpanKindProducer))
	defer span.End()

	body, err := json.Marshal(message)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "falha ao serializar a mensagem")
		return err
	}

	if err := p.ensureConnected(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "falha ao conectar no RabbitMQ")
		return err
	}

	p.mu.Lock()
	channel := p.channel
	p.mu.Unlock()

	// Headers is where the trace context rides along — there's no
	// request/response to attach it to like there is for HTTP or gRPC, so
	// movies-service's consumer has to explicitly extract this on the
	// other end (movies-service/cmd/main.go) for the two spans to join up
	// into one trace in Jaeger instead of two disconnected ones.
	headers := amqp.Table{}
	otel.GetTextMapPropagator().Inject(ctx, shared.AMQPHeaderCarrier(headers))

	err = channel.Publish(
		"",
		p.queueName,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
			Headers:     headers,
		},
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "publish falhou")
	}
	return err
}
