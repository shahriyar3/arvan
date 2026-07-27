package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/domain"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLedgerRepositoryListByAccount(t *testing.T) {
	db := newTestDB(t)
	repo := NewLedgerRepository(db)
	ctx := context.Background()

	accountA := uuid.New()
	accountB := uuid.New()
	require.NoError(t, writeDB(db).Create(&accountModel{ID: accountA, TokenHash: "a"}).Error)
	require.NoError(t, writeDB(db).Create(&accountModel{ID: accountB, TokenHash: "b"}).Error)

	entryA1 := ledgerModel{
		ID:        uuid.New(),
		AccountID: accountA,
		Delta:     100,
		Reason:    domain.LedgerReasonTopup,
		CreatedAt: time.Now().Add(-2 * time.Minute),
	}
	entryA2 := ledgerModel{
		ID:        uuid.New(),
		AccountID: accountA,
		Delta:     50,
		Reason:    domain.LedgerReasonTopup,
		CreatedAt: time.Now().Add(-1 * time.Minute),
	}
	require.NoError(t, writeDB(db).Create(&entryA1).Error)
	require.NoError(t, writeDB(db).Create(&entryA2).Error)

	t.Run("lists entries scoped to account", func(t *testing.T) {
		entries, err := repo.ListByAccount(ctx, accountA, 10, nil)
		require.NoError(t, err)
		require.Len(t, entries, 2)
		assert.Equal(t, entryA2.ID, entries[0].ID)
		assert.Equal(t, entryA1.ID, entries[1].ID)
	})

	t.Run("paginates with cursor", func(t *testing.T) {
		firstPage, err := repo.ListByAccount(ctx, accountA, 1, nil)
		require.NoError(t, err)
		require.Len(t, firstPage, 1)
		assert.Equal(t, entryA2.ID, firstPage[0].ID)

		secondPage, err := repo.ListByAccount(ctx, accountA, 1, &firstPage[0].ID)
		require.NoError(t, err)
		require.Len(t, secondPage, 1)
		assert.Equal(t, entryA1.ID, secondPage[0].ID)
	})

	t.Run("rejects cursor from another account", func(t *testing.T) {
		_, err := repo.ListByAccount(ctx, accountB, 10, &entryA1.ID)
		assert.ErrorIs(t, err, domainerrors.ErrInvalidCursor)
	})

	t.Run("rejects unknown cursor", func(t *testing.T) {
		unknown := uuid.New()
		_, err := repo.ListByAccount(ctx, accountA, 10, &unknown)
		assert.ErrorIs(t, err, domainerrors.ErrInvalidCursor)
	})
}

func TestLedgerRepositoryCreate(t *testing.T) {
	db := newTestDB(t)
	repo := NewLedgerRepository(db)
	ctx := context.Background()

	accountID := uuid.New()
	require.NoError(t, writeDB(db).Create(&accountModel{ID: accountID, TokenHash: "ledger"}).Error)

	err := repo.Create(ctx, nil, domain.LedgerEntry{
		AccountID: accountID,
		Delta:     25,
		Reason:    domain.LedgerReasonTopup,
	})
	require.NoError(t, err)

	entries, err := repo.ListByAccount(ctx, accountID, 10, nil)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, int64(25), entries[0].Delta)
}
