package provider

import (
	"context"
	"errors"
	"time"
)

var ErrNotImplemented = errors.New("provider: not implemented")

type PaymentMethod string

const (
	QRIS    PaymentMethod = "QRIS"
	VA      PaymentMethod = "VA"
	EWallet PaymentMethod = "EWALLET"
)

type CreateTopUpRequest struct {
	ExternalID                                                 string
	AmountIDR                                                  int64
	PaymentMethod                                              PaymentMethod
	ChannelCode, DonorName, DonorEmail, CallbackURL, RequestID string
	// Redirect return URLs for the hosted payment page. Providers that need
	// them (Pivot REDIRECT) read these; others ignore them.
	SuccessReturnURL, FailureReturnURL, ExpirationURL string
	// Description is the human-readable donation summary carried into the
	// provider order.
	Description string
}
type CreateTopUpResult struct {
	ChargeID, PaymentURL, QRString, VANumber, EWalletDeepLink, Status string
	// PaymentMethod is the method the provider's session actually uses, e.g.
	// "QRIS" or "REDIRECT" (a hosted list-of-payment UI). Providers set it;
	// callers persist it without guessing.
	PaymentMethod string
	ExpiresAt     time.Time
}
type CreateWithdrawalRequest struct {
	ExternalID                                 string
	AmountIDR                                  int64
	BeneficiaryRef, BeneficiaryType, RequestID string
}
type CreateWithdrawalResult struct{ WithdrawalID, Status string }
type CreateRefundRequest struct {
	ExternalID, OriginalChargeID, RequestID string
	AmountIDR                               int64
}
type CreateRefundResult struct{ RefundID, Status string }
type VerifyWebhookRequest struct {
	Body      []byte
	Signature string
	Headers   map[string]string
}
type VerifyWebhookResult struct {
	Valid                             bool
	ExternalID, TransactionID, Status string
	Payload                           map[string]interface{}
}
type Provider interface {
	CreateTopUp(context.Context, CreateTopUpRequest) (CreateTopUpResult, error)
	CreateWithdrawal(context.Context, CreateWithdrawalRequest) (CreateWithdrawalResult, error)
	CreateRefund(context.Context, CreateRefundRequest) (CreateRefundResult, error)
	VerifyWebhook(context.Context, VerifyWebhookRequest) (VerifyWebhookResult, error)
}
