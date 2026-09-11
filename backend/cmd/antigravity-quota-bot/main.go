package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"zyrouter/backend/internal/antigravitybot"
	"zyrouter/backend/internal/antigravityquota"
	"zyrouter/backend/internal/config"
	"zyrouter/backend/internal/db"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "antigravity-quota-bot:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.LoadConfig()
	token := strings.TrimSpace(os.Getenv("TELEGRAM_QUOTA_BOT_TOKEN"))
	if token == "" {
		return fmt.Errorf("TELEGRAM_QUOTA_BOT_TOKEN is required (use a separate BotFather token from the gateway verification bot)")
	}
	allowedUsers, err := antigravitybot.ParseAllowedUserIDs(os.Getenv("TELEGRAM_QUOTA_ALLOWED_USER_IDS"))
	if err != nil {
		return err
	}

	database, err := db.OpenDatabase(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open Zyrouter database %q: %w", cfg.DatabasePath, err)
	}
	defer database.Close()

	quotaService := antigravityquota.NewService(db.NewRepo(database))
	bot := antigravitybot.New(token, allowedUsers, quotaService)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Antigravity quota bot started; DB=%s\n", cfg.DatabasePath)
	return bot.Run(ctx)
}
