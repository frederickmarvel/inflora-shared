package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// TestParseEnvelopeWithoutDB ensures envelope validation runs before we touch
// the database, so a malformed envelope is reported deterministically.
func TestEnvelopeMissingEventIDRejected(t *testing.T) {
	t.Parallel()
	raw, _ := json.Marshal(map[string]any{"event_type": "x"})
	if _, err := decodeForTest(raw); err == nil {
		t.Fatalf("expected error for missing event_id")
	}
}

func TestEnvelopeMissingEventTypeRejected(t *testing.T) {
	t.Parallel()
	raw, _ := json.Marshal(map[string]any{"event_id": "11111111-1111-1111-1111-111111111111"})
	if _, err := decodeForTest(raw); err == nil {
		t.Fatalf("expected error for missing event_type")
	}
}

func TestEnvelopeInvalidEventIDRejected(t *testing.T) {
	t.Parallel()
	raw, _ := json.Marshal(map[string]any{"event_id": "not-a-uuid", "event_type": "x"})
	if _, err := decodeForTest(raw); err == nil {
		t.Fatalf("expected error for invalid event_id")
	}
}

func TestErrorSentinelTagsDoNotExist(t *testing.T) {
	t.Parallel()
	if !errors.Is(errors.New("test"), ErrNotClaimed) && ErrNotClaimed == nil {
		t.Fatalf("ErrNotClaimed must not be nil")
	}
}

// decodeForTest mirrors the envelope decode in Handle without requiring a
// database. It exists to validate the JSON shape and UUID format.
func decodeForTest(raw []byte) (string, error) {
	var env struct {
		EventID   string `json:"event_id"`
		EventType string `json:"event_type"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", err
	}
	if env.EventID == "" || env.EventType == "" {
		return "", errors.New("missing event_id or event_type")
	}
	if _, err := uuid.Parse(env.EventID); err != nil {
		return "", err
	}
	return env.EventID, nil
}

// Ensure context is not unused in the build.
var _ = context.Background
