package worker

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/shahriyar/arvan/internal/broker"
	"github.com/shahriyar/arvan/internal/config"
	"github.com/shahriyar/arvan/internal/domain"
	"github.com/shahriyar/arvan/internal/observability"
	"github.com/shahriyar/arvan/internal/operator"
	"github.com/shahriyar/arvan/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubAcknowledger struct {
	mu       sync.Mutex
	acked    int
	nacked   int
	requeued bool
}

func (s *stubAcknowledger) Ack(_ uint64, _ bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acked++
	return nil
}

func (s *stubAcknowledger) Nack(_ uint64, _ bool, requeue bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nacked++
	s.requeued = requeue
	return nil
}

func (s *stubAcknowledger) Reject(_ uint64, _ bool) error {
	return nil
}

type fakeOperator struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeOperator) Send(context.Context, operator.SendRequest) (operator.SendResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return operator.SendResult{}, f.err
	}
	return operator.SendResult{OperatorRef: "op-ref"}, nil
}

func (f *fakeOperator) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newProcessorTestHarness(t *testing.T, op operator.SMSOperator) (*Processor, uuid.UUID, uuid.UUID) {
	t.Helper()

	db := repository.NewTestDB(t)
	accountID := uuid.New()
	messageID := uuid.New()

	repository.SeedTestAccount(t, db, accountID)

	smsRepo := repository.NewSMSRepository(db)
	require.NoError(t, smsRepo.Create(context.Background(), nil, domain.SMSMessage{
		ID:          messageID,
		AccountID:   accountID,
		ToNumber:    "+989121234567",
		Body:        "Hello",
		Encoding:    domain.EncodingGSM7,
		MessageType: domain.MessageTypeStandard,
		Status:      domain.SMSStatusAccepted,
		Cost:        domain.SMSCostPerMessage,
	}))

	return NewProcessor(
		db,
		smsRepo,
		repository.NewProcessedConsumerRepository(db),
		op,
		config.WorkerConfig{},
	), accountID, messageID
}

func TestParsePayload(t *testing.T) {
	raw, err := json.Marshal(domain.SMSSendPayload{
		MessageID:   "550e8400-e29b-41d4-a716-446655440000",
		AccountID:   "660e8400-e29b-41d4-a716-446655440001",
		To:          "+989121234567",
		Body:        "Hello",
		MessageType: domain.MessageTypeStandard,
	})
	require.NoError(t, err)

	payload, err := ParsePayload(raw)
	require.NoError(t, err)
	assert.Equal(t, "+989121234567", payload.To)
	assert.Equal(t, domain.MessageTypeStandard, payload.MessageType)
}

func TestParsePayloadInvalidJSON(t *testing.T) {
	_, err := ParsePayload([]byte("not-json"))
	assert.Error(t, err)
}

func TestClaimDeliveryAcquiresBeforeOperator(t *testing.T) {
	db := repository.NewTestDB(t)
	accountID := uuid.New()
	messageID := uuid.New()

	repository.SeedTestAccount(t, db, accountID)

	smsRepo := repository.NewSMSRepository(db)
	require.NoError(t, smsRepo.Create(context.Background(), nil, domain.SMSMessage{
		ID:          messageID,
		AccountID:   accountID,
		ToNumber:    "+989121234567",
		Body:        "Hello",
		Encoding:    domain.EncodingGSM7,
		MessageType: domain.MessageTypeStandard,
		Status:      domain.SMSStatusAccepted,
		Cost:        domain.SMSCostPerMessage,
	}))

	processor := NewProcessor(
		db,
		smsRepo,
		repository.NewProcessedConsumerRepository(db),
		&fakeOperator{},
		config.WorkerConfig{},
	)

	claim, err := processor.claimDelivery(context.Background(), accountID, messageID, false)
	require.NoError(t, err)
	assert.Equal(t, claimAcquired, claim)

	exists, err := repository.NewProcessedConsumerRepository(db).Exists(context.Background(), messageID)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestClaimDeliveryAlreadySent(t *testing.T) {
	db := repository.NewTestDB(t)
	accountID := uuid.New()
	messageID := uuid.New()

	repository.SeedTestAccount(t, db, accountID)

	smsRepo := repository.NewSMSRepository(db)
	require.NoError(t, smsRepo.Create(context.Background(), nil, domain.SMSMessage{
		ID:          messageID,
		AccountID:   accountID,
		ToNumber:    "+989121234567",
		Body:        "Hello",
		Encoding:    domain.EncodingGSM7,
		MessageType: domain.MessageTypeStandard,
		Status:      domain.SMSStatusSent,
		Cost:        domain.SMSCostPerMessage,
	}))

	processor := NewProcessor(
		db,
		smsRepo,
		repository.NewProcessedConsumerRepository(db),
		&fakeOperator{},
		config.WorkerConfig{},
	)

	claim, err := processor.claimDelivery(context.Background(), accountID, messageID, false)
	require.NoError(t, err)
	assert.Equal(t, claimAlreadyDone, claim)
}

func TestClaimDeliveryInProgressWhenAlreadyClaimed(t *testing.T) {
	db := repository.NewTestDB(t)
	accountID := uuid.New()
	messageID := uuid.New()

	repository.SeedTestAccount(t, db, accountID)

	smsRepo := repository.NewSMSRepository(db)
	require.NoError(t, smsRepo.Create(context.Background(), nil, domain.SMSMessage{
		ID:          messageID,
		AccountID:   accountID,
		ToNumber:    "+989121234567",
		Body:        "Hello",
		Encoding:    domain.EncodingGSM7,
		MessageType: domain.MessageTypeStandard,
		Status:      domain.SMSStatusAccepted,
		Cost:        domain.SMSCostPerMessage,
	}))

	processedRepo := repository.NewProcessedConsumerRepository(db)
	_, err := processedRepo.MarkProcessedIfNew(context.Background(), nil, messageID)
	require.NoError(t, err)

	processor := NewProcessor(
		db,
		smsRepo,
		processedRepo,
		&fakeOperator{},
		config.WorkerConfig{},
	)

	claim, err := processor.claimDelivery(context.Background(), accountID, messageID, false)
	require.NoError(t, err)
	assert.Equal(t, claimInProgress, claim)
}

func TestClaimDeliveryResumeWhenRedelivered(t *testing.T) {
	db := repository.NewTestDB(t)
	accountID := uuid.New()
	messageID := uuid.New()

	repository.SeedTestAccount(t, db, accountID)

	smsRepo := repository.NewSMSRepository(db)
	require.NoError(t, smsRepo.Create(context.Background(), nil, domain.SMSMessage{
		ID:          messageID,
		AccountID:   accountID,
		ToNumber:    "+989121234567",
		Body:        "Hello",
		Encoding:    domain.EncodingGSM7,
		MessageType: domain.MessageTypeStandard,
		Status:      domain.SMSStatusAccepted,
		Cost:        domain.SMSCostPerMessage,
	}))

	processedRepo := repository.NewProcessedConsumerRepository(db)
	_, err := processedRepo.MarkProcessedIfNew(context.Background(), nil, messageID)
	require.NoError(t, err)

	processor := NewProcessor(
		db,
		smsRepo,
		processedRepo,
		&fakeOperator{},
		config.WorkerConfig{},
	)

	claim, err := processor.claimDelivery(context.Background(), accountID, messageID, true)
	require.NoError(t, err)
	assert.Equal(t, claimResume, claim)
}

func TestHandleDeliveryResumeSkipsOperator(t *testing.T) {
	op := &fakeOperator{}
	processor, accountID, messageID := newProcessorTestHarness(t, op)

	_, err := processor.processed.MarkProcessedIfNew(context.Background(), nil, messageID)
	require.NoError(t, err)

	ack := &stubAcknowledger{}
	body, err := json.Marshal(domain.SMSSendPayload{
		MessageID:   messageID.String(),
		AccountID:   accountID.String(),
		To:          "+989121234567",
		Body:        "Hello",
		MessageType: domain.MessageTypeStandard,
	})
	require.NoError(t, err)

	processor.handleDelivery(context.Background(), broker.QueueStandard, amqp.Delivery{
		Body:         body,
		Acknowledger: ack,
		Redelivered:  true,
	})

	assert.Equal(t, 1, ack.acked)
	assert.Equal(t, 0, op.callCount())

	msg, err := processor.sms.GetByAccountAndID(context.Background(), accountID, messageID)
	require.NoError(t, err)
	assert.Equal(t, domain.SMSStatusSent, msg.Status)
}

func TestHandleDeliverySuccessMarksSent(t *testing.T) {
	op := &fakeOperator{}
	processor, accountID, messageID := newProcessorTestHarness(t, op)

	ack := &stubAcknowledger{}
	body, err := json.Marshal(domain.SMSSendPayload{
		MessageID:   messageID.String(),
		AccountID:   accountID.String(),
		To:          "+989121234567",
		Body:        "Hello",
		MessageType: domain.MessageTypeStandard,
	})
	require.NoError(t, err)

	processor.handleDelivery(context.Background(), broker.QueueStandard, amqp.Delivery{
		Body:         body,
		Acknowledger: ack,
	})

	assert.Equal(t, 1, ack.acked)
	assert.Equal(t, 0, ack.nacked)
	assert.Equal(t, 1, op.callCount())

	msg, err := processor.sms.GetByAccountAndID(context.Background(), accountID, messageID)
	require.NoError(t, err)
	assert.Equal(t, domain.SMSStatusSent, msg.Status)
}

func TestHandleDeliveryPermanentFailureMarksFailed(t *testing.T) {
	op := &fakeOperator{err: &operator.PermanentError{StatusCode: 400, Body: "bad request"}}
	processor, accountID, messageID := newProcessorTestHarness(t, op)

	ack := &stubAcknowledger{}
	body, err := json.Marshal(domain.SMSSendPayload{
		MessageID:   messageID.String(),
		AccountID:   accountID.String(),
		To:          "+989121234567",
		Body:        "Hello",
		MessageType: domain.MessageTypeStandard,
	})
	require.NoError(t, err)

	processor.handleDelivery(context.Background(), broker.QueueStandard, amqp.Delivery{
		Body:         body,
		Acknowledger: ack,
	})

	assert.Equal(t, 1, ack.acked)
	assert.Equal(t, 0, ack.nacked)
	assert.Equal(t, 1, op.callCount())

	msg, err := processor.sms.GetByAccountAndID(context.Background(), accountID, messageID)
	require.NoError(t, err)
	assert.Equal(t, domain.SMSStatusFailed, msg.Status)

	exists, err := processor.processed.Exists(context.Background(), messageID)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestHandleDeliveryTransientFailureReleasesClaim(t *testing.T) {
	op := &fakeOperator{err: errors.New("operator unavailable")}
	processor, accountID, messageID := newProcessorTestHarness(t, op)

	ack := &stubAcknowledger{}
	body, err := json.Marshal(domain.SMSSendPayload{
		MessageID:   messageID.String(),
		AccountID:   accountID.String(),
		To:          "+989121234567",
		Body:        "Hello",
		MessageType: domain.MessageTypeStandard,
	})
	require.NoError(t, err)

	processor.handleDelivery(context.Background(), broker.QueueStandard, amqp.Delivery{
		Body:         body,
		Acknowledger: ack,
	})

	assert.Equal(t, 0, ack.acked)
	assert.Equal(t, 1, ack.nacked)
	assert.True(t, ack.requeued)
	assert.Equal(t, 1, op.callCount())

	exists, err := processor.processed.Exists(context.Background(), messageID)
	require.NoError(t, err)
	assert.False(t, exists)

	msg, err := processor.sms.GetByAccountAndID(context.Background(), accountID, messageID)
	require.NoError(t, err)
	assert.Equal(t, domain.SMSStatusAccepted, msg.Status)
}

func TestHandleDeliveryCircuitOpenRequeues(t *testing.T) {
	op := &fakeOperator{err: operator.ErrCircuitOpen}
	processor, accountID, messageID := newProcessorTestHarness(t, op)

	ack := &stubAcknowledger{}
	body, err := json.Marshal(domain.SMSSendPayload{
		MessageID:   messageID.String(),
		AccountID:   accountID.String(),
		To:          "+989121234567",
		Body:        "Hello",
		MessageType: domain.MessageTypeStandard,
	})
	require.NoError(t, err)

	processor.handleDelivery(context.Background(), broker.QueueStandard, amqp.Delivery{
		Body:         body,
		Acknowledger: ack,
	})

	assert.Equal(t, 0, ack.acked)
	assert.Equal(t, 1, ack.nacked)
	assert.True(t, ack.requeued)
	assert.Equal(t, 1, op.callCount())

	exists, err := processor.processed.Exists(context.Background(), messageID)
	require.NoError(t, err)
	assert.False(t, exists)

	msg, err := processor.sms.GetByAccountAndID(context.Background(), accountID, messageID)
	require.NoError(t, err)
	assert.Equal(t, domain.SMSStatusAccepted, msg.Status)
}

func TestHandleDeliveryConcurrentClaimsSingleOperatorCall(t *testing.T) {
	t.Skip("covered by integration test TestHandleDeliveryConcurrentClaimsSingleOperatorCallIntegration")
}

type stubDLQ struct {
	bodies   [][]byte
	failNext bool
}

func (s *stubDLQ) PublishToDLQ(_ context.Context, body []byte) error {
	if s.failNext {
		s.failNext = false
		return errors.New("dlq unavailable")
	}
	s.bodies = append(s.bodies, append([]byte(nil), body...))
	return nil
}

func TestHandleDeliveryDeadLettersAfterMaxAttempts(t *testing.T) {
	op := &fakeOperator{}
	processor, accountID, messageID := newProcessorTestHarness(t, op)
	processor.cfg.MaxDeliveryAttempts = 2
	dlq := &stubDLQ{}
	processor.SetDLQPublisher(dlq)

	ack := &stubAcknowledger{}
	body, err := json.Marshal(domain.SMSSendPayload{
		MessageID:   messageID.String(),
		AccountID:   accountID.String(),
		To:          "+989121234567",
		Body:        "Hello",
		MessageType: domain.MessageTypeStandard,
	})
	require.NoError(t, err)

	processor.handleDelivery(context.Background(), broker.QueueStandard, amqp.Delivery{
		Body:         body,
		Acknowledger: ack,
		Redelivered:  true,
		Headers: amqp.Table{
			"x-death": []any{
				amqp.Table{"count": int64(2)},
			},
		},
	})

	assert.Equal(t, 1, ack.acked)
	assert.Equal(t, 0, ack.nacked)
	assert.Equal(t, 0, op.callCount())
	assert.Len(t, dlq.bodies, 1)

	msg, err := processor.sms.GetByAccountAndID(context.Background(), accountID, messageID)
	require.NoError(t, err)
	assert.Equal(t, domain.SMSStatusDeadLettered, msg.Status)
}

func TestHandleDeliveryDeadLetterPersistsStatusBeforeDLQPublish(t *testing.T) {
	op := &fakeOperator{}
	processor, accountID, messageID := newProcessorTestHarness(t, op)
	processor.cfg.MaxDeliveryAttempts = 2
	dlq := &stubDLQ{failNext: true}
	processor.SetDLQPublisher(dlq)

	body, err := json.Marshal(domain.SMSSendPayload{
		MessageID:   messageID.String(),
		AccountID:   accountID.String(),
		To:          "+989121234567",
		Body:        "Hello",
		MessageType: domain.MessageTypeStandard,
	})
	require.NoError(t, err)

	deadLetterHeaders := amqp.Table{
		"x-death": []any{
			amqp.Table{"count": int64(2)},
		},
	}

	ack := &stubAcknowledger{}
	processor.handleDelivery(context.Background(), broker.QueueStandard, amqp.Delivery{
		Body:         body,
		Acknowledger: ack,
		Headers:      deadLetterHeaders,
	})

	assert.Equal(t, 0, ack.acked)
	assert.Equal(t, 1, ack.nacked)
	assert.True(t, ack.requeued)
	assert.Len(t, dlq.bodies, 0)

	msg, err := processor.sms.GetByAccountAndID(context.Background(), accountID, messageID)
	require.NoError(t, err)
	assert.Equal(t, domain.SMSStatusDeadLettered, msg.Status)

	ack = &stubAcknowledger{}
	processor.handleDelivery(context.Background(), broker.QueueStandard, amqp.Delivery{
		Body:         body,
		Acknowledger: ack,
		Headers:      deadLetterHeaders,
	})

	assert.Equal(t, 1, ack.acked)
	assert.Len(t, dlq.bodies, 1)
}

func histogramSampleCount(t *testing.T, histogram prometheus.Histogram) uint64 {
	t.Helper()

	ch := make(chan prometheus.Metric, 1)
	histogram.Collect(ch)
	pb := &dto.Metric{}
	require.NoError(t, (<-ch).Write(pb))
	return pb.GetHistogram().GetSampleCount()
}

func TestHandleDeliveryExpressRecordsSLAMetric(t *testing.T) {
	op := &fakeOperator{}
	processor, accountID, messageID := newProcessorTestHarness(t, op)

	before := histogramSampleCount(t, observability.ExpressOperatorDeliverySeconds)

	body, err := json.Marshal(domain.SMSSendPayload{
		MessageID:   messageID.String(),
		AccountID:   accountID.String(),
		To:          "+989121234567",
		Body:        "OTP",
		MessageType: domain.MessageTypeExpress,
	})
	require.NoError(t, err)

	processor.handleDelivery(context.Background(), broker.QueueExpress, amqp.Delivery{
		Body:         body,
		Acknowledger: &stubAcknowledger{},
	})

	assert.Equal(t, before+1, histogramSampleCount(t, observability.ExpressOperatorDeliverySeconds))
}
