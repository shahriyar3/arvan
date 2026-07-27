package repository

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/dbresolver"
)

// NewTestDB opens an in-memory SQLite database with dbresolver configured for tests.
func NewTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return newTestDB(t)
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:testdb_%s?mode=memory&cache=shared", t.Name())
	open := func() gorm.Dialector {
		return sqlite.Open(dsn)
	}

	db, err := gorm.Open(open(), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.Use(dbresolver.Register(dbresolver.Config{
		Sources:  []gorm.Dialector{open()},
		Replicas: []gorm.Dialector{open()},
		Policy:   dbresolver.RandomPolicy{},
	}))
	require.NoError(t, err)

	err = db.AutoMigrate(&accountModel{}, &ledgerModel{}, &smsMessageModel{}, &outboxEventModel{}, &idempotencyKeyModel{})
	require.NoError(t, err)

	return db
}

// UsesSQLite reports whether the test database is SQLite (no concurrent write stress).
func UsesSQLite(db *gorm.DB) bool {
	_, ok := db.Dialector.(*sqlite.Dialector)
	return ok
}
