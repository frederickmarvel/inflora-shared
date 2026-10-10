package pivot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/frederickmarvel/inflora-shared/provider"
)

// Client is the real Pivot payment adapter. It implements provider.Provider
// against Pivot's OAuth + Payment Session + Callback APIs. See
// almanac/docs/Pivot_techdocs.md for the wire contract.
//
// Money is integer IDR throughout: Pivot amounts use { value, currency }
// objects whose value is a number in major units; here the provider interface
// speaks minor units (whole rupiah), which are identical to Pivot's "value"
// because IDR has no fraction in the sandbox examples.
type Client struct {
	// BaseURL is the Pivot API base, e.g. https://sandbox-api.pivot-payments.com.
	BaseURL string
	// MerchantID and MerchantSecret authenticate the OAuth access-token call.
	MerchantID, MerchantSecret string
	// CallbackKey is the dedicated Callback API Key (X-API-Key) used to verify
	// webhooks. It is separate from the OAuth credentials.
	CallbackKey string
	// RedirectURL is the donor-facing base the hosted payment page returns to,
	// e.g. https://inflora.app/tip. Per-donation paths are appended on demand.
	RedirectURL string

	HTTP *http.Client

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// New builds a Client with the given base URL and credentials. h may be nil.
func New(baseURL, merchantID, merchantSecret, callbackKey, redirectURL string, h *http.Client) *Client {
	if h == nil {
		h = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		BaseURL:        strings.TrimRight(baseURL, "/"),
		MerchantID:     merchantID,
		MerchantSecret: merchantSecret,
		CallbackKey:    callbackKey,
		RedirectURL:    strings.TrimRight(redirectURL, "/"),
		HTTP:           h,
	}
}

// requestID derives a Pivot X-REQUEST-ID from a stable business key. Pivot
// requires 16-36 alphanumeric characters (no dashes or underscores), so the
// operation key is hashed to a deterministic 32-char hex string.
func requestID(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:16])
}

func (c *Client) do(ctx context.Context, method, path string, headers map[string]string, in, out interface{}) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("pivot: marshal request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	// Pivot returns a 2xx envelope { code, message, data } on success, and
	// plain JSON errors otherwise. Accept only 2xx.
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("pivot: status %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("pivot: decode response: %w", err)
	}
	return nil
}

// fetchAccessToken returns a cached OAuth token, fetching a fresh one when
// absent or expired. Tokens live 900 seconds; we refresh a minute early.
func (c *Client) fetchAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accessToken != "" && time.Now().Add(60*time.Second).Before(c.expiresAt) {
		return c.accessToken, nil
	}
	var out struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Data    struct {
			AccessToken string `json:"accessToken"`
			TokenType   string `json:"tokenType"`
			ExpiresIn   int    `json:"expiresIn"`
		} `json:"data"`
	}
	err := c.do(ctx, http.MethodPost, "/v1/access-token", map[string]string{
		"X-MERCHANT-ID":     c.MerchantID,
		"X-MERCHANT-SECRET": c.MerchantSecret,
	}, map[string]string{"grantType": "client_credentials"}, &out)
	if err != nil {
		return "", err
	}
	token := out.Data.AccessToken
	if token == "" {
		return "", errors.New("pivot: access-token response missing accessToken")
	}
	expiresIn := out.Data.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 900
	}
	c.accessToken = token
	c.expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	return token, nil
}

type pivotAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type pivotPaymentSession struct {
	ID               string `json:"id"`
	ClientReference  string `json:"clientReferenceId"`
	Status           string `json:"status"`
	PaymentURL       string `json:"paymentUrl"`
	ExpiryAt         string `json:"expiryAt"`
	PaymentMethodRaw struct {
		Type string `json:"type"`
	} `json:"paymentMethod"`
	ChargeDetails []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"chargeDetails"`
}

func (c *Client) CreateTopUp(ctx context.Context, r provider.CreateTopUpRequest) (provider.CreateTopUpResult, error) {
	token, err := c.fetchAccessToken(ctx)
	if err != nil {
		return provider.CreateTopUpResult{}, err
	}
	success, failure, expiration := r.SuccessReturnURL, r.FailureReturnURL, r.ExpirationURL
	if success == "" && failure == "" && expiration == "" {
		success, failure, expiration = c.redirectURLs(r.ExternalID, c.RedirectURL)
	}

	body := map[string]any{
		"clientReferenceId": r.ExternalID,
		"amount":            pivotAmount{Value: strconv.FormatInt(r.AmountIDR, 10), Currency: "IDR"},
		"paymentType":       "SINGLE",
		"mode":              "REDIRECT",
		// autoConfirm=false and no paymentMethod → Pivot redirects the donor to
		// its "List of Payment UI" so they can pick QRIS/VA/ewallet/card.
		"autoConfirm": false,
	}
	if success != "" || failure != "" || expiration != "" {
		body["redirectUrl"] = map[string]string{
			"successReturnUrl":    success,
			"failureReturnUrl":    failure,
			"expirationReturnUrl": expiration,
		}
	}
	if r.DonorName != "" || r.DonorEmail != "" {
		customer := map[string]any{}
		if r.DonorName != "" {
			customer["givenName"] = r.DonorName
		}
		if r.DonorEmail != "" {
			customer["email"] = r.DonorEmail
		}
		body["customer"] = customer
	}
	if r.ChannelCode != "" {
		body["statementDescriptor"] = r.ChannelCode[:minInt(len(r.ChannelCode), 20)]
	}
	reqID := requestID(r.RequestID)
	var envelope struct {
		Code    string              `json:"code"`
		Message string              `json:"message"`
		Data    pivotPaymentSession `json:"data"`
	}
	if err := c.do(ctx, http.MethodPost, "/v2/payments", map[string]string{
		"Authorization": "Bearer " + token,
		"X-REQUEST-ID":  reqID,
	}, body, &envelope); err != nil {
		return provider.CreateTopUpResult{}, err
	}
	session := envelope.Data
	chargeID := session.ID
	if len(session.ChargeDetails) > 0 && session.ChargeDetails[0].ID != "" {
		chargeID = session.ChargeDetails[0].ID
	}
	expiresAt := time.Time{}
	if t, err := time.Parse(time.RFC3339, session.ExpiryAt); err == nil {
		expiresAt = t.UTC()
	}
	status := strings.ToUpper(strings.TrimSpace(session.Status))
	if status == "" {
		status = "PENDING"
	}
	method := strings.ToUpper(strings.TrimSpace(session.PaymentMethodRaw.Type))
	if method == "" {
		method = "REDIRECT"
	}
	return provider.CreateTopUpResult{
		ChargeID:      chargeID,
		PaymentURL:    session.PaymentURL,
		Status:        status,
		PaymentMethod: method,
		ExpiresAt:     expiresAt,
	}, nil
}

// redirectURLs builds the three return URLs for a donation. When a base is set
// it appends the donation id so the frontend can correlate the returning
// browser; otherwise the URLs stay empty and Pivot falls back to its defaults.
func (c *Client) redirectURLs(externalID, base string) (success, failure, expiration string) {
	if base == "" {
		return "", "", ""
	}
	q := "?intent_id=" + externalID + "&result="
	return base + "/success" + q + "success",
		base + "/failure" + q + "failure",
		base + "/expiration" + q + "expiration"
}

func (c *Client) CreateWithdrawal(context.Context, provider.CreateWithdrawalRequest) (provider.CreateWithdrawalResult, error) {
	return provider.CreateWithdrawalResult{}, provider.ErrNotImplemented
}

func (c *Client) CreateRefund(context.Context, provider.CreateRefundRequest) (provider.CreateRefundResult, error) {
	return provider.CreateRefundResult{}, provider.ErrNotImplemented
}

// VerifyWebhook authenticates a Pivot callback. Pivot signs webhooks with the
// dedicated Callback API Key in the X-API-Key header, and the body carries
// { event, data: PaymentSessionObject }. ExternalID is data.clientReferenceId,
// TransactionID is data.chargeDetails[0].id (the charge id), and Status is
// data.status. The event name is preserved in Payload so palantir can validate
// it against the request's provider_event_id.
func (c *Client) VerifyWebhook(_ context.Context, r provider.VerifyWebhookRequest) (provider.VerifyWebhookResult, error) {
	out := provider.VerifyWebhookResult{}
	got := strings.TrimSpace(r.Headers["x-api-key"])
	if got == "" {
		// Accept the explicit signature field as a convenience for tests.
		got = strings.TrimSpace(r.Signature)
	}
	valid := c.CallbackKey != "" && got != "" &&
		len(got) == len(c.CallbackKey) &&
		subtle.ConstantTimeCompare([]byte(got), []byte(c.CallbackKey)) == 1
	out.Valid = valid
	if !valid {
		return out, nil
	}
	var p struct {
		Event string              `json:"event"`
		Data  pivotPaymentSession `json:"data"`
	}
	if err := json.Unmarshal(r.Body, &p); err != nil {
		return provider.VerifyWebhookResult{}, err
	}
	out.ExternalID = p.Data.ClientReference
	if len(p.Data.ChargeDetails) > 0 {
		out.TransactionID = p.Data.ChargeDetails[0].ID
	}
	out.Status = p.Data.Status
	out.Payload = map[string]interface{}{
		"event":            p.Event,
		"client_reference": p.Data.ClientReference,
		"status":           p.Data.Status,
		"charge_id":        out.TransactionID,
	}
	return out, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
