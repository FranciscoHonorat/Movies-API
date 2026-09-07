// Package resilience wraps the two network calls api-gateway makes to
// other processes it doesn't control — the gRPC client to movies-service,
// and the RabbitMQ publisher — with retry-with-backoff and a circuit
// breaker, so a slow or down dependency degrades this service's latency
// instead of piling up blocked goroutines/connections waiting on it.
package resilience

import (
	"context"
	"math/rand/v2"
	"time"
)

// RetryPolicy controls how many times, and how long between attempts,
// Retry re-tries a retryable failure.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// DefaultRetryPolicy is deliberately short: this runs inside an HTTP
// request handler, so retrying for seconds would just make api-gateway's
// own callers time out instead. 3 attempts with backoff capped at 400ms
// adds at most ~450ms of worst-case latency to a request that would
// otherwise fail outright.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 3, BaseDelay: 50 * time.Millisecond, MaxDelay: 400 * time.Millisecond}
}

// NoRetryPolicy runs fn exactly once — for operations where retrying a
// failure isn't safe (see client.go, CreateMovie: not idempotent, unlike
// the read/delete methods).
func NoRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 1}
}

// Retry calls fn until it succeeds, isRetryable(err) says the error isn't
// worth retrying, or MaxAttempts is reached — whichever comes first.
// Delay between attempts grows exponentially (BaseDelay * 2^attempt,
// capped at MaxDelay) with full jitter (a uniform random delay between 0
// and the capped value), so that many clients retrying the same
// recovering dependency at once don't all retry in lockstep and
// re-overwhelm it the moment it comes back up.
func Retry[T any](ctx context.Context, p RetryPolicy, isRetryable func(error) bool, fn func() (T, error)) (T, error) {
	var zero, result T
	var err error

	for attempt := 0; attempt < p.MaxAttempts; attempt++ {
		result, err = fn()
		if err == nil {
			return result, nil
		}
		if !isRetryable(err) {
			return zero, err
		}
		if attempt == p.MaxAttempts-1 {
			break
		}

		select {
		case <-time.After(backoff(p, attempt)):
		case <-ctx.Done():
			return zero, ctx.Err()
		}
	}
	return zero, err
}

func backoff(p RetryPolicy, attempt int) time.Duration {
	d := p.BaseDelay * time.Duration(uint(1)<<uint(attempt))
	if d <= 0 || d > p.MaxDelay {
		d = p.MaxDelay
	}
	if d <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(d) + 1))
}
