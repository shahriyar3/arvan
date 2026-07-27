package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sony/gobreaker"
)

var (
	ExpressOperatorDeliverySeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "express_operator_delivery_seconds",
		Help:    "Time from worker claim to successful operator delivery for express messages",
		Buckets: prometheus.ExponentialBuckets(0.01, 2, 12),
	})

	CircuitBreakerState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "circuit_breaker_state",
		Help: "Circuit breaker state for external dependencies (0=closed, 1=half-open, 2=open)",
	}, []string{"target"})
)

func init() {
	prometheus.MustRegister(ExpressOperatorDeliverySeconds, CircuitBreakerState)
}

func RecordCircuitBreakerState(target string, state gobreaker.State) {
	CircuitBreakerState.WithLabelValues(target).Set(float64(state))
}
