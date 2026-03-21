package tools

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var Database *pgxpool.Pool

func SetupDatabase(stop context.Context, await *sync.WaitGroup) {

	var err error
	t := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), TIMEOUT_CONTEXT)
	defer cancel()

	// Create and Test Client
	cfg, err := pgxpool.ParseConfig(DATABASE_URL)
	if err != nil {
		LoggerDatabase.Log(FATAL, "Invalid Database URI: %s", err)
	}
	if Database, err = pgxpool.NewWithConfig(ctx, cfg); err != nil {
		LoggerDatabase.Log(FATAL, "Failed to create pool: %s", err.Error())
	}
	if err = Database.Ping(ctx); err != nil {
		LoggerDatabase.Log(FATAL, "Failed to ping database: %s", err.Error())
	}

	// Shutdown Logic
	await.Add(1)
	go func() {
		defer await.Done()
		<-stop.Done()
		Database.Close()
		LoggerDatabase.Log(INFO, "Closed")
	}()

	LoggerDatabase.Log(INFO, "Ready in %s", time.Since(t))
}
