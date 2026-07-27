package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/shahriyar/arvan/internal/config"
	"github.com/shahriyar/arvan/internal/observability"
	"github.com/shahriyar/arvan/internal/repository"
	"github.com/shahriyar/arvan/internal/seed"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := observability.NewLogger(cfg.App.LogLevel)
	slog.SetDefault(logger)

	db, err := repository.NewDB(cfg.Database)
	if err != nil {
		logger.Error("failed to connect database", "error", err)
		os.Exit(1)
	}

	accountRepo := repository.NewAccountRepository(db)
	ctx := context.Background()

	for _, account := range seed.Accounts {
		tokenHash := seed.TokenHash(account.Token)
		created, err := accountRepo.UpsertByTokenHash(ctx, tokenHash)
		if err != nil {
			logger.Error("failed to seed account", "name", account.Name, "error", err)
			os.Exit(1)
		}
		logger.Info("seeded account", "name", account.Name, "account_id", created.ID.String())
	}

	logger.Info("seed completed", "accounts", len(seed.Accounts))
}
