package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/shahriyar/arvan/internal/observability"
	"github.com/shahriyar/arvan/internal/repository"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

type SMSService struct {
	accounts    *repository.AccountRepository
	ledger      *repository.LedgerRepository
	sms         *repository.SMSRepository
	outbox      *repository.OutboxRepository
	idempotency *repository.IdempotencyRepository
}

func NewSMSService(
	accounts *repository.AccountRepository,
	ledger *repository.LedgerRepository,
	sms *repository.SMSRepository,
	outbox *repository.OutboxRepository,
	idempotency *repository.IdempotencyRepository,
) *SMSService {
	return &SMSService{
		accounts:    accounts,
		ledger:      ledger,
		sms:         sms,
		outbox:      outbox,
		idempotency: idempotency,
	}
}

func (s *SMSService) Send(ctx context.Context, accountID uuid.UUID, input domain.SendSMSInput) (domain.SendSMSResult, error) {
	ctx, span := observability.StartSpan(ctx, "sms.send",
		trace.WithAttributes(
			attribute.String("account_id", accountID.String()),
			attribute.String("message_type", input.MessageType),
		),
	)
	defer span.End()

	encoding, err := validateSendInput(input)
	if err != nil {
		return domain.SendSMSResult{}, err
	}

	messageID := uuid.New()
	outboxID := uuid.New()
	cost := domain.SMSCostPerMessage

	var result domain.SendSMSResult
	err = s.accounts.DB().Clauses(dbresolver.Write).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if input.IdempotencyKey != nil && *input.IdempotencyKey != "" {
			record, claimed, err := s.idempotency.ClaimOrGet(ctx, tx, accountID, *input.IdempotencyKey)
			if err != nil {
				return err
			}
			if !claimed {
				if cached, ok := domain.ParseIdempotencyResponse(record.ResponseSnapshot); ok {
					parsedID, err := uuid.Parse(cached.MessageID)
					if err != nil {
						return fmt.Errorf("parse cached message id: %w", err)
					}
					result = domain.SendSMSResult{
						MessageID: parsedID,
						Status:    cached.Status,
					}
					return nil
				}
				return domainerrors.ErrIdempotencyInProgress
			}
		}

		lockedBalance, err := s.accounts.LockAccountForUpdate(ctx, tx, accountID)
		if err != nil {
			return err
		}

		if lockedBalance < cost {
			observability.RecordBalanceDeductError()
			return domainerrors.ErrInsufficientBalance
		}

		if _, err := s.accounts.DeductLockedBalance(ctx, tx, accountID, cost); err != nil {
			observability.RecordBalanceDeductError()
			return err
		}

		refID := messageID
		if err := s.ledger.Create(ctx, tx, domain.LedgerEntry{
			AccountID: accountID,
			Delta:     -cost,
			Reason:    domain.LedgerReasonSend,
			RefID:     &refID,
		}); err != nil {
			return err
		}

		smsMsg := domain.SMSMessage{
			ID:          messageID,
			AccountID:   accountID,
			ToNumber:    input.To,
			Body:        input.Body,
			Encoding:    encoding,
			MessageType: input.MessageType,
			Status:      domain.SMSStatusAccepted,
			Cost:        cost,
		}
		if input.IdempotencyKey != nil && *input.IdempotencyKey != "" {
			smsMsg.IdempotencyKey = input.IdempotencyKey
		}

		if err := s.sms.Create(ctx, tx, smsMsg); err != nil {
			return err
		}

		payload, err := json.Marshal(domain.SMSSendPayload{
			MessageID:    messageID.String(),
			AccountID:    accountID.String(),
			To:           input.To,
			Body:         input.Body,
			MessageType:  input.MessageType,
			TraceContext: observability.InjectMap(ctx),
		})
		if err != nil {
			return fmt.Errorf("marshal outbox payload: %w", err)
		}

		if err := s.outbox.Create(ctx, tx, domain.OutboxEvent{
			ID:          outboxID,
			AggregateID: messageID,
			EventType:   domain.OutboxEventTypeSMSSendRequested,
			Payload:     payload,
			Status:      domain.OutboxStatusPending,
		}); err != nil {
			return err
		}

		result = domain.SendSMSResult{
			MessageID: messageID,
			Status:    domain.SMSStatusAccepted,
		}

		if input.IdempotencyKey != nil && *input.IdempotencyKey != "" {
			snapshot, err := domain.MarshalIdempotencyResponse(result)
			if err != nil {
				return fmt.Errorf("marshal idempotency response: %w", err)
			}
			if err := s.idempotency.UpdateSnapshot(ctx, tx, accountID, *input.IdempotencyKey, snapshot); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return domain.SendSMSResult{}, fmt.Errorf("send sms: %w", err)
	}

	return result, nil
}

func validateSendInput(input domain.SendSMSInput) (string, error) {
	if strings.TrimSpace(input.To) == "" || strings.TrimSpace(input.Body) == "" {
		return "", domainerrors.ErrInvalidRequest
	}

	if !domain.ValidateE164(input.To) {
		return "", domainerrors.ErrInvalidPhoneNumber
	}

	switch input.MessageType {
	case domain.MessageTypeStandard, domain.MessageTypeExpress:
	default:
		return "", domainerrors.ErrInvalidMessageType
	}

	encoding, err := domain.ValidateSinglePageBody(input.Body)
	if err != nil {
		return "", err
	}

	return encoding, nil
}

func (s *SMSService) Get(ctx context.Context, accountID, messageID uuid.UUID) (domain.SMSMessage, error) {
	msg, err := s.sms.GetByAccountAndID(ctx, accountID, messageID)
	if err != nil {
		return domain.SMSMessage{}, fmt.Errorf("get sms: %w", err)
	}
	return *msg, nil
}

func (s *SMSService) List(
	ctx context.Context,
	accountID uuid.UUID,
	limit int,
	cursor *uuid.UUID,
) ([]domain.SMSMessage, error) {
	messages, err := s.sms.ListByAccount(ctx, accountID, limit, cursor)
	if err != nil {
		return nil, fmt.Errorf("list sms: %w", err)
	}
	return messages, nil
}
