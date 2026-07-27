package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/config"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/shahriyar/arvan/internal/observability"
	"github.com/shahriyar/arvan/internal/operator"
	"github.com/shahriyar/arvan/internal/repository"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

type Consumer interface {
	Consume(queue string, prefetch int) (<-chan amqp.Delivery, error)
}

type DLQPublisher interface {
	PublishToDLQ(ctx context.Context, body []byte) error
}

type Processor struct {
	db        *gorm.DB
	sms       *repository.SMSRepository
	processed *repository.ProcessedConsumerRepository
	operator  operator.SMSOperator
	cfg       config.WorkerConfig
	bulkhead  *Bulkhead
	dlq       DLQPublisher
	inFlight  sync.WaitGroup
}

type deliveryClaim int

const (
	claimAcquired deliveryClaim = iota
	claimAlreadyDone
	claimInProgress
	claimResume
)

func NewProcessor(
	db *gorm.DB,
	sms *repository.SMSRepository,
	processed *repository.ProcessedConsumerRepository,
	op operator.SMSOperator,
	cfg config.WorkerConfig,
) *Processor {
	return &Processor{
		db:        db,
		sms:       sms,
		processed: processed,
		operator:  op,
		cfg:       cfg,
		bulkhead:  NewBulkhead(cfg.ExpressPoolSize, cfg.StandardPoolSize),
	}
}

func (p *Processor) SetDLQPublisher(publisher DLQPublisher) {
	p.dlq = publisher
}

func (p *Processor) RunConsumer(ctx context.Context, consumer Consumer, queue string) error {
	deliveries, err := consumer.Consume(queue, p.cfg.PrefetchCount)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return nil
			}
			p.inFlight.Add(1)
			go func(d amqp.Delivery, q string) {
				defer p.inFlight.Done()
				if err := p.bulkhead.Acquire(ctx, q); err != nil {
					if nackErr := d.Nack(false, true); nackErr != nil {
						slog.Error("nack delivery on bulkhead acquire cancel", "error", nackErr)
					}
					return
				}
				defer p.bulkhead.Release(q)
				p.handleDelivery(ctx, q, d)
			}(delivery, queue)
		}
	}
}

func (p *Processor) Wait() {
	p.inFlight.Wait()
}

func (p *Processor) handleDelivery(ctx context.Context, queue string, delivery amqp.Delivery) {
	requeue := false
	defer func() {
		if requeue {
			if err := delivery.Nack(false, true); err != nil {
				slog.ErrorContext(ctx, "nack delivery", "error", err)
			}
			return
		}
		if err := delivery.Ack(false); err != nil {
			slog.ErrorContext(ctx, "ack delivery", "error", err)
		}
	}()

	var payload domain.SMSSendPayload
	if err := json.Unmarshal(delivery.Body, &payload); err != nil {
		slog.Error("invalid queue payload", "error", err)
		return
	}

	traceCtx := observability.ExtractMap(ctx, payload.TraceContext)
	traceCtx = observability.ExtractAMQP(traceCtx, delivery.Headers)
	traceCtx, span := observability.StartSpan(traceCtx, "worker.process",
		trace.WithAttributes(
			attribute.String("queue", queue),
			attribute.String("message_id", payload.MessageID),
		),
	)
	defer span.End()
	ctx = traceCtx

	messageID, err := uuid.Parse(payload.MessageID)
	if err != nil {
		slog.ErrorContext(ctx, "invalid message_id in payload", "error", err)
		return
	}
	accountID, err := uuid.Parse(payload.AccountID)
	if err != nil {
		slog.ErrorContext(ctx, "invalid account_id in payload", "error", err)
		return
	}

	if p.shouldDeadLetter(delivery) {
		if err := p.deadLetter(ctx, accountID, messageID, delivery.Body); err != nil {
			slog.ErrorContext(ctx, "dead letter delivery failed", "message_id", messageID, "error", err)
			requeue = true
		}
		return
	}

	claim, err := p.claimDelivery(ctx, accountID, messageID, delivery.Redelivered)
	if err != nil {
		slog.ErrorContext(ctx, "claim delivery failed", "message_id", messageID, "error", err)
		requeue = true
		return
	}
	switch claim {
	case claimAlreadyDone:
		slog.InfoContext(ctx, "skip duplicate delivery", "message_id", messageID)
		return
	case claimInProgress:
		requeue = true
		return
	}

	start := time.Now()
	if claim != claimResume {
		opCtx, opSpan := observability.StartSpan(ctx, "operator.send",
			trace.WithAttributes(attribute.String("message_id", payload.MessageID)),
		)
		_, err = p.operator.Send(opCtx, operator.SendRequest{
			MessageID: payload.MessageID,
			AccountID: payload.AccountID,
			To:        payload.To,
			Body:      payload.Body,
		})
		opSpan.End()
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		if operator.IsPermanent(err) {
			if markErr := p.markFailed(ctx, accountID, messageID); markErr != nil {
				slog.ErrorContext(ctx, "mark sms failed", "message_id", messageID, "error", markErr)
				requeue = true
				return
			}
			slog.WarnContext(ctx, "operator permanently rejected sms", "message_id", messageID, "error", err)
			return
		}

		if releaseErr := p.releaseClaim(ctx, messageID); releaseErr != nil {
			slog.ErrorContext(ctx, "release delivery claim", "message_id", messageID, "error", releaseErr)
		}
		slog.WarnContext(ctx, "operator send failed", "message_id", messageID, "error", err)
		requeue = true
		return
	}

	if err := p.markSent(ctx, accountID, messageID); err != nil {
		slog.ErrorContext(ctx, "mark sms sent failed", "message_id", messageID, "error", err)
		requeue = true
		return
	}

	if payload.MessageType == domain.MessageTypeExpress {
		observability.ExpressOperatorDeliverySeconds.Observe(time.Since(start).Seconds())
	}
}

func (p *Processor) shouldDeadLetter(delivery amqp.Delivery) bool {
	if p.cfg.MaxDeliveryAttempts <= 0 {
		return false
	}
	return deliveryAttemptCount(delivery) >= p.cfg.MaxDeliveryAttempts
}

func (p *Processor) deadLetter(ctx context.Context, accountID, messageID uuid.UUID, body []byte) error {
	err := p.db.Clauses(dbresolver.Write).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updated, err := p.sms.MarkDeadLetteredIfAccepted(ctx, tx, accountID, messageID)
		if err != nil {
			return err
		}
		if !updated {
			slog.WarnContext(ctx, "sms status not updated to dead_lettered",
				"message_id", messageID,
				"account_id", accountID,
			)
		}
		_, err = p.processed.MarkProcessedIfNew(ctx, tx, messageID)
		return err
	})
	if err != nil {
		return err
	}

	if p.dlq != nil {
		if err := p.dlq.PublishToDLQ(ctx, body); err != nil {
			return fmt.Errorf("publish dlq: %w", err)
		}
	}
	return nil
}

func (p *Processor) claimDelivery(ctx context.Context, accountID, messageID uuid.UUID, redelivered bool) (deliveryClaim, error) {
	var claim deliveryClaim
	err := p.db.Clauses(dbresolver.Write).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		msg, err := p.sms.GetByAccountAndIDTx(ctx, tx, accountID, messageID)
		if err != nil {
			if errors.Is(err, domainerrors.ErrNotFound) {
				claim = claimAlreadyDone
				return nil
			}
			return err
		}

		switch msg.Status {
		case domain.SMSStatusSent, domain.SMSStatusFailed, domain.SMSStatusDeadLettered:
			claim = claimAlreadyDone
			return nil
		}

		inserted, err := p.processed.MarkProcessedIfNew(ctx, tx, messageID)
		if err != nil {
			return err
		}
		if inserted {
			claim = claimAcquired
			return nil
		}

		msg, err = p.sms.GetByAccountAndIDTx(ctx, tx, accountID, messageID)
		if err != nil {
			if errors.Is(err, domainerrors.ErrNotFound) {
				claim = claimAlreadyDone
				return nil
			}
			return err
		}
		if msg.Status == domain.SMSStatusSent || msg.Status == domain.SMSStatusFailed || msg.Status == domain.SMSStatusDeadLettered {
			claim = claimAlreadyDone
			return nil
		}

		if redelivered {
			claim = claimResume
		} else {
			claim = claimInProgress
		}
		return nil
	})
	if err != nil {
		return claimInProgress, err
	}
	return claim, nil
}

func (p *Processor) releaseClaim(ctx context.Context, messageID uuid.UUID) error {
	return p.processed.DeleteClaim(ctx, nil, messageID)
}

func (p *Processor) markSent(ctx context.Context, accountID, messageID uuid.UUID) error {
	return p.db.Clauses(dbresolver.Write).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updated, err := p.sms.MarkSentIfAccepted(ctx, tx, accountID, messageID)
		if err != nil {
			return err
		}
		if !updated {
			slog.WarnContext(ctx, "sms status not updated to sent",
				"message_id", messageID,
				"account_id", accountID,
			)
		}
		return nil
	})
}

func (p *Processor) markFailed(ctx context.Context, accountID, messageID uuid.UUID) error {
	return p.db.Clauses(dbresolver.Write).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updated, err := p.sms.MarkFailedIfAccepted(ctx, tx, accountID, messageID)
		if err != nil {
			return err
		}
		if !updated {
			slog.WarnContext(ctx, "sms status not updated to failed",
				"message_id", messageID,
				"account_id", accountID,
			)
		}
		return nil
	})
}

func ParsePayload(body []byte) (domain.SMSSendPayload, error) {
	var payload domain.SMSSendPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return domain.SMSSendPayload{}, fmt.Errorf("unmarshal payload: %w", err)
	}
	return payload, nil
}
