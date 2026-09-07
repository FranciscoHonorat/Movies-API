package handlers_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FranciscoHonorat/movies/api-gateway/internal/handlers"
	"github.com/FranciscoHonorat/movies/shared"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockMoviePublisher struct {
	mock.Mock
}

func (m *MockMoviePublisher) Publish(ctx context.Context, movie shared.MoviePublisherMessage) error {
	args := m.Called(ctx, movie)
	return args.Error(0)
}

func newTestRouter(publisher *MockMoviePublisher) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	movieHandler := handlers.NewMovieHandler(nil, publisher)
	r.POST("/api/v1/movies", movieHandler.CreateMovie)
	return r
}

func doCreateMovieRequest(t *testing.T, r *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/movies", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCreateMovie_ValidPayloadDoesNotPanic(t *testing.T) {
	publisher := new(MockMoviePublisher)
	publisher.On("Publish", mock.Anything, mock.MatchedBy(func(msg shared.MoviePublisherMessage) bool {
		return msg.Title == "Inception" && msg.Year == "2010" && msg.CorrelationID != ""
	})).Return(nil)

	r := newTestRouter(publisher)

	w := doCreateMovieRequest(t, r, `{"title":"Inception","year":"2010"}`)

	require.Equal(t, http.StatusAccepted, w.Code)
	assert.Contains(t, w.Body.String(), "correlation_id")
	publisher.AssertExpectations(t)
}

func TestCreateMovie_MissingTitleIsRejected(t *testing.T) {
	publisher := new(MockMoviePublisher)
	r := newTestRouter(publisher)

	w := doCreateMovieRequest(t, r, `{"year":"2010"}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	publisher.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything)
}

func TestCreateMovie_TitleExceedingMaxLengthIsRejected(t *testing.T) {
	publisher := new(MockMoviePublisher)
	r := newTestRouter(publisher)

	longTitle := strings.Repeat("a", 301)
	w := doCreateMovieRequest(t, r, `{"title":"`+longTitle+`","year":"2010"}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	publisher.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything)
}

func TestCreateMovie_PublisherErrorReturns500(t *testing.T) {
	publisher := new(MockMoviePublisher)
	publisher.On("Publish", mock.Anything, mock.Anything).Return(assert.AnError)

	r := newTestRouter(publisher)

	w := doCreateMovieRequest(t, r, `{"title":"Inception","year":"2010"}`)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
