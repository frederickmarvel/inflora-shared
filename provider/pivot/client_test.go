package pivot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/frederickmarvel/inflora-shared/provider"
)

// TestClientCreateTopUp verifies the real Pivot request wire shape: the OAuth
// token call, then the REDIRECT payment-session call, and the charge-id
// extraction from data.chargeDetails[0].id.
func TestClientCreateTopUp(t *testing.T) {
	var tokenBody, paymentBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/access-token", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-MERCHANT-ID") != "mid" || r.Header.Get("X-MERCHANT-SECRET") != "secret" {
			t.Fatalf("missing merchant auth headers")
		}
		_ = json.NewDecoder(r.Body).Decode(&tokenBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"00","message":"Success","data":{"accessToken":"tok","tokenType":"Bearer","expiresIn":900}}`))
	})
	mux.HandleFunc("/v2/payments", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Fatalf("missing bearer token, got %q", r.Header.Get("Authorization"))
		}
		_ = json.NewDecoder(r.Body).Decode(&paymentBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"00","message":"Success","data":{
			"id":"session-1","clientReferenceId":"donation-1","status":"PENDING",
			"paymentUrl":"https://pay.test/abc","paymentMethod":{"type":"REDIRECT"},
			"chargeDetails":[{"id":"charge-1","status":"PENDING"}]
		}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL, "mid", "secret", "cb-key", "https://inflora.test/tip", nil)
	res, err := c.CreateTopUp(context.Background(), provider.CreateTopUpRequest{
		ExternalID: "donation-1", AmountIDR: 50_000, RequestID: "req-1234567890123456",
		DonorName: "Andi", DonorEmail: "andi@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ChargeID != "charge-1" || res.PaymentURL != "https://pay.test/abc" || res.PaymentMethod != "REDIRECT" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if tokenBody["grantType"] != "client_credentials" {
		t.Fatalf("token body = %v", tokenBody)
	}
	if paymentBody["mode"] != "REDIRECT" || paymentBody["autoConfirm"] != false || paymentBody["paymentType"] != "SINGLE" {
		t.Fatalf("payment body = %v", paymentBody)
	}
	if paymentBody["clientReferenceId"] != "donation-1" {
		t.Fatalf("clientReferenceId = %v", paymentBody["clientReferenceId"])
	}
	amount := paymentBody["amount"].(map[string]any)
	if amount["value"] != "50000" || amount["currency"] != "IDR" {
		t.Fatalf("amount = %v", amount)
	}
	customer := paymentBody["customer"].(map[string]any)
	if customer["givenName"] != "Andi" || customer["email"] != "andi@example.com" {
		t.Fatalf("customer = %v", customer)
	}
}

// TestClientVerifyWebhook verifies the real Pivot callback: X-API-Key auth and
// the nested data shape, with the event preserved for palantir's validation.
func TestClientVerifyWebhook(t *testing.T) {
	c := New("https://unused", "mid", "secret", "cb-key", "", nil)
	body := []byte(`{"event":"PAYMENT.PAID","data":{"clientReferenceId":"donation-1","status":"ACTIVE","chargeDetails":[{"id":"charge-1"}]}}`)
	res, err := c.VerifyWebhook(context.Background(), provider.VerifyWebhookRequest{
		Body:    body,
		Headers: map[string]string{"x-api-key": "cb-key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Valid || res.ExternalID != "donation-1" || res.TransactionID != "charge-1" || res.Status != "ACTIVE" {
		t.Fatalf("unexpected verify result: %+v", res)
	}
	if res.Payload["event"] != "PAYMENT.PAID" {
		t.Fatalf("event not preserved: %v", res.Payload)
	}

	// Wrong key → invalid.
	bad, err := c.VerifyWebhook(context.Background(), provider.VerifyWebhookRequest{
		Body:    body,
		Headers: map[string]string{"x-api-key": "wrong"},
	})
	if err != nil || bad.Valid {
		t.Fatalf("expected invalid, got %+v err=%v", bad, err)
	}
}

// TestClientRequestID ensures the X-REQUEST-ID stays within Pivot's 16-36
// alphanumeric bounds for a short key.
func TestClientRequestID(t *testing.T) {
	if id := requestID("short"); len(id) < 16 || len(id) > 36 {
		t.Fatalf("requestID too short/long: %q", id)
	}
	if strings.IndexFunc(requestID("short"), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-')
	}) != -1 {
		t.Fatalf("requestID has non-alphanumeric chars")
	}
}
