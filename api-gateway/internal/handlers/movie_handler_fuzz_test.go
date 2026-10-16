package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/FranciscoHonorat/movies/api-gateway/internal/handlers"
	"github.com/FranciscoHonorat/movies/proto"
	"github.com/FranciscoHonorat/movies/shared"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

// nopPublisher accepts every message — used where testify's mock would
// dominate the cost being measured (fuzz/benchmark loops).
type nopPublisher struct{}

func (nopPublisher) Publish(context.Context, shared.MoviePublisherMessage) error { return nil }

// recordingClient records the ID DeleteMovie was called with.
type recordingClient struct {
	proto.MovieServiceClient
	deletedID int32
	called    bool
}

func (r *recordingClient) DeleteMovie(_ context.Context, in *proto.DeleteMovieRequest, _ ...grpc.CallOption) (*proto.DeleteMovieResponse, error) {
	r.called = true
	r.deletedID = in.Id
	return &proto.DeleteMovieResponse{}, nil
}

// FuzzCreateMovie sends arbitrary bodies to POST /movies: the handler must
// never panic and must only answer 202 (aceito) or 400 (inválido).
func FuzzCreateMovie(f *testing.F) {
	for _, seed := range []string{
		`{"title":"Inception","year":"2010"}`,
		`{"title":"","year":"2010"}`,
		`{"year":"2010"}`,
		`{"title":"x"}`,
		`{"title":123,"year":2010}`,
		`not json`,
		``,
		`{"title":"a","year":"b","extra":{"nested":[1,2,3]}}`,
	} {
		f.Add(seed)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handlers.NewMovieHandler(nil, nopPublisher{})
	r.POST("/api/v1/movies", h.CreateMovie)

	f.Fuzz(func(t *testing.T, body string) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/movies", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusAccepted && w.Code != http.StatusBadRequest {
			t.Fatalf("body %q → status %d, esperado 202 ou 400", body, w.Code)
		}
	})
}

// FuzzDeleteMovie checks that the ID forwarded to movies-service is the
// same number the client sent in the path — a 2xx for an ID that does not
// fit in int32 would mean the handler deleted a different movie.
func FuzzDeleteMovie(f *testing.F) {
	for _, seed := range []string{"1", "68152", "0", "-1", "abc", "2147483647", "2147483648", "4294967297"} {
		f.Add(seed)
	}

	gin.SetMode(gin.TestMode)

	f.Fuzz(func(t *testing.T, id string) {
		client := &recordingClient{}
		r := gin.New()
		h := handlers.NewMovieHandler(client, nil)
		r.DELETE("/api/v1/movies/:id", h.DeleteMovie)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/movies/"+url.PathEscape(id), nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if !client.called {
			return
		}
		want, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			t.Fatalf("id %q não é inteiro, mas chegou ao movies-service", id)
		}
		if int64(client.deletedID) != want {
			t.Fatalf("id %q enviado ao movies-service como %d (overflow de int32)", id, client.deletedID)
		}
	})
}

func BenchmarkCreateMovieHandler(b *testing.B) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handlers.NewMovieHandler(nil, nopPublisher{})
	r.POST("/api/v1/movies", h.CreateMovie)
	body := `{"title":"Inception","year":"2010"}`

	b.ReportAllocs()
	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/movies", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
}
