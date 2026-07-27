package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sony/gobreaker"
)

var (
	SMSAcceptTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sms_accept_total",
		Help: "Total SMS accept requests by HTTP status code",
	}, []string{"status"})

	SMSAcceptDurationSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "sms_accept_duration_seconds",
		Help:    "Latency of POST /v1/sms/send handler",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 14),
	})

	BalanceDeductErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "balance_deduct_errors_total",
		Help: "Balance deduction failures (insufficient balance or deduct errors)",
	})

	OutboxPendingGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "outbox_pending_gauge",
		Help: "Number of unpublished outbox events",
	})

	OutboxPublishErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "outbox_publish_errors_total",
		Help: "Outbox relay failures to publish events to RabbitMQ",
	})

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
	prometheus.MustRegister(
		SMSAcceptTotal,
		SMSAcceptDurationSeconds,
		BalanceDeductErrorsTotal,
		OutboxPendingGauge,
		OutboxPublishErrorsTotal,
		ExpressOperatorDeliverySeconds,
		CircuitBreakerState,
	)
}

func RecordCircuitBreakerState(target string, state gobreaker.State) {
	CircuitBreakerState.WithLabelValues(target).Set(float64(state))
}

func RecordSMSAccept(status string, durationSeconds float64) {
	SMSAcceptTotal.WithLabelValues(status).Inc()
	SMSAcceptDurationSeconds.Observe(durationSeconds)
}

func RecordBalanceDeductError() {
	BalanceDeductErrorsTotal.Inc()
}

func SetOutboxPending(count float64) {
	OutboxPendingGauge.Set(count)
}

func RecordOutboxPublishError() {
	OutboxPublishErrorsTotal.Inc()
}
