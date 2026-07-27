package repository

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const defaultIntegrationDSN = "postgres://sms:sms@localhost:5433/sms_gateway_test?sslmode=disable"

// IntegrationDSN returns the PostgreSQL DSN for integration tests.
func IntegrationDSN() string {
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	return defaultIntegrationDSN
}

// NewIntegrationDB opens PostgreSQL for concurrency and integration tests.
// Skips the test when the database is unavailable.
func NewIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := IntegrationDSN()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Skipf("integration database unavailable: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Skipf("integration database unavailable: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("integration database unavailable: %v", err)
	}

	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetMaxIdleConns(25)

	if !integrationSchemaReady(db) {
		t.Skipf("integration schema missing; run: make test-integration-setup")
	}

	truncateIntegrationTables(t, db)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	return db
}

func truncateIntegrationTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	err := db.Exec(`
		TRUNCATE TABLE
			idempotency_keys,
			outbox_events,
			sms_messages,
			account_ledger,
			accounts
		RESTART IDENTITY CASCADE
	`).Error
	require.NoError(t, err, "truncate integration tables")
}

func integrationSchemaReady(db *gorm.DB) bool {
	var exists bool
	err := db.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = current_schema() AND table_name = 'accounts'
		)
	`).Scan(&exists).Error
	return err == nil && exists
}

// IntegrationDBAvailable reports whether PostgreSQL integration tests can run.
func IntegrationDBAvailable() bool {
	dsn := IntegrationDSN()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return false
	}
	sqlDB, err := db.DB()
	if err != nil {
		return false
	}
	defer sqlDB.Close()
	return sqlDB.Ping() == nil
}

// SkipIntegrationUnlessAvailable skips unless TEST_DATABASE_URL is set or PostgreSQL is reachable.
func SkipIntegrationUnlessAvailable(t *testing.T) {
	t.Helper()
	if os.Getenv("TEST_DATABASE_URL") != "" {
		return
	}
	if !IntegrationDBAvailable() {
		t.Skip("integration database unavailable; set TEST_DATABASE_URL or run: make test-integration-setup")
	}
}

// IntegrationSkipReason returns a human-readable reason when integration DB is unavailable.
func IntegrationSkipReason() string {
	if IntegrationDBAvailable() {
		return ""
	}
	return fmt.Sprintf("integration database unavailable at %s", IntegrationDSN())
}
