package observability

import (
	"context"
	"log/slog"
	"time"

	"github.com/shahriyar/arvan/internal/repository"
)

type OutboxMetricsReporter struct {
	outbox   *repository.OutboxRepository
	interval time.Duration
}

func NewOutboxMetricsReporter(outbox *repository.OutboxRepository, interval time.Duration) *OutboxMetricsReporter {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	return &OutboxMetricsReporter{outbox: outbox, interval: interval}
}

func (r *OutboxMetricsReporter) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.reportOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reportOnce(ctx)
		}
	}
}

func (r *OutboxMetricsReporter) reportOnce(ctx context.Context) {
	count, err := r.outbox.CountPending(ctx)
	if err != nil {
		slog.WarnContext(ctx, "outbox pending gauge update failed", "error", err)
		return
	}
	SetOutboxPending(float64(count))
}
