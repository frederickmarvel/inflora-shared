package events

import "time"

type DonationIntentCreatedEvent struct {
	IntentID             string    `json:"intent_id"`
	DonationID           string    `json:"donation_id"`
	StreamerID           string    `json:"streamer_id"`
	AmountIDR            int64     `json:"amount_idr"`
	Currency             string    `json:"currency,omitempty"`
	PaymentMethod        string    `json:"payment_method,omitempty"`
	DonorDisplayName     string    `json:"donor_display_name,omitempty"`
	DonorEmail           string    `json:"donor_email,omitempty"`
	Message              string    `json:"message,omitempty"`
	IsAnonymous          bool      `json:"is_anonymous"`
	DisplayRateIDRPerSec int64     `json:"display_rate_idr_per_sec"`
	DisplayMinSec        int       `json:"display_min_sec"`
	DisplayMaxSec        int       `json:"display_max_sec"`
	DisplayDurationSec   int       `json:"display_duration_sec"`
	VoiceURL             string    `json:"voice_url,omitempty"`
	VoiceDurationSec     int       `json:"voice_duration_sec,omitempty"`
	YouTubeURL           string    `json:"youtube_url,omitempty"`
	YouTubeStartSec      int       `json:"youtube_start_sec,omitempty"`
	YouTubeEndSec        int       `json:"youtube_end_sec,omitempty"`
	ClientIP             string    `json:"client_ip,omitempty"`
	UserAgent            string    `json:"user_agent,omitempty"`
	CaptchaToken         string    `json:"captcha_token,omitempty"`
	// Rich-intent fields (CONTRACT_DRIFT D-031/D-037). Optional: legacy donors
	// are unaffected.
	LiveSessionID *string            `json:"live_session_id,omitempty"`
	WidgetID      string             `json:"widget_id,omitempty"`
	Quantity      *int               `json:"quantity,omitempty"`
	Tier          string             `json:"tier,omitempty"`
	Media         *DonationMediaEvent `json:"media,omitempty"`
	Call          *DonationCallEvent  `json:"call,omitempty"`
	ExpiresAt     time.Time          `json:"expires_at"`
	CreatedAt     time.Time          `json:"created_at"`
}

// DonationMediaEvent is the media-share attachment on a rich tip.
type DonationMediaEvent struct {
	Type       string `json:"type"`
	Title      string `json:"title,omitempty"`
	DurationMS int    `json:"duration_ms,omitempty"`
}

// DonationCallEvent is the call attachment on a rich tip.
type DonationCallEvent struct {
	Topic string `json:"topic,omitempty"`
	Mode  string `json:"mode,omitempty"`
}

type DonationChargedEvent struct {
	DonationID          string     `json:"donation_id"`
	IntentID            string     `json:"intent_id"`
	StreamerID          string     `json:"streamer_id"`
	AmountIDR           int64      `json:"amount_idr"`
	MDRIDR              int64      `json:"mdr_idr"`
	MDRRateBPS          int64      `json:"mdr_rate_bps"`
	GrossChargedIDR     int64      `json:"gross_charged_idr"`
	PlatformFeeIDR      int64      `json:"platform_fee_idr"`
	NetIDR              int64      `json:"net_idr"`
	Currency            string     `json:"currency"`
	SettlementStatus    string     `json:"settlement_status"`
	DonorDisplayName    string     `json:"donor_display_name,omitempty"`
	IsAnonymous         bool       `json:"is_anonymous"`
	Message             string     `json:"message,omitempty"`
	VoiceURL            string     `json:"voice_url,omitempty"`
	VoiceDurationSec    int        `json:"voice_duration_sec,omitempty"`
	YouTubeURL          string     `json:"youtube_url,omitempty"`
	YouTubeStartSec     int        `json:"youtube_start_sec,omitempty"`
	YouTubeEndSec       int        `json:"youtube_end_sec,omitempty"`
	DisplayDurationSec  int        `json:"display_duration_sec"`
	ProviderChargeID    string     `json:"provider_charge_id"`
	ProviderName        string     `json:"provider_name"`
	PaymentMethod       string     `json:"payment_method"`
	LedgerEntryIDs      []string   `json:"ledger_entry_ids"`
	LedgerCorrelationID string     `json:"ledger_correlation_id"`
	CapturedAt          time.Time  `json:"captured_at"`
	ExpectedSettledAt   *time.Time `json:"expected_settled_at,omitempty"`
	// Additive review state (CONTRACT_DRIFT D-038, owner-amended 2026-10-18):
	// review_status is "APPROVED" for every charged donation (airing is never
	// delayed); review_flag_reason carries the moderation badge (may be empty).
	// Old producers leave them empty — nothing changes for those rows.
	ReviewStatus     string `json:"review_status,omitempty"`
	ReviewFlagReason string `json:"review_flag_reason,omitempty"`
}

// DonationReviewedEvent carries a creator decision on an already-aired
// donation (CONTRACT_DRIFT D-038): ReviewStatus is "APPROVED" (idempotent
// confirm, airing unaffected) or "REJECTED" (replay-safe TAKEDOWN — the frame
// is removed from the overlay projection; terminal, no restore).
// Charge-initialized APPROVED rows never emit it: the charged event is their
// only airing signal. The display subset mirrors DonationChargedEvent so
// consumers can act without a DB read.
type DonationReviewedEvent struct {
	DonationID         string     `json:"donation_id"`
	IntentID           string     `json:"intent_id"`
	StreamerID         string     `json:"streamer_id"`
	ReviewStatus       string     `json:"review_status"`
	Reviewer           string     `json:"reviewer"`
	FlagReason         string     `json:"flag_reason,omitempty"`
	ReviewedAt         time.Time  `json:"reviewed_at"`
	AiredAt            *time.Time `json:"aired_at,omitempty"`
	AmountIDR          int64      `json:"amount_idr"`
	MDRIDR             int64      `json:"mdr_idr"`
	GrossChargedIDR    int64      `json:"gross_charged_idr"`
	PlatformFeeIDR     int64      `json:"platform_fee_idr"`
	NetIDR             int64      `json:"net_idr"`
	DonorDisplayName   string     `json:"donor_display_name,omitempty"`
	IsAnonymous        bool       `json:"is_anonymous"`
	Message            string     `json:"message,omitempty"`
	VoiceURL           string     `json:"voice_url,omitempty"`
	VoiceDurationSec   int        `json:"voice_duration_sec,omitempty"`
	YouTubeURL         string     `json:"youtube_url,omitempty"`
	YouTubeStartSec    int        `json:"youtube_start_sec,omitempty"`
	YouTubeEndSec      int        `json:"youtube_end_sec,omitempty"`
	DisplayDurationSec int        `json:"display_duration_sec"`
	SettlementStatus   string     `json:"settlement_status"`
	PaymentMethod      string     `json:"payment_method"`
}

type DonationSettledEvent struct {
	DonationID            string    `json:"donation_id"`
	IntentID              string    `json:"intent_id"`
	StreamerID            string    `json:"streamer_id"`
	AmountIDR             int64     `json:"amount_idr"`
	MDRIDR                int64     `json:"mdr_idr"`
	NetIDR                int64     `json:"net_idr"`
	ProviderName          string    `json:"provider_name"`
	ProviderSettlementID  string    `json:"provider_settlement_id"`
	SettlementHeldSeconds int       `json:"settlement_held_seconds"`
	LedgerEntryIDs        []string  `json:"ledger_entry_ids"`
	LedgerCorrelationID   string    `json:"ledger_correlation_id"`
	SettledAt             time.Time `json:"settled_at"`
}
type DonationFailedEvent struct {
	IntentID          string    `json:"intent_id"`
	DonationID        string    `json:"donation_id"`
	StreamerID        string    `json:"streamer_id"`
	AmountIDR         int64     `json:"amount_idr"`
	FailureReason     string    `json:"failure_reason"`
	ProviderErrorCode string    `json:"provider_error_code,omitempty"`
	ProviderName      string    `json:"provider_name,omitempty"`
	FailedAt          time.Time `json:"failed_at"`
}
type DonationRefundedEvent struct {
	RefundID         string    `json:"refund_id"`
	DonationID       string    `json:"donation_id"`
	StreamerID       string    `json:"streamer_id"`
	AmountIDR        int64     `json:"amount_idr"`
	Reason           string    `json:"reason"`
	ProviderName     string    `json:"provider_name"`
	ProviderRefundID string    `json:"provider_refund_id"`
	LedgerEntryIDs   []string  `json:"ledger_entry_ids"`
	RefundedAt       time.Time `json:"refunded_at"`
}
type DonationReceiptSentEvent struct {
	DonationID      string    `json:"donation_id"`
	IntentID        string    `json:"intent_id"`
	StreamerID      string    `json:"streamer_id"`
	DonorEmail      string    `json:"donor_email"`
	ReceiptID       string    `json:"receipt_id"`
	ProviderMsgID   string    `json:"provider_msg_id"`
	AmountIDR       int64     `json:"amount_idr"`
	GrossChargedIDR int64     `json:"gross_charged_idr"`
	SentAt          time.Time `json:"sent_at"`
}
type PayoutRequestedEvent struct {
	PayoutID         string    `json:"payout_id"`
	StreamerID       string    `json:"streamer_id"`
	AmountIDR        int64     `json:"amount_idr"`
	BankAccountID    string    `json:"bank_account_id"`
	BankAccountLast4 string    `json:"bank_account_last4"`
	BankCode         string    `json:"bank_code"`
	RequestedAt      time.Time `json:"requested_at"`
}
type PayoutBatchedEvent struct {
	BatchID          string    `json:"batch_id"`
	PayoutID         string    `json:"payout_id"`
	StreamerID       string    `json:"streamer_id"`
	AmountIDR        int64     `json:"amount_idr"`
	BankAccountLast4 string    `json:"bank_account_last4"`
	BatchTotalIDR    int64     `json:"batch_total_idr"`
	BatchPayoutCount int       `json:"batch_payout_count"`
	FIFOPosition     int       `json:"fifo_position"`
	BatchedAt        time.Time `json:"batched_at"`
}
type PayoutSettledEvent struct {
	PayoutID         string    `json:"payout_id"`
	StreamerID       string    `json:"streamer_id"`
	AmountIDR        int64     `json:"amount_idr"`
	ProviderPayoutID string    `json:"provider_payout_id"`
	BankReference    string    `json:"bank_reference"`
	SettledAt        time.Time `json:"settled_at"`
}
type PayoutFailedEvent struct {
	PayoutID      string    `json:"payout_id"`
	StreamerID    string    `json:"streamer_id"`
	AmountIDR     int64     `json:"amount_idr"`
	FailureReason string    `json:"failure_reason"`
	ProviderError string    `json:"provider_error,omitempty"`
	FailedAt      time.Time `json:"failed_at"`
}
type PayoutBatchExecutedEvent struct {
	BatchID         string    `json:"batch_id"`
	PayoutID        string    `json:"payout_id"`
	StreamerID      string    `json:"streamer_id"`
	AmountIDR       int64     `json:"amount_idr"`
	ProviderBatchID string    `json:"provider_batch_id"`
	ProviderName    string    `json:"provider_name"`
	ExecutedAt      time.Time `json:"executed_at"`
}
type PayoutBatchCompletedEvent struct {
	BatchID         string    `json:"batch_id"`
	TotalAmountIDR  int64     `json:"total_amount_idr"`
	SettledCount    int       `json:"settled_count"`
	FailedCount     int       `json:"failed_count"`
	ProviderBatchID string    `json:"provider_batch_id"`
	CompletedAt     time.Time `json:"completed_at"`
}
type PayoutBatchFailedEvent struct {
	BatchID       string    `json:"batch_id"`
	FailureReason string    `json:"failure_reason"`
	ProviderError string    `json:"provider_error,omitempty"`
	FailedAt      time.Time `json:"failed_at"`
}
type StreamerRegisteredEvent struct {
	StreamerID   string    `json:"streamer_id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	RegisteredAt time.Time `json:"registered_at"`
}
type StreamerSettingsUpdatedEvent struct {
	StreamerID           string    `json:"streamer_id"`
	DisplayRateIDRPerSec int64     `json:"display_rate_idr_per_sec"`
	DisplayMinSec        int       `json:"display_min_sec"`
	DisplayMaxSec        int       `json:"display_max_sec"`
	ShowDonorName        bool      `json:"show_donor_name"`
	AllowVoice           bool      `json:"allow_voice"`
	AllowYouTube         bool      `json:"allow_youtube"`
	AutoPlayVoice        bool      `json:"auto_play_voice"`
	AutoPlayYouTube      bool      `json:"auto_play_youtube"`
	NotifyOnDonation     bool      `json:"notify_on_donation"`
	MinDonationIDR       int64     `json:"min_donation_idr"`
	MaxDonationIDR       int64     `json:"max_donation_idr"`
	UpdatedAt            time.Time `json:"updated_at"`
}
type OverlayTokenRotatedEvent struct {
	StreamerID    string    `json:"streamer_id"`
	OldTokenLast4 string    `json:"old_token_last4"`
	NewTokenLast4 string    `json:"new_token_last4"`
	RotatedAt     time.Time `json:"rotated_at"`
	ClientIP      string    `json:"client_ip"`
}
type FundHoldCreatedEvent struct {
	HoldID     string     `json:"hold_id"`
	TargetType string     `json:"target_type"`
	StreamerID string     `json:"streamer_id"`
	DonationID *string    `json:"donation_id"`
	PayoutID   *string    `json:"payout_id"`
	AmountIDR  *int64     `json:"amount_idr"`
	Reason     string     `json:"reason"`
	ExpiresAt  *time.Time `json:"expires_at"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
}
type FundHoldReleasedEvent struct {
	HoldID         string    `json:"hold_id"`
	TargetType     string    `json:"target_type"`
	StreamerID     string    `json:"streamer_id"`
	ReleasedBy     string    `json:"released_by"`
	ReleasedAt     time.Time `json:"released_at"`
	ResolutionNote string    `json:"resolution_note"`
}
type FraudFlaggedEvent struct {
	Rule             string    `json:"rule"`
	StreamerID       *string   `json:"streamer_id"`
	DonorFingerprint string    `json:"donor_fingerprint"`
	Count            int       `json:"count"`
	WindowSeconds    int       `json:"window_seconds"`
	ActionTaken      string    `json:"action_taken"`
	FlaggedAt        time.Time `json:"flagged_at"`
}
