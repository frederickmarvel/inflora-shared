package xendit

import (
	"context"
	"github.com/frederickmarvel/inflora-shared/provider"
)

type Client struct{}

func New() *Client { return &Client{} }
func (*Client) CreateTopUp(context.Context, provider.CreateTopUpRequest) (provider.CreateTopUpResult, error) {
	return provider.CreateTopUpResult{}, provider.ErrNotImplemented
}
func (*Client) CreateWithdrawal(context.Context, provider.CreateWithdrawalRequest) (provider.CreateWithdrawalResult, error) {
	return provider.CreateWithdrawalResult{}, provider.ErrNotImplemented
}
func (*Client) CreateRefund(context.Context, provider.CreateRefundRequest) (provider.CreateRefundResult, error) {
	return provider.CreateRefundResult{}, provider.ErrNotImplemented
}
func (*Client) VerifyWebhook(context.Context, provider.VerifyWebhookRequest) (provider.VerifyWebhookResult, error) {
	return provider.VerifyWebhookResult{}, provider.ErrNotImplemented
}
