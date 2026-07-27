package repository

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const defaultIntegrationDSN = "postgres://sms:sms@localhost:5433/sms_gateway_test?sslmode=disable"

var (
	integrationSetupMu  sync.Mutex
	sharedIntegrationDB *gorm.DB
)

// NewIntegrationDB returns a shared PostgreSQL pool for integration tests.
// Skips the test when the database is unavailable.
func NewIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()

	integrationSetupMu.Lock()
	defer integrationSetupMu.Unlock()

	if sharedIntegrationDB == nil {
		db, err := openIntegrationDB()
		if err != nil {
			t.Skipf("integration database unavailable: %v", err)
		}
		if !integrationSchemaReady(db) {
			t.Skipf("integration schema missing; run: make test-integration-setup")
		}
		sharedIntegrationDB = db
	}

	truncateIntegrationTables(t, sharedIntegrationDB)
	return sharedIntegrationDB
}

func openIntegrationDB() (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(IntegrationDSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if err := sqlDB.Ping(); err != nil {
		return nil, err
	}

	// Single pool; dbresolver is exercised in unit tests (SQLite) and production NewDB.
	sqlDB.SetMaxOpenConns(30)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	return db, nil
}

func truncateIntegrationTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	const query = `
		TRUNCATE TABLE
			idempotency_keys,
			processed_consumer_events,
			outbox_events,
			sms_messages,
			account_ledger,
			accounts
		RESTART IDENTITY CASCADE
	`
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		lastErr = db.Exec(query).Error
		if lastErr == nil {
			return
		}
		if !strings.Contains(lastErr.Error(), "deadlock detected") {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
	}
	require.NoError(t, lastErr, "truncate integration tables")
}

// IntegrationDSN returns the PostgreSQL DSN for integration tests.
func IntegrationDSN() string {
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	return defaultIntegrationDSN
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
