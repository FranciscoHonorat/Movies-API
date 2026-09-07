package resilience

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/FranciscoHonorat/movies/api-gateway/internal/ports/output"
	"github.com/FranciscoHonorat/movies/shared"
	"github.com/sony/gobreaker/v2"
)

// Publisher wraps an output.MoviePublisher with the same retry+breaker
// treatment as Client, for the same reason: a RabbitMQ blip shouldn't
// turn into every CreateMovie request blocking on a dead connection.
//
// Retrying a publish isn't perfectly safe: without publisher confirms
// (the underlying RabbitMQPublisher doesn't use them), a "failed" publish
// might have actually reached the broker before the connection dropped,
// so a retry could occasionally enqueue the same movie twice under the
// same correlation_id. Accepted trade-off, not fixed here — see
// docs/adr/0006-*.md.
type Publisher struct {
	inner       output.MoviePublisher
	breaker     *gobreaker.CircuitBreaker[any]
	retry       RetryPolicy
	callTimeout time.Duration
}

func NewPublisher(inner output.MoviePublisher) *Publisher {
	return &Publisher{
		inner: inner,
		// Unlike gRPC, a publish error has no "the broker answered
		// correctly, this specific message just wasn't found" case —
		// every error here is a sign of trouble with the broker itself.
		breaker:     newBreaker("rabbitmq-publish", func(err error) bool { return err == nil }),
		retry:       DefaultRetryPolicy(),
		callTimeout: 500 * time.Millisecond,
	}
}

func (p *Publisher) Publish(ctx context.Context, movie shared.MoviePublisherMessage) error {
	_, err := Retry(ctx, p.retry, isRetryablePublish, func() (struct{}, error) {
		return executeBreaker(ctx, p.breaker, p.callTimeout, func(attemptCtx context.Context) (struct{}, error) {
			// amqp091-go's Publish doesn't take/respect a context, so
			// callTimeout here only bounds how long Retry waits before
			// moving on — it can't abort an already-in-flight Publish
			// call the way it aborts a gRPC call. Measured against a
			// stopped RabbitMQ broker: Publish over a connection that's
			// already known-closed fails in <1ms (the amqp091-go client
			// notices the closed connection immediately), so this hasn't
			// shown the ~20s-hang problem the gRPC client had — see
			// docs/adr/0006-*.md for the actual measurement, not just
			// this assumption.
			return struct{}{}, p.inner.Publish(attemptCtx, movie)
		})
	})
	if err != nil {
		slog.Error("publish em movies_queue falhou após retries", "error", err)
	}
	return err
}

// isRetryablePublish: RabbitMQPublisher's Publish only fails on
// channel/connection-level errors (JSON marshal of a small known struct
// never fails) — every failure here is exactly the transient, infra-side
// kind retry exists for, except the breaker's own "don't bother" errors,
// which retrying would defeat the purpose of.
func isRetryablePublish(err error) bool {
	if err == nil {
		return false
	}
	return !errors.Is(err, gobreaker.ErrOpenState) && !errors.Is(err, gobreaker.ErrTooManyRequests)
}
