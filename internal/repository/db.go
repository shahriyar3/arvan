package repository

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shahriyar/arvan/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/dbresolver"
)

func NewDB(cfg config.DatabaseConfig) (*gorm.DB, error) {
	replicaDSN := cfg.ReplicaDSN
	if replicaDSN == "" {
		replicaDSN = cfg.PrimaryDSN
	}

	db, err := gorm.Open(postgres.Open(cfg.PrimaryDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Use(dbresolver.Register(dbresolver.Config{
		Sources:  []gorm.Dialector{postgres.Open(cfg.PrimaryDSN)},
		Replicas: []gorm.Dialector{postgres.Open(replicaDSN)},
		Policy:   dbresolver.RandomPolicy{},
	})); err != nil {
		return nil, fmt.Errorf("register dbresolver: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)

	return db, nil
}

func writeDB(db *gorm.DB) *gorm.DB {
	return db.Clauses(dbresolver.Write)
}

func readDB(db *gorm.DB) *gorm.DB {
	return db.Clauses(dbresolver.Read)
}

type ledgerModel struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	AccountID uuid.UUID  `gorm:"type:uuid;not null;index:idx_account_ledger_account,priority:1"`
	Delta     int64      `gorm:"not null"`
	Reason    string     `gorm:"size:50;not null"`
	RefID     *uuid.UUID `gorm:"type:uuid"`
	CreatedAt time.Time  `gorm:"not null;autoCreateTime;index:idx_account_ledger_account,priority:2,sort:desc"`
}

func (ledgerModel) TableName() string {
	return "account_ledger"
}
