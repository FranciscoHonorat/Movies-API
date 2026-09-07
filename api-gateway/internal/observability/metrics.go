// Package observability exposes Prometheus metrics for api-gateway.
// movies-service does not have an equivalent yet — it only serves gRPC, and
// wiring Prometheus into a gRPC server needs a separate HTTP listener that
// this change doesn't add (see docs/adr/0003, "Itens descobertos").
package observability

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total de requisições HTTP recebidas pelo api-gateway.",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duração das requisições HTTP no api-gateway.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal, httpRequestDuration)
}

// GinMiddleware records request count and latency per (method, route
// pattern, status). Uses c.FullPath() rather than the raw URL so
// /movies/123 and /movies/456 collapse into one "/movies/:id" series
// instead of creating unbounded cardinality per ID.
func GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}

		httpRequestsTotal.WithLabelValues(c.Request.Method, path, strconv.Itoa(c.Writer.Status())).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, path).Observe(time.Since(start).Seconds())
	}
}

// Handler serves the /metrics endpoint that Prometheus scrapes (see
// infra/kubernetes/monitoring/prometheus.yaml).
func Handler() gin.HandlerFunc {
	h := promhttp.Handler()
	return gin.WrapH(h)
}
