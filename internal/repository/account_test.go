package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	domainerrors "github.com/shahriyar/arvan/internal/domain/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAccountRepositoryFindByTokenHash(t *testing.T) {
	db := newTestDB(t)
	repo := NewAccountRepository(db)
	ctx := context.Background()

	accountID := uuid.New()
	tokenHash := "hash-account-a"
	require.NoError(t, writeDB(db).Create(&accountModel{
		ID:        accountID,
		TokenHash: tokenHash,
	}).Error)

	t.Run("finds existing account on primary", func(t *testing.T) {
		account, err := repo.FindByTokenHash(ctx, tokenHash)
		require.NoError(t, err)
		assert.Equal(t, accountID, account.ID)
	})

	t.Run("missing token returns not found", func(t *testing.T) {
		_, err := repo.FindByTokenHash(ctx, "missing")
		assert.ErrorIs(t, err, domainerrors.ErrNotFound)
	})
}

func TestAccountRepositoryLockAndAddBalance(t *testing.T) {
	db := newTestDB(t)
	repo := NewAccountRepository(db)
	ctx := context.Background()

	accountID := uuid.New()
	require.NoError(t, writeDB(db).Create(&accountModel{
		ID:        accountID,
		TokenHash: "hash-topup",
		Balance:   100,
	}).Error)

	err := db.Transaction(func(tx *gorm.DB) error {
		balance, err := repo.LockAndAddBalance(ctx, tx, accountID, 50)
		if err != nil {
			return err
		}
		assert.Equal(t, int64(150), balance)
		return nil
	})
	require.NoError(t, err)

	var model accountModel
	require.NoError(t, readDB(db).First(&model, "id = ?", accountID).Error)
	assert.Equal(t, int64(150), model.Balance)
}

func TestAccountRepositoryLockAndAddBalanceNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewAccountRepository(db)
	ctx := context.Background()

	err := db.Transaction(func(tx *gorm.DB) error {
		_, err := repo.LockAndAddBalance(ctx, tx, uuid.New(), 10)
		return err
	})
	assert.ErrorIs(t, err, domainerrors.ErrNotFound)
}
