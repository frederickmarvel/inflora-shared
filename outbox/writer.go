// Package outbox provides the transactional outbox writer and dispatcher
// primitives shared across all Inflora producers (saruman, palantir,
// tolkien, ingest, ws-gateway). See almanac/planning/AI_BACKEND_COMPLETION_PLAN.md
// §10 (Phase 1) and §11 (Phase 2).
package outbox

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

// ErrTxRequired is returned when a caller invokes Enqueue without an open
// transaction.
var ErrTxRequired = errors.New("outbox: enqueue requires an open *sql.Tx")

// EnvelopePayload is the on-disk envelope row. It carries the full frozen
// event envelope from 00-frozen-contracts §1 plus producer metadata needed
// by the dispatcher.
type EnvelopePayload struct {
	Envelope events.Envelope `json:"envelope"`
}

// Enqueue inserts an outbox row inside the caller's transaction so the
// business state and the event publish intent are committed atomically. The
// caller MUST commit (or roll back) the transaction.
//
// The function:
//   - Validates that the payload is a non-null JSON object.
//   - Generates a UUIDv4 event_id when the envelope's EventID is empty.
//   - Inserts into outbox_events with available_at = NOW() + 0 (ready).
//   - Returns the assigned event_id so callers can reference it (e.g. as
//     correlation_id in the same transaction).
func Enqueue(ctx context.Context, tx *sql.Tx, producer, aggregateType string, aggregateID uuid.UUID, streamerID *uuid.UUID, env events.Envelope) (uuid.UUID, error) {
	if tx == nil {
		return uuid.Nil, ErrTxRequired
	}
	if env.EventID == "" {
		env.EventID = uuid.NewString()
	}
	if env.EventType == "" || env.Producer == "" || env.OccurredAt.IsZero() {
		return uuid.Nil, fmt.Errorf("outbox: envelope missing required field (event_type=%q occurred_at=%v)", env.EventType, env.OccurredAt)
	}
	if !json.Valid(env.Payload) || len(env.Payload) == 0 || env.Payload[0] != '{' {
		return uuid.Nil, fmt.Errorf("outbox: payload must be a non-null JSON object")
	}
	env.OccurredAt = env.OccurredAt.UTC()

	envelope := EnvelopePayload{Envelope: env}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return uuid.Nil, fmt.Errorf("outbox: marshal envelope: %w", err)
	}

	subject := events.Subject(env.EventType, "")
	if env.StreamerID != nil && *env.StreamerID != "" {
		subject = events.Subject(env.EventType, *env.StreamerID)
	}

	var streamer sql.NullString
	if streamerID != nil {
		streamer = sql.NullString{String: streamerID.String(), Valid: true}
	}

	eventID := uuid.MustParse(env.EventID)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO outbox_events (
			event_id, producer, event_type, event_version, aggregate_type, aggregate_id,
			streamer_id, subject, envelope, occurred_at, available_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW())
	`, eventID, producer, env.EventType, env.EventVersion, aggregateType, aggregateID, streamer, subject, raw, env.OccurredAt); err != nil {
		return uuid.Nil, fmt.Errorf("outbox: insert: %w", err)
	}
	return eventID, nil
}

// EnqueueRaw inserts an outbox row using caller-built columns. Use this
// when the caller already has pre-marshaled bytes for envelope / subject /
// version. Validation rules from Enqueue still apply.
func EnqueueRaw(ctx context.Context, tx *sql.Tx, params RawParams) (uuid.UUID, error) {
	if tx == nil {
		return uuid.Nil, ErrTxRequired
	}
	if params.EventID == uuid.Nil {
		params.EventID = uuid.New()
	}
	if params.OccurredAt.IsZero() {
		params.OccurredAt = time.Now().UTC()
	}
	if params.AvailableAt.IsZero() {
		params.AvailableAt = params.OccurredAt
	}
	if !json.Valid(params.Envelope) || len(params.Envelope) == 0 || params.Envelope[0] != '{' {
		return uuid.Nil, fmt.Errorf("outbox: envelope must be a non-null JSON object")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO outbox_events (
			event_id, producer, event_type, event_version, aggregate_type, aggregate_id,
			streamer_id, subject, envelope, occurred_at, available_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`,
		params.EventID, params.Producer, params.EventType, params.EventVersion, params.AggregateType, params.AggregateID,
		params.StreamerID, params.Subject, params.Envelope, params.OccurredAt, params.AvailableAt,
	); err != nil {
		return uuid.Nil, fmt.Errorf("outbox: insert raw: %w", err)
	}
	return params.EventID, nil
}

// RawParams is the input shape for EnqueueRaw.
type RawParams struct {
	EventID       uuid.UUID
	Producer      string
	EventType     string
	EventVersion  string
	AggregateType string
	AggregateID   uuid.UUID
	StreamerID    sql.NullString
	Subject       string
	Envelope      json.RawMessage
	OccurredAt    time.Time
	AvailableAt   time.Time
}
