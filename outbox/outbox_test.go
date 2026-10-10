package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/frederickmarvel/inflora-shared/events"
)

type spyPub struct {
	mu        sync.Mutex
	subjects  []string
	envelopes []events.Envelope
	err       error
}

func (s *spyPub) Publish(_ context.Context, subject string, e events.Envelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.subjects = append(s.subjects, subject)
	s.envelopes = append(s.envelopes, e)
	return nil
}

func TestDefaultConsumerName(t *testing.T) {
	t.Parallel()
	if got := DefaultConsumerName("saruman", "pg.gateway.topup.completed.v1"); got != "saruman.pg-gateway-topup-completed-v1" {
		t.Fatalf("consumer name = %q", got)
	}
}

func TestMarshalEnvelopeRoundTrip(t *testing.T) {
	t.Parallel()
	env := events.Envelope{
		EventID:      "11111111-1111-1111-1111-111111111111",
		EventType:    "donation.charged.v1",
		EventVersion: "1",
		OccurredAt:   time.Now().UTC(),
		Producer:     "saruman",
		Payload:      json.RawMessage(`{"x":1}`),
	}
	raw, err := MarshalEnvelope(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded events.Envelope
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.EventID != env.EventID || decoded.EventType != env.EventType {
		t.Fatalf("round trip mismatch")
	}
}

func TestPublisherContractIsUsed(t *testing.T) {
	t.Parallel()
	pub := &spyPub{}
	err := pub.Publish(context.Background(), "donation.charged.v1.abc", events.Envelope{EventID: "x", EventType: "donation.charged.v1"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if len(pub.subjects) != 1 || pub.subjects[0] != "donation.charged.v1.abc" {
		t.Fatalf("subjects = %v", pub.subjects)
	}
}

func TestPublisherPropagatesError(t *testing.T) {
	t.Parallel()
	pub := &spyPub{err: errors.New("boom")}
	if err := pub.Publish(context.Background(), "x", events.Envelope{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestNewDispatcherRequiresProducer(t *testing.T) {
	t.Parallel()
	if _, err := NewDispatcher(&sql.DB{}, &spyPub{}, DispatcherConfig{}); err == nil {
		t.Fatal("expected producer validation error")
	}
}

func TestDispatcherBackoff(t *testing.T) {
	t.Parallel()
	d := &Dispatcher{cfg: DispatcherConfig{BaseBackoff: time.Second, MaxBackoff: 5 * time.Second}}
	for _, tc := range []struct {
		attempt int
		want    time.Duration
	}{
		{1, time.Second}, {2, 2 * time.Second}, {3, 4 * time.Second}, {4, 5 * time.Second}, {20, 5 * time.Second},
	} {
		if got := d.backoff(tc.attempt); got != tc.want {
			t.Errorf("backoff(%d) = %s, want %s", tc.attempt, got, tc.want)
		}
	}
}

func TestDecodeEnvelopeSupportsWrappedAndRaw(t *testing.T) {
	t.Parallel()
	env := events.Envelope{EventID: "11111111-1111-1111-1111-111111111111", EventType: "x.v1", Payload: json.RawMessage(`{"x":1}`)}
	wrapped, err := json.Marshal(EnvelopePayload{Envelope: env})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range [][]byte{wrapped, raw} {
		got, err := decodeEnvelope(input)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.EventID != env.EventID {
			t.Fatalf("event ID = %q", got.EventID)
		}
	}
}

func TestDecodeEnvelopeRejectsMissingEventID(t *testing.T) {
	t.Parallel()
	if _, err := decodeEnvelope([]byte(`{"event_type":"x.v1"}`)); err == nil {
		t.Fatal("expected error")
	}
}
