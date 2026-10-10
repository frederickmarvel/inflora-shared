// Package outbox provides transactional outbox writing and dispatch.
package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/frederickmarvel/inflora-shared/events"
	"github.com/google/uuid"
)

// Publisher must return nil only after JetStream acknowledges the publish. Its
// implementation must use Envelope.EventID as Nats-Msg-Id for deduplication.
type Publisher interface {
	Publish(context.Context, string, events.Envelope) error
}

// DispatcherConfig controls PostgreSQL leasing and retry behavior.
type DispatcherConfig struct {
	WorkerName     string
	Producer       string
	PollInterval   time.Duration
	ClaimTTL       time.Duration
	PublishTimeout time.Duration
	MaxAttempts    int
	BaseBackoff    time.Duration
	MaxBackoff     time.Duration
}

// Dispatcher is compatible with runtime.Worker.
type Dispatcher struct {
	db        *sql.DB
	publisher Publisher
	cfg       DispatcherConfig
	now       func() time.Time
}

type claimedEvent struct {
	eventID   uuid.UUID
	subject   string
	envelope  json.RawMessage
	attempts  int
	claimedAt time.Time
}

// NewDispatcher constructs a database-backed dispatcher.
func NewDispatcher(db *sql.DB, publisher Publisher, cfg DispatcherConfig) (*Dispatcher, error) {
	if db == nil || publisher == nil {
		return nil, errors.New("outbox: database and publisher are required")
	}
	if strings.TrimSpace(cfg.Producer) == "" {
		return nil, errors.New("outbox: producer is required")
	}
	if cfg.WorkerName == "" {
		cfg.WorkerName = cfg.Producer + "-outbox"
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 250 * time.Millisecond
	}
	if cfg.ClaimTTL <= 0 {
		cfg.ClaimTTL = 30 * time.Second
	}
	if cfg.PublishTimeout <= 0 {
		cfg.PublishTimeout = 10 * time.Second
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 10
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = time.Second
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 5 * time.Minute
	}
	if cfg.MaxBackoff < cfg.BaseBackoff {
		cfg.MaxBackoff = cfg.BaseBackoff
	}
	return &Dispatcher{db: db, publisher: publisher, cfg: cfg, now: time.Now}, nil
}

func (d *Dispatcher) Name() string { return d.cfg.WorkerName }

// Run dispatches due rows until cancellation. Durable publish failures are
// rescheduled and do not terminate the worker; database failures do.
func (d *Dispatcher) Run(ctx context.Context) error {
	for {
		didWork, err := d.DispatchOne(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if didWork {
			continue
		}
		timer := time.NewTimer(d.cfg.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// DispatchOne claims and processes one due event. The claim transaction ends
// before network I/O, preventing slow JetStream acknowledgements from holding
// PostgreSQL locks.
func (d *Dispatcher) DispatchOne(ctx context.Context) (bool, error) {
	event, ok, err := d.claimOne(ctx)
	if err != nil || !ok {
		return ok, err
	}
	env, publishErr := decodeEnvelope(event.envelope)
	if publishErr == nil {
		publishCtx, cancel := context.WithTimeout(ctx, d.cfg.PublishTimeout)
		publishErr = d.publisher.Publish(publishCtx, event.subject, env)
		cancel()
	}
	if publishErr != nil {
		if ctx.Err() != nil {
			return true, ctx.Err()
		}
		if err := d.fail(ctx, event, publishErr); err != nil {
			return true, err
		}
		return true, nil
	}
	res, err := d.db.ExecContext(ctx, `
		UPDATE outbox_events
		SET published_at=$4, claimed_at=NULL, claimed_by=NULL, last_error=NULL
		WHERE event_id=$1 AND claimed_by=$2 AND claimed_at=$3 AND published_at IS NULL
	`, event.eventID, d.cfg.WorkerName, event.claimedAt, d.now().UTC())
	if err != nil {
		return true, fmt.Errorf("outbox: mark published: %w", err)
	}
	return true, requireOne(res, event.eventID)
}

func (d *Dispatcher) claimOne(ctx context.Context) (claimedEvent, bool, error) {
	now := d.now().UTC()
	var event claimedEvent
	err := d.db.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT event_id FROM outbox_events
			WHERE producer=$1 AND published_at IS NULL AND available_at <= $2
			  AND (claimed_at IS NULL OR claimed_at < $3)
			ORDER BY available_at, event_id
			LIMIT 1 FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox_events o
		SET claimed_at=$2, claimed_by=$4, attempts=o.attempts+1
		FROM candidate WHERE o.event_id=candidate.event_id
		RETURNING o.event_id,o.subject,o.envelope,o.attempts,o.claimed_at
	`, d.cfg.Producer, now, now.Add(-d.cfg.ClaimTTL), d.cfg.WorkerName).Scan(
		&event.eventID, &event.subject, &event.envelope, &event.attempts, &event.claimedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return claimedEvent{}, false, nil
	}
	if err != nil {
		return claimedEvent{}, false, fmt.Errorf("outbox: claim: %w", err)
	}
	return event, true, nil
}

func (d *Dispatcher) fail(ctx context.Context, event claimedEvent, publishErr error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("outbox: begin failure: %w", err)
	}
	defer tx.Rollback()
	now := d.now().UTC()
	var res sql.Result
	if event.attempts >= d.cfg.MaxAttempts {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO event_dead_letters
			(consumer_name,event_id,subject,envelope,attempt_count,error_class,last_error,first_failed_at,last_failed_at)
			VALUES ($1,$2,$3,$4,$5,'OUTBOX_PUBLISH',$6,$7,$7)
			ON CONFLICT (consumer_name,event_id) DO UPDATE SET
			attempt_count=GREATEST(event_dead_letters.attempt_count,EXCLUDED.attempt_count),
			last_error=EXCLUDED.last_error,last_failed_at=EXCLUDED.last_failed_at
		`, d.cfg.Producer+".outbox", event.eventID, event.subject, event.envelope, event.attempts, publishErr.Error(), now)
		if err == nil {
			res, err = tx.ExecContext(ctx, `
				UPDATE outbox_events SET dead_lettered_at=$4,claimed_at=NULL,claimed_by=NULL,last_error=$5
				WHERE event_id=$1 AND claimed_by=$2 AND claimed_at=$3
				  AND published_at IS NULL AND dead_lettered_at IS NULL
			`, event.eventID, d.cfg.WorkerName, event.claimedAt, now, publishErr.Error())
		}
	} else {
		res, err = tx.ExecContext(ctx, `
			UPDATE outbox_events SET available_at=$4,claimed_at=NULL,claimed_by=NULL,last_error=$5
			WHERE event_id=$1 AND claimed_by=$2 AND claimed_at=$3 AND published_at IS NULL
		`, event.eventID, d.cfg.WorkerName, event.claimedAt, now.Add(d.backoff(event.attempts)), publishErr.Error())
	}
	if err != nil {
		return fmt.Errorf("outbox: record failure: %w", err)
	}
	if err := requireOne(res, event.eventID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("outbox: commit failure: %w", err)
	}
	return nil
}

func requireOne(result sql.Result, eventID uuid.UUID) error {
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("outbox: rows affected: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("outbox: claim ownership lost for %s", eventID)
	}
	return nil
}

func (d *Dispatcher) backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := d.cfg.BaseBackoff
	for i := 1; i < attempt && delay < d.cfg.MaxBackoff; i++ {
		if delay > d.cfg.MaxBackoff/2 {
			return d.cfg.MaxBackoff
		}
		delay *= 2
	}
	if delay > d.cfg.MaxBackoff {
		return d.cfg.MaxBackoff
	}
	return delay
}

func decodeEnvelope(raw json.RawMessage) (events.Envelope, error) {
	var wrapped EnvelopePayload
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Envelope.EventID != "" {
		return wrapped.Envelope, nil
	}
	var env events.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return env, fmt.Errorf("outbox: decode envelope: %w", err)
	}
	if env.EventID == "" {
		return env, errors.New("outbox: envelope missing event_id")
	}
	return env, nil
}

func MarshalEnvelope(env events.Envelope) ([]byte, error) {
	b, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("outbox: marshal envelope: %w", err)
	}
	return b, nil
}

func DefaultConsumerName(producer, eventType string) string {
	return fmt.Sprintf("%s.%s", producer, strings.ReplaceAll(eventType, ".", "-"))
}

var uuidFn = uuid.New

func NewUUID() string { return uuidFn().String() }
