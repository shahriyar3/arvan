package outbox

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/broker"
	"github.com/shahriyar/arvan/internal/config"
	"github.com/shahriyar/arvan/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePublisher struct {
	queues []string
	bodies [][]byte
	err    error
}

func (f *fakePublisher) Publish(_ context.Context, queue string, body []byte) error {
	if f.err != nil {
		return f.err
	}
	f.queues = append(f.queues, queue)
	f.bodies = append(f.bodies, append([]byte(nil), body...))
	return nil
}

type fakeOutboxStore struct {
	published []uuid.UUID
}

func (f *fakeOutboxStore) ClaimPendingBatch(context.Context, int, time.Duration) ([]domain.OutboxEventRecord, error) {
	return nil, nil
}

func (f *fakeOutboxStore) IsPublished(_ context.Context, eventID uuid.UUID) (bool, error) {
	for _, id := range f.published {
		if id == eventID {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeOutboxStore) MarkPublished(_ context.Context, eventID uuid.UUID) error {
	f.published = append(f.published, eventID)
	return nil
}

func (f *fakeOutboxStore) RecordPublishFailure(context.Context, uuid.UUID) error {
	return nil
}

func TestQueueForPayload(t *testing.T) {
	assert.Equal(t, broker.QueueExpress, QueueForPayload(domain.SMSSendPayload{MessageType: domain.MessageTypeExpress}))
	assert.Equal(t, broker.QueueStandard, QueueForPayload(domain.SMSSendPayload{MessageType: domain.MessageTypeStandard}))
}

func TestRelayPublishEventSkipsAlreadyPublished(t *testing.T) {
	eventID := uuid.New()
	payload := domain.SMSSendPayload{
		MessageID:   uuid.NewString(),
		AccountID:   uuid.NewString(),
		To:          "+989121234567",
		Body:        "Hello",
		MessageType: domain.MessageTypeStandard,
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	publisher := &fakePublisher{}
	outboxStore := &fakeOutboxStore{published: []uuid.UUID{eventID}}
	relay := NewRelay(outboxStore, publisher, config.OutboxRelayConfig{})

	err = relay.publishEvent(context.Background(), domain.OutboxEventRecord{
		ID:      eventID,
		Payload: raw,
	})
	require.NoError(t, err)
	assert.Empty(t, publisher.queues)
}

func TestRelayPublishEventRoutesExpressQueue(t *testing.T) {
	eventID := uuid.New()
	payload := domain.SMSSendPayload{
		MessageID:   uuid.NewString(),
		AccountID:   uuid.NewString(),
		To:          "+989121234567",
		Body:        "OTP",
		MessageType: domain.MessageTypeExpress,
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	publisher := &fakePublisher{}
	outboxStore := &fakeOutboxStore{}
	relay := NewRelay(outboxStore, publisher, config.OutboxRelayConfig{})

	err = relay.publishEvent(context.Background(), domain.OutboxEventRecord{
		ID:      eventID,
		Payload: raw,
	})
	require.NoError(t, err)
	require.Len(t, publisher.queues, 1)
	assert.Equal(t, broker.QueueExpress, publisher.queues[0])
	assert.Equal(t, raw, publisher.bodies[0])
	assert.Equal(t, []uuid.UUID{eventID}, outboxStore.published)
}

func TestRelayPublishEventRoutesStandardQueue(t *testing.T) {
	eventID := uuid.New()
	payload := domain.SMSSendPayload{
		MessageID:   uuid.NewString(),
		AccountID:   uuid.NewString(),
		To:          "+989121234567",
		Body:        "Hello",
		MessageType: domain.MessageTypeStandard,
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	publisher := &fakePublisher{}
	outboxStore := &fakeOutboxStore{}
	relay := NewRelay(outboxStore, publisher, config.OutboxRelayConfig{})

	err = relay.publishEvent(context.Background(), domain.OutboxEventRecord{
		ID:      eventID,
		Payload: raw,
	})
	require.NoError(t, err)
	assert.Equal(t, broker.QueueStandard, publisher.queues[0])
}
