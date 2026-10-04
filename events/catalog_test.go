package events

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDonationChargedEventRoundTrip(t *testing.T) {
	want := DonationChargedEvent{DonationID: "donation", IntentID: "intent", StreamerID: "streamer", AmountIDR: 10000, MDRIDR: 70, MDRRateBPS: 70, GrossChargedIDR: 10070, PlatformFeeIDR: 1, NetIDR: 9999, Currency: "IDR", SettlementStatus: "PENDING", DonorDisplayName: "Andi", Message: "Semangat", ProviderChargeID: "charge", ProviderName: "midtrans", PaymentMethod: "QRIS", LedgerEntryIDs: []string{"a", "b"}, LedgerCorrelationID: "correlation", CapturedAt: time.Date(2026, 10, 2, 14, 30, 1, 500000000, time.UTC)}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got DonationChargedEvent
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestFundHoldCreatedNullableFields(t *testing.T) {
	b, err := json.Marshal(FundHoldCreatedEvent{HoldID: "hold", TargetType: "STREAMER", StreamerID: "streamer"})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"donation_id":null`, `"payout_id":null`, `"amount_idr":null`, `"expires_at":null`} {
		if !strings.Contains(string(b), field) {
			t.Fatalf("missing %s in %s", field, b)
		}
	}
}
