package midtrans

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/frederickmarvel/inflora-shared/provider"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL, ServerKey string
	HTTP               *http.Client
}

func New(baseURL, serverKey string, h *http.Client) *Client {
	if h == nil {
		h = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{strings.TrimRight(baseURL, "/"), serverKey, h}
}
func (c *Client) post(ctx context.Context, path string, in, out interface{}) error {
	b, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.ServerKey, "")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		x, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("midtrans: status %d: %s", res.StatusCode, x)
	}
	return json.NewDecoder(res.Body).Decode(out)
}
func (c *Client) CreateTopUp(ctx context.Context, r provider.CreateTopUpRequest) (provider.CreateTopUpResult, error) {
	var x struct {
		Token         string `json:"token"`
		RedirectURL   string `json:"redirect_url"`
		TransactionID string `json:"transaction_id"`
		Status        string `json:"transaction_status"`
	}
	err := c.post(ctx, "/snap/v1/transactions", map[string]interface{}{"transaction_details": map[string]interface{}{"order_id": r.ExternalID, "gross_amount": r.AmountIDR}, "customer_details": map[string]string{"first_name": r.DonorName, "email": r.DonorEmail}}, &x)
	return provider.CreateTopUpResult{ChargeID: first(x.TransactionID, r.ExternalID), PaymentURL: x.RedirectURL, Status: first(x.Status, "PENDING")}, err
}
func (c *Client) CreateWithdrawal(ctx context.Context, r provider.CreateWithdrawalRequest) (provider.CreateWithdrawalResult, error) {
	var x struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	err := c.post(ctx, "/v1/payouts", r, &x)
	return provider.CreateWithdrawalResult{WithdrawalID: x.ID, Status: x.Status}, err
}
func (c *Client) CreateRefund(ctx context.Context, r provider.CreateRefundRequest) (provider.CreateRefundResult, error) {
	var x struct {
		RefundKey string `json:"refund_key"`
		Status    string `json:"transaction_status"`
	}
	err := c.post(ctx, "/v2/"+r.OriginalChargeID+"/refund", map[string]interface{}{"refund_key": r.RequestID, "amount": r.AmountIDR}, &x)
	return provider.CreateRefundResult{RefundID: x.RefundKey, Status: x.Status}, err
}
func (c *Client) VerifyWebhook(_ context.Context, r provider.VerifyWebhookRequest) (provider.VerifyWebhookResult, error) {
	var p map[string]interface{}
	if err := json.Unmarshal(r.Body, &p); err != nil {
		return provider.VerifyWebhookResult{}, err
	}
	sig := r.Signature
	if sig == "" {
		sig, _ = p["signature_key"].(string)
	}
	order, _ := p["order_id"].(string)
	code, _ := p["status_code"].(string)
	gross, _ := p["gross_amount"].(string)
	a := sha512.Sum512([]byte(order + code + gross + c.ServerKey))
	b := sha256.Sum256(append(append([]byte{}, r.Body...), []byte(c.ServerKey)...))
	valid := equal(sig, hex.EncodeToString(a[:])) || equal(sig, hex.EncodeToString(b[:]))
	status, _ := p["transaction_status"].(string)
	tx, _ := p["transaction_id"].(string)
	return provider.VerifyWebhookResult{Valid: valid, ExternalID: order, TransactionID: tx, Status: status, Payload: p}, nil
}
func equal(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(strings.ToLower(a)), []byte(strings.ToLower(b))) == 1
}
func first(v, d string) string {
	if v != "" {
		return v
	}
	return d
}
