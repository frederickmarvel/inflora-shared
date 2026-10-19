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
}
type CreateTopUpResult struct {
	ChargeID, PaymentURL, QRString, VANumber, EWalletDeepLink, Status string
	ExpiresAt                                                         time.Time
}
type CreateWithdrawalRequest struct {
	ExternalID                                 string
	AmountIDR                                  int64
	BeneficiaryRef, BeneficiaryType, RequestID string
}
type CreateWithdrawalResult struct{ WithdrawalID, Status string }

// GetWithdrawalStatusRequest identifies a payout for a provider status probe
// (D-055). Exactly one of ExternalID / ProviderWithdrawalID is required.
type GetWithdrawalStatusRequest struct {
	ExternalID           string // saruman payout id (stable platform key)
	ProviderWithdrawalID string // provider-side payout identifier
	RequestID            string // correlation for provider-side telemetry
}

// GetWithdrawalStatusResult is the provider-side truth for a payout. Status
// is PROCESSING | COMPLETED | FAILED or "" when the provider has no record.
type GetWithdrawalStatusResult struct {
	Status        string
	FailureCode   string
	BankReference string
}
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
	// GetWithdrawalStatus probes the provider for a payout's current status
	// (D-055). Providers without a status API (Midtrans IRIS at Phase C) must
	// return ErrNotImplemented; callers surface that as gRPC Unimplemented so
	// the sweep/review flow stays the fallback.
	GetWithdrawalStatus(context.Context, GetWithdrawalStatusRequest) (GetWithdrawalStatusResult, error)
	CreateRefund(context.Context, CreateRefundRequest) (CreateRefundResult, error)
	VerifyWebhook(context.Context, VerifyWebhookRequest) (VerifyWebhookResult, error)
}
