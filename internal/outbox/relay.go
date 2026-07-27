package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/broker"
	"github.com/shahriyar/arvan/internal/config"
	"github.com/shahriyar/arvan/internal/domain"
	"github.com/shahriyar/arvan/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const markPublishedMaxAttempts = 5

type Publisher interface {
	Publish(ctx context.Context, queue string, body []byte) error
}

type OutboxStore interface {
	ClaimPendingBatch(ctx context.Context, limit int, lockDuration time.Duration) ([]domain.OutboxEventRecord, error)
	IsPublished(ctx context.Context, eventID uuid.UUID) (bool, error)
	MarkPublished(ctx context.Context, eventID uuid.UUID) error
	RecordPublishFailure(ctx context.Context, eventID uuid.UUID) error
}

type publishEventError struct {
	phase string
	err   error
}

func (e *publishEventError) Error() string {
	return fmt.Sprintf("outbox %s failed: %v", e.phase, e.err)
}

func (e *publishEventError) Unwrap() error {
	return e.err
}

type Relay struct {
	outbox    OutboxStore
	publisher Publisher
	cfg       config.OutboxRelayConfig

	wg     sync.WaitGroup
	stopCh chan struct{}
}

func NewRelay(outbox OutboxStore, publisher Publisher, cfg config.OutboxRelayConfig) *Relay {
	return &Relay{
		outbox:    outbox,
		publisher: publisher,
		cfg:       cfg,
		stopCh:    make(chan struct{}),
	}
}

func (r *Relay) Start(ctx context.Context) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.loop(ctx)
	}()
}

func (r *Relay) loop(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		if err := r.pollOnce(ctx); err != nil {
			slog.ErrorContext(ctx, "outbox relay poll failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
		}
	}
}

func (r *Relay) pollOnce(ctx context.Context) error {
	events, err := r.outbox.ClaimPendingBatch(ctx, r.cfg.BatchSize, r.cfg.LockDuration)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := r.publishEvent(ctx, event); err != nil {
			var phaseErr *publishEventError
			if errors.As(err, &phaseErr) && phaseErr.phase == "mark" {
				slog.ErrorContext(ctx, "mark outbox published failed after successful publish; keeping lock for retry",
					"event_id", event.ID,
					"aggregate_id", event.AggregateID,
					"error", err,
				)
				continue
			}

			slog.ErrorContext(ctx, "publish outbox event failed",
				"event_id", event.ID,
				"aggregate_id", event.AggregateID,
				"error", err,
			)
			if recordErr := r.outbox.RecordPublishFailure(ctx, event.ID); recordErr != nil {
				slog.ErrorContext(ctx, "record outbox publish failure", "event_id", event.ID, "error", recordErr)
			}
		}
	}
	return nil
}

func (r *Relay) publishEvent(ctx context.Context, event domain.OutboxEventRecord) error {
	published, err := r.outbox.IsPublished(ctx, event.ID)
	if err != nil {
		return &publishEventError{phase: "check", err: err}
	}
	if published {
		return nil
	}

	var payload domain.SMSSendPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return &publishEventError{phase: "decode", err: err}
	}

	publishCtx := observability.ExtractMap(ctx, payload.TraceContext)
	publishCtx, span := observability.StartSpan(publishCtx, "outbox.publish",
		trace.WithAttributes(
			attribute.String("event_id", event.ID.String()),
			attribute.String("message_id", payload.MessageID),
			attribute.String("queue", broker.QueueForMessageType(payload.MessageType)),
		),
	)
	defer span.End()

	queue := broker.QueueForMessageType(payload.MessageType)
	if err := r.publisher.Publish(publishCtx, queue, event.Payload); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		observability.RecordOutboxPublishError()
		return &publishEventError{phase: "publish", err: err}
	}

	if err := r.markPublishedWithRetry(ctx, event.ID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return &publishEventError{phase: "mark", err: err}
	}

	return nil
}

func (r *Relay) markPublishedWithRetry(ctx context.Context, eventID uuid.UUID) error {
	var lastErr error
	for attempt := 0; attempt < markPublishedMaxAttempts; attempt++ {
		if err := r.outbox.MarkPublished(ctx, eventID); err == nil {
			return nil
		} else {
			lastErr = err
			published, checkErr := r.outbox.IsPublished(ctx, eventID)
			if checkErr == nil && published {
				return nil
			}
		}
		if attempt < markPublishedMaxAttempts-1 {
			time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
		}
	}
	return lastErr
}

func (r *Relay) Stop() {
	close(r.stopCh)
}

func (r *Relay) Wait() {
	r.wg.Wait()
}

func QueueForPayload(payload domain.SMSSendPayload) string {
	return broker.QueueForMessageType(payload.MessageType)
}
