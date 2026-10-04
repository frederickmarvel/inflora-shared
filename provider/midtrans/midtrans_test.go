package midtrans

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"github.com/frederickmarvel/inflora-shared/provider"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateTopUp(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, _, ok := r.BasicAuth(); !ok || u != "key" {
			t.Error("missing auth")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"redirect_url":"https://pay","transaction_id":"tx1","transaction_status":"pending"}`))
	}))
	defer s.Close()
	got, err := New(s.URL, "key", s.Client()).CreateTopUp(context.Background(), provider.CreateTopUpRequest{ExternalID: "o1", AmountIDR: 100})
	if err != nil || got.ChargeID != "tx1" || got.PaymentURL != "https://pay" {
		t.Fatalf("%#v %v", got, err)
	}
}
func TestVerifyWebhookSHA512(t *testing.T) {
	body := []byte(`{"order_id":"o1","status_code":"200","gross_amount":"100.00","transaction_status":"settlement"}`)
	h := sha512.Sum512([]byte("o1" + "200" + "100.00" + "key"))
	got, err := New("", "key", nil).VerifyWebhook(context.Background(), provider.VerifyWebhookRequest{Body: body, Signature: hex.EncodeToString(h[:])})
	if err != nil || !got.Valid || got.Status != "settlement" {
		t.Fatalf("%#v %v", got, err)
	}
}
