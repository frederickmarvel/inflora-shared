// Package inbox provides the durable consumer inbox primitive shared across
// every Inflora consumer. The consumer_inbox table (Phase 1 schema) is the
// correctness boundary for event handling; in-memory dedup is only a
// performance optimization.
package inbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/frederickmarvel/inflora-shared/events"
	"github.com/google/uuid"
)

// ErrNotClaimed is returned by Handle when the inbox row was claimed
// successfully by another consumer first; the caller should treat it as a
// no-op success (idempotent redelivery).
var ErrNotClaimed = errors.New("inbox: not claimed")

// Handler is the business mutation callback. It runs inside the same
// transaction as the inbox claim. Any error rolls back both the inbox
// claim and the business effect.
type Handler func(ctx context.Context, tx *sql.Tx, env events.Envelope) error

// ClaimParams configures a single inbox Handle call.
type ClaimParams struct {
	ConsumerName string
	Subject      string // for logging / dead-letter context only
	Envelope     []byte // raw envelope JSON bytes
}

// Handle is the durable inbox entry point. It claims (consumer_name, event_id)
// in the same DB transaction as the business callback. Duplicates that arrive
// after a successful claim return ErrNotClaimed, which callers treat as a
// successful no-op.
//
// Required envelope shape: 00-frozen-contracts §1.
func Handle(ctx context.Context, db *sql.DB, consumerName string, rawEnvelope []byte, handler Handler) error {
	if db == nil {
		return fmt.Errorf("inbox: nil database")
	}
	if consumerName == "" {
		return fmt.Errorf("inbox: consumer name required")
	}
	if handler == nil {
		return fmt.Errorf("inbox: nil handler")
	}
	var env events.Envelope
	if err := json.Unmarshal(rawEnvelope, &env); err != nil {
		return fmt.Errorf("inbox: decode envelope: %w", err)
	}
	if env.EventID == "" || env.EventType == "" {
		return fmt.Errorf("inbox: envelope missing event_id or event_type")
	}
	eventID, err := uuid.Parse(env.EventID)
	if err != nil {
		return fmt.Errorf("inbox: invalid event_id: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("inbox: begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var inserted int
	row := tx.QueryRowContext(ctx,
		`INSERT INTO consumer_inbox (consumer_name, event_id, event_type, streamer_id, received_at) VALUES ($1, $2, $3, $4, NOW()) ON CONFLICT (consumer_name, event_id) DO NOTHING RETURNING 1`,
		consumerName, eventID, env.EventType, nullableUUID(env.StreamerID),
	)
	if err := row.Scan(&inserted); err != nil {
		// No rows = duplicate redelivery -> no-op success.
		if errors.Is(err, sql.ErrNoRows) {
			_ = tx.Commit()
			committed = true
			return ErrNotClaimed
		}
		return fmt.Errorf("inbox: claim: %w", err)
	}
	if err := handler(ctx, tx, env); err != nil {
		// Leave attempts++ and let the dispatcher redeliver / dead-letter.
		_, _ = tx.ExecContext(ctx,
			`UPDATE consumer_inbox SET attempts = attempts + 1, last_error = $2 WHERE consumer_name = $1 AND event_id = $3`,
			consumerName, err.Error(), eventID,
		)
		return fmt.Errorf("inbox: handler: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE consumer_inbox SET processed_at = NOW(), attempts = attempts + 1 WHERE consumer_name = $1 AND event_id = $2`,
		consumerName, eventID,
	); err != nil {
		return fmt.Errorf("inbox: mark processed: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("inbox: commit: %w", err)
	}
	committed = true
	return nil
}

// MarkDeadLetter persists a permanent failure to event_dead_letters. The
// caller is responsible for ACKing the NATS message only after this
// commits. Replay uses the same event_id; the (consumer_name, event_id)
// unique key guarantees idempotency.
func MarkDeadLetter(ctx context.Context, db *sql.DB, consumerName string, rawEnvelope []byte, subject string, attempts int, errClass string, errMsg string) error {
	var env events.Envelope
	if err := json.Unmarshal(rawEnvelope, &env); err != nil {
		return fmt.Errorf("dead-letter: decode envelope: %w", err)
	}
	eventID, err := uuid.Parse(env.EventID)
	if err != nil {
		return fmt.Errorf("dead-letter: invalid event_id: %w", err)
	}
	if errClass == "" {
		errClass = "PERMANENT"
	}
	now := time.Now().UTC()
	_, err = db.ExecContext(ctx, `
		INSERT INTO event_dead_letters (
			consumer_name, event_id, subject, envelope, attempt_count, error_class,
			last_error, first_failed_at, last_failed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		ON CONFLICT (consumer_name, event_id) DO UPDATE SET
			attempt_count = GREATEST(event_dead_letters.attempt_count, EXCLUDED.attempt_count),
			last_error     = EXCLUDED.last_error,
			last_failed_at = EXCLUDED.last_failed_at
	`, consumerName, eventID, subject, rawEnvelope, attempts, errClass, errMsg, now)
	if err != nil {
		return fmt.Errorf("dead-letter: upsert: %w", err)
	}
	return nil
}

func nullableUUID(s *string) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}
