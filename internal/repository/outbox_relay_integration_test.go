//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutboxRepositoryClaimPendingBatchSkipLocked(t *testing.T) {
	SkipIntegrationUnlessAvailable(t)
	db := NewIntegrationDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	eventIDs := make([]uuid.UUID, 4)
	for i := range eventIDs {
		eventIDs[i] = uuid.New()
		payload, err := json.Marshal(domain.SMSSendPayload{
			MessageID:   uuid.NewString(),
			AccountID:   uuid.NewString(),
			To:          "+989121234567",
			Body:        "Hello",
			MessageType: domain.MessageTypeStandard,
		})
		require.NoError(t, err)

		require.NoError(t, repo.Create(ctx, nil, domain.OutboxEvent{
			ID:          eventIDs[i],
			AggregateID: uuid.New(),
			EventType:   domain.OutboxEventTypeSMSSendRequested,
			Payload:     payload,
			Status:      domain.OutboxStatusPending,
		}))
	}

	const workers = 4
	var wg sync.WaitGroup
	claimed := make(chan uuid.UUID, len(eventIDs))
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			batch, err := repo.ClaimPendingBatch(ctx, 2, 30*time.Second)
			if err != nil {
				errs <- err
				return
			}
			for _, event := range batch {
				claimed <- event.ID
			}
		}()
	}

	wg.Wait()
	close(claimed)
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	seen := make(map[uuid.UUID]struct{})
	for id := range claimed {
		_, exists := seen[id]
		assert.False(t, exists, "duplicate claim for event %s", id)
		seen[id] = struct{}{}
	}
	assert.Len(t, seen, len(eventIDs))
}

func TestProcessedConsumerRepositoryClaimIsExclusive(t *testing.T) {
	SkipIntegrationUnlessAvailable(t)
	db := NewIntegrationDB(t)
	repo := NewProcessedConsumerRepository(db)
	ctx := context.Background()
	messageID := uuid.New()

	const workers = 8
	var wg sync.WaitGroup
	var insertedCount int
	var mu sync.Mutex

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inserted, err := repo.MarkProcessedIfNew(ctx, nil, messageID)
			require.NoError(t, err)
			if inserted {
				mu.Lock()
				insertedCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, 1, insertedCount)
}

func TestOutboxRepositoryMarkPublishedIsIdempotent(t *testing.T) {
	SkipIntegrationUnlessAvailable(t)
	db := NewIntegrationDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	eventID := uuid.New()
	payload, err := json.Marshal(domain.SMSSendPayload{
		MessageID:   uuid.NewString(),
		AccountID:   uuid.NewString(),
		To:          "+989121234567",
		Body:        "Hello",
		MessageType: domain.MessageTypeStandard,
	})
	require.NoError(t, err)

	require.NoError(t, repo.Create(ctx, nil, domain.OutboxEvent{
		ID:          eventID,
		AggregateID: uuid.New(),
		EventType:   domain.OutboxEventTypeSMSSendRequested,
		Payload:     payload,
		Status:      domain.OutboxStatusPending,
	}))

	require.NoError(t, repo.MarkPublished(ctx, eventID))
	require.NoError(t, repo.MarkPublished(ctx, eventID))

	published, err := repo.IsPublished(ctx, eventID)
	require.NoError(t, err)
	assert.True(t, published)
}
