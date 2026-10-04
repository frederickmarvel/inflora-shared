package pivot

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/frederickmarvel/inflora-shared/provider"
	"time"
)

type Stub struct {
	Secret string
	Now    func() time.Time
}

func NewStub(secret string) *Stub { return &Stub{Secret: secret, Now: time.Now} }
func id(kind, s string) string {
	h := sha256.Sum256([]byte(kind + ":" + s))
	return fmt.Sprintf("pivot_%s_%x", kind, h[:8])
}
func (s *Stub) CreateTopUp(_ context.Context, r provider.CreateTopUpRequest) (provider.CreateTopUpResult, error) {
	n := s.Now().UTC()
	cid := id("topup", r.ExternalID+":"+r.RequestID)
	return provider.CreateTopUpResult{ChargeID: cid, PaymentURL: "stub://pivot/pay/" + cid, QRString: "pivot-qr:" + cid, Status: "PENDING", ExpiresAt: n.Add(15 * time.Minute)}, nil
}
func (s *Stub) CreateWithdrawal(_ context.Context, r provider.CreateWithdrawalRequest) (provider.CreateWithdrawalResult, error) {
	return provider.CreateWithdrawalResult{WithdrawalID: id("withdrawal", r.ExternalID+":"+r.RequestID), Status: "PROCESSING"}, nil
}
func (s *Stub) CreateRefund(_ context.Context, r provider.CreateRefundRequest) (provider.CreateRefundResult, error) {
	return provider.CreateRefundResult{RefundID: id("refund", r.ExternalID+":"+r.RequestID), Status: "PROCESSING"}, nil
}
func (s *Stub) VerifyWebhook(_ context.Context, r provider.VerifyWebhookRequest) (provider.VerifyWebhookResult, error) {
	m := hmac.New(sha256.New, []byte(s.Secret))
	m.Write(r.Body)
	valid := hmac.Equal([]byte(hex.EncodeToString(m.Sum(nil))), []byte(r.Signature))
	out := provider.VerifyWebhookResult{Valid: valid}
	if valid {
		_ = json.Unmarshal(r.Body, &out.Payload)
		out.ExternalID, _ = out.Payload["external_id"].(string)
		out.TransactionID, _ = out.Payload["transaction_id"].(string)
		out.Status, _ = out.Payload["status"].(string)
	}
	return out, nil
}
