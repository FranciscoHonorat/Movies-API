package resilience_test

import (
	"testing"

	"github.com/FranciscoHonorat/movies/api-gateway/internal/resilience"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsRetryableGRPC(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil não é retryable (não é erro)", nil, false},
		{"Unavailable é retryable", status.Error(codes.Unavailable, "down"), true},
		{"DeadlineExceeded é retryable", status.Error(codes.DeadlineExceeded, "slow"), true},
		{"ResourceExhausted é retryable", status.Error(codes.ResourceExhausted, "busy"), true},
		{"Internal é retryable", status.Error(codes.Internal, "oops"), true},
		{"NotFound não é retryable", status.Error(codes.NotFound, "no"), false},
		{"InvalidArgument não é retryable", status.Error(codes.InvalidArgument, "bad"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resilience.IsRetryableGRPC(tt.err))
		})
	}
}

func TestIsSuccessfulGRPC(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil é sucesso", nil, true},
		{"NotFound conta como sucesso pro breaker", status.Error(codes.NotFound, "no"), true},
		{"InvalidArgument conta como sucesso pro breaker", status.Error(codes.InvalidArgument, "bad"), true},
		{"Unavailable conta como falha pro breaker", status.Error(codes.Unavailable, "down"), false},
		{"Internal conta como falha pro breaker", status.Error(codes.Internal, "oops"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resilience.IsSuccessfulGRPC(tt.err))
		})
	}
}
