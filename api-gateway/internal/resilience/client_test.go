package resilience_test

import (
	"context"
	"sync"
	"testing"

	"github.com/FranciscoHonorat/movies/api-gateway/internal/resilience"
	"github.com/FranciscoHonorat/movies/proto"
	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeMovieServiceClient implements proto.MovieServiceClient with a
// single controllable behavior function, so tests can script exactly how
// many times (and how) the "dependency" fails before succeeding again.
type fakeMovieServiceClient struct {
	mu    sync.Mutex
	calls int
	// behavior is called once per underlying RPC attempt (i.e. once per
	// gobreaker.Execute, not once per Client method call — a single
	// Client.GetMovie may invoke this multiple times via retry).
	behavior func(call int) error
}

func (f *fakeMovieServiceClient) call() (int, error) {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.mu.Unlock()
	return n, f.behavior(n)
}

func (f *fakeMovieServiceClient) GetMovie(context.Context, *proto.GetMovieRequest, ...grpc.CallOption) (*proto.GetMovieResponse, error) {
	_, err := f.call()
	if err != nil {
		return nil, err
	}
	return &proto.GetMovieResponse{Movie: &proto.Movie{Id: 1, Title: "Inception", Year: "2010"}}, nil
}

func (f *fakeMovieServiceClient) ListMovie(context.Context, *proto.ListMovieRequest, ...grpc.CallOption) (*proto.ListMovieResponse, error) {
	_, err := f.call()
	return &proto.ListMovieResponse{}, err
}

func (f *fakeMovieServiceClient) CreateMovie(context.Context, *proto.CreateMovieRequest, ...grpc.CallOption) (*proto.CreateMovieResponse, error) {
	_, err := f.call()
	return &proto.CreateMovieResponse{}, err
}

func (f *fakeMovieServiceClient) DeleteMovie(context.Context, *proto.DeleteMovieRequest, ...grpc.CallOption) (*proto.DeleteMovieResponse, error) {
	_, err := f.call()
	return &proto.DeleteMovieResponse{}, err
}

func (f *fakeMovieServiceClient) GetMovieStatus(context.Context, *proto.GetMovieStatusRequest, ...grpc.CallOption) (*proto.GetMovieStatusResponse, error) {
	_, err := f.call()
	return &proto.GetMovieStatusResponse{}, err
}

func (f *fakeMovieServiceClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func unavailable() error { return status.Error(codes.Unavailable, "movies-service indisponível") }

func TestClient_RetriesTransientFailureThenSucceeds(t *testing.T) {
	fake := &fakeMovieServiceClient{behavior: func(call int) error {
		if call < 3 {
			return unavailable()
		}
		return nil
	}}
	client := resilience.NewMovieServiceClient(fake)

	resp, err := client.GetMovie(context.Background(), &proto.GetMovieRequest{Id: 1})

	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Movie.Id)
	assert.Equal(t, 3, fake.callCount(), "deveria ter tentado 3 vezes até suceder")
}

func TestClient_DoesNotRetryNotFound(t *testing.T) {
	fake := &fakeMovieServiceClient{behavior: func(int) error {
		return status.Error(codes.NotFound, "filme não encontrado")
	}}
	client := resilience.NewMovieServiceClient(fake)

	_, err := client.GetMovie(context.Background(), &proto.GetMovieRequest{Id: 999})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Equal(t, 1, fake.callCount(), "NotFound não deveria gerar retry")
}

// TestClient_BreakerOpensAndStopsCallingInner is the one that actually
// proves the circuit breaker does something: past the failure threshold,
// further calls must fail immediately WITHOUT reaching the fake at all —
// that's the entire point (stop hammering a dependency known to be down).
func TestClient_BreakerOpensAndStopsCallingInner(t *testing.T) {
	fake := &fakeMovieServiceClient{behavior: func(int) error { return unavailable() }}
	client := resilience.NewMovieServiceClient(fake)

	// Each outer GetMovie retries up to 3 times; ReadyToTrip fires at 5
	// consecutive failures — reached partway through the 2nd outer call.
	var lastErr error
	for i := 0; i < 2; i++ {
		_, lastErr = client.GetMovie(context.Background(), &proto.GetMovieRequest{Id: 1})
	}
	require.Error(t, lastErr)
	callsWhenTripped := fake.callCount()
	assert.LessOrEqual(t, callsWhenTripped, 5, "o breaker deveria ter cortado antes do retry esgotar todas as tentativas")

	// Now hammer it — the fake must NOT see any of these calls; the
	// breaker itself should reject them.
	for i := 0; i < 10; i++ {
		_, err := client.GetMovie(context.Background(), &proto.GetMovieRequest{Id: 1})
		require.ErrorIs(t, err, gobreaker.ErrOpenState)
	}
	assert.Equal(t, callsWhenTripped, fake.callCount(), "com o breaker aberto, o cliente interno não deveria ser chamado de novo")
}

func TestClient_CreateMovieDoesNotRetry(t *testing.T) {
	fake := &fakeMovieServiceClient{behavior: func(int) error { return unavailable() }}
	client := resilience.NewMovieServiceClient(fake)

	_, err := client.CreateMovie(context.Background(), &proto.CreateMovieRequest{Title: "Tenet", Year: "2020"})

	require.Error(t, err)
	assert.Equal(t, 1, fake.callCount(), "CreateMovie não é idempotente — não deveria retentar automaticamente")
}
