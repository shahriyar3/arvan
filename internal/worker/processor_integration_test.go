//go:build integration

package worker

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/broker"
	"github.com/shahriyar/arvan/internal/config"
	"github.com/shahriyar/arvan/internal/domain"
	"github.com/shahriyar/arvan/internal/operator"
	"github.com/shahriyar/arvan/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleDeliveryConcurrentClaimsSingleOperatorCallIntegration(t *testing.T) {
	repository.SkipIntegrationUnlessAvailable(t)
	db := repository.NewIntegrationDB(t)

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

	op := &fakeOperator{}
	processor := NewProcessor(
		db,
		smsRepo,
		repository.NewProcessedConsumerRepository(db),
		op,
		config.WorkerConfig{},
	)

	body, err := json.Marshal(domain.SMSSendPayload{
		MessageID:   messageID.String(),
		AccountID:   accountID.String(),
		To:          "+989121234567",
		Body:        "Hello",
		MessageType: domain.MessageTypeStandard,
	})
	require.NoError(t, err)

	const workers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			processor.handleDelivery(context.Background(), broker.QueueStandard, amqp.Delivery{
				Body:         append([]byte(nil), body...),
				Acknowledger: &stubAcknowledger{},
			})
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, 1, op.callCount())

	msg, err := smsRepo.GetByAccountAndID(context.Background(), accountID, messageID)
	require.NoError(t, err)
	assert.Equal(t, domain.SMSStatusSent, msg.Status)
}

// keep compiler happy when fakeOperator is only used here in integration build
var _ operator.SMSOperator = (*fakeOperator)(nil)
