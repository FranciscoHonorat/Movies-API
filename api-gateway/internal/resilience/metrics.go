package resilience

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sony/gobreaker/v2"
)

var (
	breakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "circuit_breaker_state",
			Help: "Estado atual do circuit breaker: 0=closed, 1=half-open, 2=open.",
		},
		[]string{"breaker"},
	)

	breakerTripsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "circuit_breaker_trips_total",
			Help: "Quantas vezes um circuit breaker abriu (foi de closed/half-open para open).",
		},
		[]string{"breaker"},
	)
)

func init() {
	prometheus.MustRegister(breakerState, breakerTripsTotal)
}

// observeStateChange is passed as gobreaker.Settings.OnStateChange for
// every breaker in this package, so /metrics always reflects live breaker
// state — the same endpoint api-gateway's HTTP metrics already use (see
// internal/observability).
func observeStateChange(name string, _, to gobreaker.State) {
	breakerState.WithLabelValues(name).Set(float64(to))
	if to == gobreaker.StateOpen {
		breakerTripsTotal.WithLabelValues(name).Inc()
	}
}
