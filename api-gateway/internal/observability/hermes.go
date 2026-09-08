package observability

import (
	"strconv"
	"time"

	hermes "github.com/FranciscoHonorat/hermes-observability/packages/agent-go"
	"github.com/gin-gonic/gin"
)

// HermesGinMiddleware records http_requests_total / http_request_duration_ms
// / http_errors_total to Hermes, alongside (not instead of) GinMiddleware's
// Prometheus metrics above — Prometheus/Grafana keep working exactly as
// they do today. Uses c.FullPath() for the same reason GinMiddleware does:
// bounded cardinality (one series per route pattern, not per resolved ID).
//
// Metrics only, deliberately: tracing already goes to Jaeger via otelgin
// (see main.go and tracing.go), so this doesn't call client.StartSpan —
// running two tracing systems side by side would just be redundant spans
// with nothing extra to show for it.
func HermesGinMiddleware(client *hermes.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}

		status := c.Writer.Status()
		duration := float64(time.Since(start)) / float64(time.Millisecond)
		labels := map[string]any{"method": c.Request.Method, "path": path, "status": strconv.Itoa(status)}

		client.Increment("http_requests_total", 1, labels)
		client.Histogram("http_request_duration_ms", duration, hermes.UnitMilliseconds, labels)
		if status >= 400 {
			client.Increment("http_errors_total", 1, labels)
		}
	}
}
