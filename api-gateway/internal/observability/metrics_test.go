package observability_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FranciscoHonorat/movies/api-gateway/internal/observability"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGinMiddleware_RecordsMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(observability.GinMiddleware())
	r.GET("/movies/:id", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/metrics", observability.Handler())

	req := httptest.NewRequest(http.MethodGet, "/movies/42", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	// The route pattern, not the raw "/movies/42", must appear — otherwise
	// every distinct movie ID would create its own metric series
	// (unbounded cardinality).
	assert.True(t, strings.Contains(body, `path="/movies/:id"`), "esperado label path=\"/movies/:id\" em:\n%s", body)
	assert.False(t, strings.Contains(body, `path="/movies/42"`), "cardinalidade por ID não deveria aparecer nas métricas")
}
