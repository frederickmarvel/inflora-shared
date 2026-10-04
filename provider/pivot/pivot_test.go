package pivot

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"github.com/frederickmarvel/inflora-shared/provider"
	"testing"
	"time"
)

func TestStubDeterministic(t *testing.T) {
	s := NewStub("secret")
	s.Now = func() time.Time { return time.Unix(1, 0) }
	r := provider.CreateTopUpRequest{ExternalID: "d1", RequestID: "r1"}
	a, _ := s.CreateTopUp(context.Background(), r)
	b, _ := s.CreateTopUp(context.Background(), r)
	if a != b || a.Status != "PENDING" {
		t.Fatalf("not deterministic: %#v %#v", a, b)
	}
}
func TestVerifyWebhook(t *testing.T) {
	s := NewStub("secret")
	body := []byte(`{"external_id":"d1","status":"settled"}`)
	m := hmac.New(sha256.New, []byte("secret"))
	m.Write(body)
	r, err := s.VerifyWebhook(context.Background(), provider.VerifyWebhookRequest{Body: body, Signature: hex.EncodeToString(m.Sum(nil))})
	if err != nil || !r.Valid || r.ExternalID != "d1" {
		t.Fatalf("%#v %v", r, err)
	}
}
