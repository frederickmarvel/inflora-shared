package ledger

import (
	"context"
	"errors"
	"testing"
)

func TestDoubleEntryRejectsNonPositiveAmount(t *testing.T) {
	for _, n := range []int64{0, -1} {
		err := new(Ledger).DoubleEntry(context.Background(), DoubleEntryParams{AmountIDR: n})
		if !errors.Is(err, ErrInvalidAmount) {
			t.Fatalf("amount %d: %v", n, err)
		}
	}
}
func TestSchemaEnums(t *testing.T) {
	if AccountStreamerAvailable != "STREAMER_AVAILABLE" || EntryRefundCredit != "REFUND_CREDIT" || Debit != "DEBIT" {
		t.Fatal("v2 enum mapping changed")
	}
}
