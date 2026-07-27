package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/config"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/shahriyar/arvan/internal/operator"
	"github.com/shahriyar/arvan/internal/repository"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

type Consumer interface {
	Consume(queue string, prefetch int) (<-chan amqp.Delivery, error)
}

type Processor struct {
	db        *gorm.DB
	sms       *repository.SMSRepository
	processed *repository.ProcessedConsumerRepository
	operator  operator.SMSOperator
	cfg       config.WorkerConfig
	inFlight  sync.WaitGroup
}

type deliveryClaim int

const (
	claimAcquired deliveryClaim = iota
	claimAlreadyDone
	claimInProgress
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
	}
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
			go func(d amqp.Delivery) {
				defer p.inFlight.Done()
				p.handleDelivery(ctx, d)
			}(delivery)
		}
	}
}

func (p *Processor) Wait() {
	p.inFlight.Wait()
}

func (p *Processor) handleDelivery(ctx context.Context, delivery amqp.Delivery) {
	requeue := false
	defer func() {
		if requeue {
			if err := delivery.Nack(false, true); err != nil {
				slog.Error("nack delivery", "error", err)
			}
			return
		}
		if err := delivery.Ack(false); err != nil {
			slog.Error("ack delivery", "error", err)
		}
	}()

	var payload domain.SMSSendPayload
	if err := json.Unmarshal(delivery.Body, &payload); err != nil {
		slog.Error("invalid queue payload", "error", err)
		return
	}

	messageID, err := uuid.Parse(payload.MessageID)
	if err != nil {
		slog.Error("invalid message_id in payload", "error", err)
		return
	}
	accountID, err := uuid.Parse(payload.AccountID)
	if err != nil {
		slog.Error("invalid account_id in payload", "error", err)
		return
	}

	claim, err := p.claimDelivery(ctx, accountID, messageID)
	if err != nil {
		slog.Error("claim delivery failed", "message_id", messageID, "error", err)
		requeue = true
		return
	}
	switch claim {
	case claimAlreadyDone:
		slog.Info("skip duplicate delivery", "message_id", messageID)
		return
	case claimInProgress:
		requeue = true
		return
	}

	_, err = p.operator.Send(ctx, operator.SendRequest{
		MessageID: payload.MessageID,
		AccountID: payload.AccountID,
		To:        payload.To,
		Body:      payload.Body,
	})
	if err != nil {
		if operator.IsPermanent(err) {
			if markErr := p.markFailed(ctx, accountID, messageID); markErr != nil {
				slog.Error("mark sms failed", "message_id", messageID, "error", markErr)
				requeue = true
				return
			}
			slog.Warn("operator permanently rejected sms", "message_id", messageID, "error", err)
			return
		}

		if releaseErr := p.releaseClaim(ctx, messageID); releaseErr != nil {
			slog.Error("release delivery claim", "message_id", messageID, "error", releaseErr)
		}
		slog.Warn("operator send failed", "message_id", messageID, "error", err)
		requeue = true
		return
	}

	if err := p.markSent(ctx, accountID, messageID); err != nil {
		if releaseErr := p.releaseClaim(ctx, messageID); releaseErr != nil {
			slog.Error("release delivery claim", "message_id", messageID, "error", releaseErr)
		}
		slog.Error("mark sms sent failed", "message_id", messageID, "error", err)
		requeue = true
		return
	}
}

func (p *Processor) claimDelivery(ctx context.Context, accountID, messageID uuid.UUID) (deliveryClaim, error) {
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
		case domain.SMSStatusSent, domain.SMSStatusFailed:
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
		if msg.Status == domain.SMSStatusSent || msg.Status == domain.SMSStatusFailed {
			claim = claimAlreadyDone
			return nil
		}

		claim = claimInProgress
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
			slog.Warn("sms status not updated to sent",
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
			slog.Warn("sms status not updated to failed",
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
