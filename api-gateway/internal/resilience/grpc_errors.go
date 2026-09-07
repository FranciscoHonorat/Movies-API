package resilience

import (
	"errors"

	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IsRetryableGRPC reports whether a gRPC error looks like a transient,
// infrastructure-side failure (movies-service overloaded, mid-restart,
// network blip) as opposed to a legitimate answer about the specific
// request. Retrying codes.NotFound or codes.InvalidArgument would just
// burn the retry budget on a request that's never going to succeed —
// and retrying gobreaker.ErrOpenState/ErrTooManyRequests would defeat the
// entire point of the breaker, hammering it with retries instead of
// failing fast the moment it trips.
func IsRetryableGRPC(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return false
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Aborted, codes.Internal, codes.Unknown:
		return true
	default:
		return false
	}
}

// IsSuccessfulGRPC tells the circuit breaker which errors should count
// against its failure budget. A NotFound or InvalidArgument means
// movies-service is answering correctly — the request was bad, not the
// dependency — so it shouldn't push the breaker toward tripping; only
// errors that actually indicate the dependency itself is unhealthy should.
func IsSuccessfulGRPC(err error) bool {
	if err == nil {
		return true
	}
	switch status.Code(err) {
	case codes.NotFound, codes.InvalidArgument:
		return true
	default:
		return false
	}
}
