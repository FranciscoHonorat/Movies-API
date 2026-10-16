package resilience_test

import (
	"context"
	"testing"

	"github.com/FranciscoHonorat/movies/api-gateway/internal/resilience"
	"github.com/FranciscoHonorat/movies/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func BenchmarkRetry_SuccessFirstTry(b *testing.B) {
	ctx := context.Background()
	policy := resilience.DefaultRetryPolicy()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = resilience.Retry(ctx, policy, alwaysRetryable, func() (int, error) { return 1, nil })
	}
}

func BenchmarkIsRetryableGRPC(b *testing.B) {
	err := status.Error(codes.Unavailable, "down")
	for b.Loop() {
		_ = resilience.IsRetryableGRPC(err)
	}
}

// BenchmarkClient_GetMovie measures the overhead the circuit breaker +
// retry wrapper adds on top of a healthy dependency.
func BenchmarkClient_GetMovie(b *testing.B) {
	client := resilience.NewMovieServiceClient(&fakeMovieServiceClient{behavior: func(int) error { return nil }})
	ctx := context.Background()
	req := &proto.GetMovieRequest{Id: 1}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = client.GetMovie(ctx, req)
	}
}
