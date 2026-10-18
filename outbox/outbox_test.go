package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/frederickmarvel/inflora-shared/events"
	"github.com/google/uuid"
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

type fakeResult struct{ rows int64 }

func (f fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (f fakeResult) RowsAffected() (int64, error) { return f.rows, nil }

// TestRequireOneContract locks in the post-publish/post-fail CAS
// semantics: 0 rows = non-fatal skip, 1 row = success, anything else = bug.
// The bug we're guarding against is the previous behavior where 0 rows
// returned a fatal "claim ownership lost" error that killed the worker
// and took the whole service down on a duplicate-completion race.
func TestRequireOneContract(t *testing.T) {
	t.Parallel()
	eid := uuid.New()
	cases := []struct {
		name     string
		rows     int64
		wantSkip bool
		wantErr  bool
	}{
		{"zero rows is a non-fatal skip", 0, true, false},
		{"one row is success", 1, false, false},
		{"two rows is a bug", 2, false, true},
		{"negative rows is a bug", -1, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			skip, err := requireOne(fakeResult{rows: c.rows}, eid)
			if skip != c.wantSkip {
				t.Errorf("skip = %v, want %v", skip, c.wantSkip)
			}
			gotErr := err != nil
			if gotErr != c.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, c.wantErr)
			}
			if c.wantErr && c.rows > 0 {
				if !strings.Contains(err.Error(), eid.String()) {
					t.Errorf("error %q should include event ID %s for debuggability", err, eid)
				}
			}
		})
	}
}
