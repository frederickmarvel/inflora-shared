package events

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	DonationIntentCreated   = "donation.intent.created.v1"
	DonationCharged         = "donation.charged.v1"
	DonationSettled         = "donation.settled.v1"
	DonationFailed          = "donation.failed.v1"
	DonationRefunded        = "donation.refunded.v1"
	DonationReceiptSent     = "donation.receipt_sent.v1"
	PayoutRequested         = "payout.requested.v1"
	PayoutBatched           = "payout.batched.v1"
	PayoutSettled           = "payout.settled.v1"
	PayoutFailed            = "payout.failed.v1"
	PayoutBatchExecuted     = "payout.batch.executed.v1"
	PayoutBatchCompleted    = "payout.batch.completed.v1"
	PayoutBatchFailed       = "payout.batch.failed.v1"
	StreamerRegistered      = "streamer.registered.v1"
	StreamerSettingsUpdated = "streamer.settings.updated.v1"
	OverlayTokenRotated     = "overlay.token.rotated.v1"
	FundHoldCreated         = "fund_hold.created.v1"
	FundHoldReleased        = "fund_hold.released.v1"
	FraudFlagged            = "fraud.flagged.v1"
)

var ErrPublisherClosed = errors.New("events: publisher closed")
var ErrDuplicate = errors.New("events: duplicate event")

// Envelope is the frozen JSON event contract. Payload must be a JSON object.
type Envelope struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	EventVersion  string          `json:"event_version"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Producer      string          `json:"producer"`
	StreamerID    *string         `json:"streamer_id"`
	CausationID   string          `json:"causation_id,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	SchemaURL     string          `json:"schema_url,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

func NewEnvelope(eventType, producer string, streamerID *string, payload any) (Envelope, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("events: marshal payload: %w", err)
	}
	if len(b) == 0 || string(b) == "null" || b[0] != '{' {
		return Envelope{}, errors.New("events: payload must be a non-null object")
	}
	id, err := uuid()
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{EventID: id, EventType: eventType, EventVersion: eventVersion(eventType), OccurredAt: time.Now().UTC(), Producer: producer, StreamerID: streamerID, Payload: b}, nil
}

func Subject(topic, streamerID string) string {
	if strings.TrimSpace(streamerID) == "" {
		return topic
	}
	return topic + "." + streamerID
}

func eventVersion(topic string) string {
	p := strings.Split(topic, ".")
	if len(p) > 0 {
		return strings.TrimPrefix(p[len(p)-1], "v")
	}
	return "1"
}

type Publisher interface {
	Publish(context.Context, string, Envelope) error
	Close() error
}

type MemoryPublisher struct {
	mu     sync.Mutex
	closed bool
	events []Envelope
}

func NewMemoryPublisher() *MemoryPublisher { return &MemoryPublisher{} }
func (p *MemoryPublisher) Publish(_ context.Context, _ string, e Envelope) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrPublisherClosed
	}
	for _, old := range p.events {
		if old.EventID == e.EventID {
			return ErrDuplicate
		}
	}
	p.events = append(p.events, e)
	return nil
}
func (p *MemoryPublisher) Events() []Envelope {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Envelope, len(p.events))
	copy(out, p.events)
	return out
}
func (p *MemoryPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}

func uuid() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("events: event id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}
