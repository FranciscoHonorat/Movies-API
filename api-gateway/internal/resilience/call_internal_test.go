package resilience

import (
	"context"
	"testing"

	"github.com/FranciscoHonorat/movies/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// call é o helper genérico que vai substituir o corpo repetido dos 5
// métodos do Client: retry + circuit breaker + timeout em volta de
// qualquer RPC do proto.MovieServiceClient.
func TestCall_RepassaRequestERetentaErroTransiente(t *testing.T) {
	c := NewMovieServiceClient(nil) // o inner não é usado: a RPC vem por parâmetro

	tentativas := 0
	rpc := func(_ context.Context, in *proto.GetMovieRequest, _ ...grpc.CallOption) (*proto.GetMovieResponse, error) {
		tentativas++
		if tentativas == 1 {
			return nil, status.Error(codes.Unavailable, "movies-service fora do ar")
		}
		return &proto.GetMovieResponse{Movie: &proto.Movie{Id: in.Id}}, nil
	}

	resp, err := call(context.Background(), c, c.retry, rpc, &proto.GetMovieRequest{Id: 42})

	require.NoError(t, err)
	assert.Equal(t, int32(42), resp.GetMovie().GetId())
	assert.Equal(t, 2, tentativas, "a 1ª tentativa falha com Unavailable e deve ser retentada")
}
