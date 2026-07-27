package outbox

import (
	"context"
	"encoding/json"
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

type Publisher interface {
	Publish(ctx context.Context, queue string, body []byte) error
}

type OutboxStore interface {
	ClaimPendingBatch(ctx context.Context, limit int, lockDuration time.Duration) ([]domain.OutboxEventRecord, error)
	MarkPublished(ctx context.Context, eventID uuid.UUID) error
	RecordPublishFailure(ctx context.Context, eventID uuid.UUID) error
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
	var payload domain.SMSSendPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal outbox payload: %w", err)
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
		return err
	}

	if err := r.outbox.MarkPublished(ctx, event.ID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
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
