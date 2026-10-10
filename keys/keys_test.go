package keys

import "testing"

func TestDerivedIsDeterministic(t *testing.T) {
	t.Parallel()
	if a, b := Derived("hello:world"), Derived("hello:world"); a != b {
		t.Fatalf("derived not deterministic: %s vs %s", a, b)
	}
	if a, b := Derived("hello:world"), Derived("hello:world!"); a == b {
		t.Fatalf("derived collides on different inputs: %s vs %s", a, b)
	}
}

func TestOperationKeysAreStableAcrossCalls(t *testing.T) {
	t.Parallel()
	donationID := "11111111-1111-1111-1111-111111111111"
	a := DonationCaptureKey(donationID)
	b := DonationCaptureKey(donationID)
	if a != b {
		t.Fatalf("donation capture key not stable: %s vs %s", a, b)
	}
	if a == DonationSettleKey(donationID) {
		t.Fatalf("capture and settle keys collide for donation %s", donationID)
	}
}

func TestPalantirKeysAreDistinct(t *testing.T) {
	t.Parallel()
	donationID := "22222222-2222-2222-2222-222222222222"
	if PalantirTopUpKey(donationID) == PalantirWithdrawalKey(donationID) {
		t.Fatalf("palantir top-up and withdrawal collide for %s", donationID)
	}
}

func TestFormatUUID(t *testing.T) {
	t.Parallel()
	sum := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a,
		0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14,
	}
	got := formatUUID(sum)
	want := "01020304-0506-0708-090a-0b0c0d0e0f10"
	if got != want {
		t.Fatalf("formatUUID = %s, want %s", got, want)
	}
}

func TestMustUUIDRejects(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("mustUUID accepted invalid UUID")
		}
	}()
	mustUUID("not-a-uuid")
}
