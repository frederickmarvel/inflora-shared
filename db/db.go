package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Config controls a PostgreSQL database/sql pool backed by pgx stdlib.
type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	PingTimeout     time.Duration
}

// Open creates and verifies a pgx-backed sql.DB.
func Open(ctx context.Context, cfg Config) (*sql.DB, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("db: DSN is required")
	}
	database, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	if cfg.MaxOpenConns > 0 {
		database.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns >= 0 {
		database.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime >= 0 {
		database.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	pingCtx := ctx
	cancel := func() {}
	if cfg.PingTimeout > 0 {
		pingCtx, cancel = context.WithTimeout(ctx, cfg.PingTimeout)
	}
	defer cancel()
	if err = database.PingContext(pingCtx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return database, nil
}

// InTx executes fn in a serializable transaction. It rolls back on errors and
// panics, and only returns nil after Commit succeeds.
func InTx(ctx context.Context, database *sql.DB, fn func(*sql.Tx) error) (err error) {
	if database == nil {
		return fmt.Errorf("db: nil database")
	}
	if fn == nil {
		return fmt.Errorf("db: nil transaction function")
	}
	tx, err := database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("db: begin transaction: %w", err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback()
			panic(recovered)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("db: commit transaction: %w", err)
	}
	return nil
}
