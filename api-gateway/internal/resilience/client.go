package resilience

import (
	"context"
	"log/slog"
	"time"

	"github.com/FranciscoHonorat/movies/proto"
	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"
)

// Client wraps a proto.MovieServiceClient with a circuit breaker and
// retry-with-backoff, so a slow or down movies-service degrades
// api-gateway's own latency gracefully (fail fast once the breaker trips)
// instead of every request piling up waiting on a dependency that's
// already known to be unhealthy. Implements proto.MovieServiceClient, so
// it's a drop-in replacement wherever that interface is used — see
// cmd/main.go.
type Client struct {
	inner   proto.MovieServiceClient
	breaker *gobreaker.CircuitBreaker[any]
	retry   RetryPolicy
	// createRetry is intentionally NOT retried by default: unlike
	// GetMovie/ListMovie/DeleteMovie/GetMovieStatus, CreateMovie isn't
	// idempotent — movies-service mints a new ID on every call, so
	// retrying a CreateMovie whose response was merely lost (not the
	// request) would create two movies. api-gateway's own CreateMovie
	// HTTP handler doesn't actually call this RPC (it publishes to
	// RabbitMQ instead, see ResilientPublisher) — this only matters if
	// this gRPC method is ever called directly by something else.
	createRetry RetryPolicy
	// callTimeout bounds a single attempt. Without this, a single failed
	// attempt against a movies-service that's down (not erroring — just
	// unreachable) can block for ~20s: grpc.NewClient connects lazily and
	// retries the connection internally with its own backoff, and none of
	// that shows up as an error our Retry/breaker layer can see until gRPC
	// itself gives up. Measured directly: with no timeout, a request
	// against a stopped movies-service took 20.05s before this package's
	// retry logic ever got a chance to run. Retry-with-backoff and a
	// circuit breaker are pointless if a single attempt can hang that
	// long — this is what actually makes both effective.
	callTimeout time.Duration
}

// newBreaker builds a breaker with the settings shared by every dependency
// this package protects; isSuccessful is the one thing that's genuinely
// dependency-specific (what counts as "the dependency is fine, this
// particular request just didn't work out" varies by protocol — see
// IsSuccessfulGRPC vs. the plain "any error is a failure" used for the
// RabbitMQ publisher, which has no equivalent to a gRPC NotFound).
func newBreaker(name string, isSuccessful func(error) bool) *gobreaker.CircuitBreaker[any] {
	return gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name: name,
		// One trial request while half-open: if it succeeds we go back to
		// closed, if it fails we reopen — no reason to let more than one
		// request through to test a dependency we just decided was down.
		MaxRequests: 1,
		// Reset the closed-state failure counter every 10s, so a handful
		// of failures hours apart don't add up to a trip.
		Interval: 10 * time.Second,
		// Stay open for 5s before allowing a half-open trial — long
		// enough that a brief blip (a pod restart, a GC pause) doesn't
		// bounce the breaker open/closed/open, short enough that a real
		// recovery is noticed quickly.
		Timeout: 5 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
		IsSuccessful: isSuccessful,
		OnStateChange: func(name string, from, to gobreaker.State) {
			slog.Warn("circuit breaker mudou de estado", "breaker", name, "de", from.String(), "para", to.String())
			observeStateChange(name, from, to)
		},
	})
}

func NewMovieServiceClient(inner proto.MovieServiceClient) *Client {
	return &Client{
		inner:       inner,
		breaker:     newBreaker("movies-service-grpc", IsSuccessfulGRPC),
		retry:       DefaultRetryPolicy(),
		createRetry: NoRetryPolicy(),
		callTimeout: 500 * time.Millisecond,
	}
}

// executeBreaker bridges the any-typed breaker (one breaker instance is
// shared across all 5 RPC methods below, each with a different response
// type — Go generics don't let a single CircuitBreaker[T] value serve
// more than one T) back to the concrete response type each method needs.
// It also applies callTimeout to this one attempt — see the field doc on
// Client for why that's load-bearing, not just tidiness.
func executeBreaker[T any](ctx context.Context, cb *gobreaker.CircuitBreaker[any], timeout time.Duration, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	r, err := cb.Execute(func() (any, error) { return fn(attemptCtx) })
	if err != nil {
		return zero, err
	}
	v, _ := r.(T)
	return v, nil
}

func (c *Client) GetMovie(ctx context.Context, in *proto.GetMovieRequest, opts ...grpc.CallOption) (*proto.GetMovieResponse, error) {
	return Retry(ctx, c.retry, IsRetryableGRPC, func() (*proto.GetMovieResponse, error) {
		return executeBreaker(ctx, c.breaker, c.callTimeout, func(attemptCtx context.Context) (*proto.GetMovieResponse, error) {
			return c.inner.GetMovie(attemptCtx, in, opts...)
		})
	})
}

func (c *Client) ListMovie(ctx context.Context, in *proto.ListMovieRequest, opts ...grpc.CallOption) (*proto.ListMovieResponse, error) {
	return Retry(ctx, c.retry, IsRetryableGRPC, func() (*proto.ListMovieResponse, error) {
		return executeBreaker(ctx, c.breaker, c.callTimeout, func(attemptCtx context.Context) (*proto.ListMovieResponse, error) {
			return c.inner.ListMovie(attemptCtx, in, opts...)
		})
	})
}

func (c *Client) DeleteMovie(ctx context.Context, in *proto.DeleteMovieRequest, opts ...grpc.CallOption) (*proto.DeleteMovieResponse, error) {
	return Retry(ctx, c.retry, IsRetryableGRPC, func() (*proto.DeleteMovieResponse, error) {
		return executeBreaker(ctx, c.breaker, c.callTimeout, func(attemptCtx context.Context) (*proto.DeleteMovieResponse, error) {
			return c.inner.DeleteMovie(attemptCtx, in, opts...)
		})
	})
}

func (c *Client) GetMovieStatus(ctx context.Context, in *proto.GetMovieStatusRequest, opts ...grpc.CallOption) (*proto.GetMovieStatusResponse, error) {
	return Retry(ctx, c.retry, IsRetryableGRPC, func() (*proto.GetMovieStatusResponse, error) {
		return executeBreaker(ctx, c.breaker, c.callTimeout, func(attemptCtx context.Context) (*proto.GetMovieStatusResponse, error) {
			return c.inner.GetMovieStatus(attemptCtx, in, opts...)
		})
	})
}

// CreateMovie uses createRetry (no retries) — see the field doc on Client.
func (c *Client) CreateMovie(ctx context.Context, in *proto.CreateMovieRequest, opts ...grpc.CallOption) (*proto.CreateMovieResponse, error) {
	return Retry(ctx, c.createRetry, IsRetryableGRPC, func() (*proto.CreateMovieResponse, error) {
		return executeBreaker(ctx, c.breaker, c.callTimeout, func(attemptCtx context.Context) (*proto.CreateMovieResponse, error) {
			return c.inner.CreateMovie(attemptCtx, in, opts...)
		})
	})
}
