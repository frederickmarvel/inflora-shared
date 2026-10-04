package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

const migrationTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
version text PRIMARY KEY,
applied_at timestamptz NOT NULL DEFAULT now()
)`

// Migrate applies pending .sql files from migrationsFS beneath dir in lexical
// order. Each file is atomic and recorded by file name. Plain SQL and the UP
// section of files using "-- +goose Up" or "-- migrate:up" are supported.
func Migrate(ctx context.Context, database *sql.DB, migrationsFS fs.FS, dir string) error {
	if database == nil {
		return fmt.Errorf("db: nil database")
	}
	if migrationsFS == nil {
		return fmt.Errorf("db: nil migration filesystem")
	}
	if dir == "" {
		dir = "."
	}
	if _, err := database.ExecContext(ctx, migrationTable); err != nil {
		return fmt.Errorf("db: create migration table: %w", err)
	}
	entries, err := fs.ReadDir(migrationsFS, dir)
	if err != nil {
		return fmt.Errorf("db: read migrations: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var applied bool
		err = database.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, name).Scan(&applied)
		if err != nil {
			return fmt.Errorf("db: check migration %s: %w", name, err)
		}
		if applied {
			continue
		}
		body, readErr := fs.ReadFile(migrationsFS, path.Join(dir, name))
		if readErr != nil {
			return fmt.Errorf("db: read migration %s: %w", name, readErr)
		}
		upSQL := upSection(string(body))
		if strings.TrimSpace(upSQL) == "" {
			return fmt.Errorf("db: migration %s has empty up section", name)
		}
		err = InTx(ctx, database, func(tx *sql.Tx) error {
			if _, execErr := tx.ExecContext(ctx, upSQL); execErr != nil {
				return fmt.Errorf("execute: %w", execErr)
			}
			if _, execErr := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES ($1, $2)`, name, time.Now().UTC()); execErr != nil {
				return fmt.Errorf("record: %w", execErr)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("db: apply migration %s: %w", name, err)
		}
	}
	return nil
}

func upSection(body string) string {
	lines := strings.Split(body, "\n")
	start, end := 0, len(lines)
	for i, line := range lines {
		marker := strings.ToLower(strings.TrimSpace(line))
		switch marker {
		case "-- +goose up", "-- migrate:up":
			start = i + 1
		case "-- +goose down", "-- migrate:down":
			if i >= start {
				end = i
				return strings.Join(lines[start:end], "\n")
			}
		}
	}
	return strings.Join(lines[start:end], "\n")
}
