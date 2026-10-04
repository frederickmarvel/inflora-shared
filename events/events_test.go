package events

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestEnvelopeContract(t *testing.T) {
	streamerID := "abc-123"
	e, err := NewEnvelope(DonationIntentCreated, "ingest-api", &streamerID, map[string]any{"amount_idr": int64(10000)})
	if err != nil {
		t.Fatal(err)
	}
	if e.EventVersion != "1" || e.EventType != DonationIntentCreated || e.EventID == "" {
		t.Fatalf("unexpected envelope: %+v", e)
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"event_version":"1"`) || !strings.Contains(string(b), `"payload":{"amount_idr":10000}`) {
		t.Fatalf("wire contract mismatch: %s", b)
	}
}

func TestNewEnvelopeRejectsNonObjectPayload(t *testing.T) {
	for _, payload := range []any{nil, []string{"bad"}, "bad"} {
		if _, err := NewEnvelope(DonationCharged, "saruman", nil, payload); err == nil {
			t.Fatalf("accepted payload %#v", payload)
		}
	}
}

func TestCatalogAndSubject(t *testing.T) {
	topics := []string{DonationIntentCreated, DonationCharged, DonationSettled, DonationFailed, DonationRefunded, DonationReceiptSent, PayoutRequested, PayoutBatched, PayoutSettled, PayoutFailed, PayoutBatchExecuted, PayoutBatchCompleted, PayoutBatchFailed, StreamerRegistered, StreamerSettingsUpdated, OverlayTokenRotated, FundHoldCreated, FundHoldReleased, FraudFlagged}
	if len(topics) != 19 {
		t.Fatalf("got %d catalog topics", len(topics))
	}
	seen := map[string]bool{}
	for _, topic := range topics {
		if seen[topic] || !strings.HasSuffix(topic, ".v1") {
			t.Fatalf("invalid or duplicate topic %q", topic)
		}
		seen[topic] = true
	}
	if got := Subject(DonationCharged, "streamer-id"); got != "donation.charged.v1.streamer-id" {
		t.Fatal(got)
	}
}

func TestMemoryPublisherDeduplicatesEventID(t *testing.T) {
	p := NewMemoryPublisher()
	e, err := NewEnvelope(FraudFlagged, "saruman", nil, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Publish(context.Background(), Subject(e.EventType, ""), e); err != nil {
		t.Fatal(err)
	}
	if err = p.Publish(context.Background(), Subject(e.EventType, ""), e); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("got %v", err)
	}
	if len(p.Events()) != 1 {
		t.Fatal("duplicate was recorded")
	}
}
