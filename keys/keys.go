// Package keys derives stable business-operation keys via UUIDv5. See
// almanac/planning/schemas/11-operation-keys.md for the freezing contract.
//
// All operational keys in Inflora are UUIDv5 values derived from a
// fixed namespace UUID and a stable name. Re-deriving the same input
// always yields the same key, so duplicate retries cannot produce
// duplicate financial effects (D-011, D-012).
package keys

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
)

// Namespace is the frozen UUIDv5 namespace for Inflora operation keys.
// Recorded in almanac/planning/schemas/11-operation-keys.md §2.
var Namespace = mustUUID("7f9a3b6c-2c8e-4f01-9c0e-1a4e7d8c9e02")

// Derived returns the UUIDv5 string for name, derived against the frozen
// Inflora operation namespace. Names with multiple parts should be joined
// with ':' — see the almanac.
func Derived(name string) string {
	return uuidV5(Namespace, name)
}

// DonationCaptureKey returns the operation key for a donation capture
// transition. The corresponding ledger entries are posted under this key.
func DonationCaptureKey(donationID string) string {
	return Derived("donation_capture:" + donationID)
}

// DonationSettleKey returns the operation key for a donation settlement
// transition (PENDING → AVAILABLE).
func DonationSettleKey(donationID string) string {
	return Derived("donation_settle:" + donationID)
}

// CreatePayoutKey returns the operation key for a payout reservation
// transition.
func CreatePayoutKey(payoutID string) string {
	return Derived("create_payout:" + payoutID)
}

// CompletePayoutKey returns the operation key for a payout completion
// transition.
func CompletePayoutKey(payoutID string) string {
	return Derived("complete_payout:" + payoutID)
}

// FailPayoutKey returns the operation key for a payout failure-release
// transition.
func FailPayoutKey(payoutID string) string {
	return Derived("fail_payout:" + payoutID)
}

// DonationRefundKey returns the operation key for a refund compensation
// transition. Both donation_id and refund_id participate so a single
// donation may have multiple refund operations.
func DonationRefundKey(donationID, refundID string) string {
	return Derived("donation_refund:" + donationID + ":" + refundID)
}

// PalantirTopUpKey returns the deterministic provider-layer key for a
// Palantir top-up. Idempotent on the gateway_topups.palantir_ref column.
func PalantirTopUpKey(donationID string) string {
	return Derived("palantir_topup:" + donationID)
}

// PalantirWithdrawalKey returns the deterministic provider-layer key for a
// Palantir withdrawal.
func PalantirWithdrawalKey(payoutID string) string {
	return Derived("palantir_withdrawal:" + payoutID)
}

// PalantirRefundKey returns the deterministic provider-layer key for a
// Palantir refund.
func PalantirRefundKey(refundID string) string {
	return Derived("palantir_refund:" + refundID)
}

// uuidV5 is a minimal UUIDv5 (SHA-1, RFC 4122 §4.3) implementation that
// does not pull in github.com/google/uuid. We keep this internal so
// shared does not gain a new top-level dep.
func uuidV5(namespace, name string) string {
	h := sha1.New()
	h.Write([]byte(namespace))
	h.Write([]byte(name))
	sum := h.Sum(nil)
	// Set version (5) and variant bits.
	sum[6] = (sum[6] & 0x0f) | 0x50
	sum[8] = (sum[8] & 0x3f) | 0x80
	return formatUUID(sum)
}

func formatUUID(b []byte) string {
	hex := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex[0:8], hex[8:12], hex[12:16], hex[16:20], hex[20:32])
}

func mustUUID(s string) string {
	// Accept only canonical 8-4-4-4-12 hex.
	if len(s) != 36 {
		panic(fmt.Sprintf("keys: invalid UUID %q", s))
	}
	for i, ch := range s {
		switch i {
		case 8, 13, 18, 23:
			if ch != '-' {
				panic(fmt.Sprintf("keys: invalid UUID %q", s))
			}
		default:
			if !strings.ContainsRune("0123456789abcdef", ch) {
				panic(fmt.Sprintf("keys: invalid UUID %q", s))
			}
		}
	}
	return strings.ToLower(s)
}
