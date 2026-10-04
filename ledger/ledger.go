package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type AccountType string

const (
	AccountDonorCash         AccountType = "DONOR_CASH"
	AccountStreamerPending   AccountType = "STREAMER_PENDING"
	AccountStreamerAvailable AccountType = "STREAMER_AVAILABLE"
	AccountStreamerPaid      AccountType = "STREAMER_PAID"
	AccountPlatformRevenue   AccountType = "PLATFORM_REVENUE"
	AccountMDRRevenue        AccountType = "MDR_REVENUE"
)

type Direction string

const (
	Debit  Direction = "DEBIT"
	Credit Direction = "CREDIT"
)

type EntryType string

const (
	EntryDonationDeposit         EntryType = "DONATION_DEPOSIT"
	EntryStreamerPendingCredit   EntryType = "STREAMER_PENDING_CREDIT"
	EntryStreamerAvailableCredit EntryType = "STREAMER_AVAILABLE_CREDIT"
	EntryPlatformFee             EntryType = "PLATFORM_FEE"
	EntryMDRFee                  EntryType = "MDR_FEE"
	EntryStreamerPaidDebit       EntryType = "STREAMER_PAID_DEBIT"
	EntryRefundDebit             EntryType = "REFUND_DEBIT"
	EntryRefundCredit            EntryType = "REFUND_CREDIT"
	EntryHoldDebit               EntryType = "HOLD_DEBIT"
	EntryHoldCredit              EntryType = "HOLD_CREDIT"
)

type DoubleEntryParams struct {
	StreamerID, CorrelationID    string
	DebitAccount, CreditAccount  AccountType
	AmountIDR                    int64
	EntryType                    EntryType
	DonationID, PayoutID, HoldID *string
	ExternalRef                  string
	Metadata                     map[string]interface{}
}
type LedgerAccount struct {
	ID          string
	StreamerID  string
	AccountType AccountType
	Currency    string
	BalanceIDR  int64
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
type LedgerEntry struct {
	ID, StreamerID, AccountID string
	Direction                 Direction
	AmountIDR                 int64
	EntryType                 EntryType
	CorrelationID             string
	CreatedAt                 time.Time
}
type ReconcileResult struct {
	Balanced             bool
	Debit, Credit, Drift int64
}
type Ledger struct{ DB *sql.DB }

func New(db *sql.DB) *Ledger { return &Ledger{DB: db} }

var ErrInvalidAmount = errors.New("ledger: amount must be positive")

func (l *Ledger) DoubleEntry(ctx context.Context, p DoubleEntryParams) error {
	if p.AmountIDR <= 0 {
		return ErrInvalidAmount
	}
	if l == nil || l.DB == nil {
		return errors.New("ledger: nil database")
	}
	tx, err := l.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// The canonical schema indexes correlation_id but does not make it unique.
	// Serialize equal correlation IDs explicitly so two concurrent first-time
	// requests cannot both pass the idempotency check and post duplicate entries.
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", p.CorrelationID); err != nil {
		return fmt.Errorf("ledger: lock correlation: %w", err)
	}
	var exists int
	err = tx.QueryRowContext(ctx, "SELECT 1 FROM ledger_entries WHERE correlation_id=$1 LIMIT 1 FOR UPDATE", p.CorrelationID).Scan(&exists)
	if err == nil {
		return tx.Commit()
	}
	if err != sql.ErrNoRows {
		return err
	}
	first, second := string(p.DebitAccount), string(p.CreditAccount)
	if first > second {
		first, second = second, first
	}
	ids := map[string]string{}
	for _, typ := range []string{first, second} {
		var id string
		err = tx.QueryRowContext(ctx, "SELECT id FROM ledger_accounts WHERE streamer_id=$1 AND account_type=$2 AND currency='IDR' FOR UPDATE", p.StreamerID, typ).Scan(&id)
		if err != nil {
			return err
		}
		ids[typ] = id
	}
	metadata, err := json.Marshal(p.Metadata)
	if err != nil {
		return fmt.Errorf("ledger: marshal metadata: %w", err)
	}
	now := time.Now().UTC()
	for _, e := range []struct {
		a string
		d Direction
	}{{string(p.DebitAccount), Debit}, {string(p.CreditAccount), Credit}} {
		_, err = tx.ExecContext(ctx, "INSERT INTO ledger_entries (streamer_id,account_id,direction,amount_idr,entry_type,external_ref,donation_id,payout_id,hold_id,correlation_id,metadata,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)", p.StreamerID, ids[e.a], e.d, p.AmountIDR, p.EntryType, p.ExternalRef, p.DonationID, p.PayoutID, p.HoldID, p.CorrelationID, metadata, now)
		if err != nil {
			return err
		}
		delta := p.AmountIDR
		if e.d == Debit {
			delta = -delta
		}
		_, err = tx.ExecContext(ctx, "UPDATE ledger_accounts SET balance_idr=balance_idr+$1, version=version+1, updated_at=$2 WHERE id=$3", delta, now, ids[e.a])
		if err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("ledger commit: %w", err)
	}
	return nil
}

func (l *Ledger) Reconcile(ctx context.Context, streamerID string) (ReconcileResult, error) {
	var r ReconcileResult
	err := l.DB.QueryRowContext(ctx, "SELECT COALESCE(SUM(CASE WHEN direction='DEBIT' THEN amount_idr ELSE 0 END),0), COALESCE(SUM(CASE WHEN direction='CREDIT' THEN amount_idr ELSE 0 END),0) FROM ledger_entries WHERE streamer_id=$1", streamerID).Scan(&r.Debit, &r.Credit)
	if err != nil {
		return r, err
	}
	r.Drift = r.Debit - r.Credit
	if r.Drift < 0 {
		r.Drift = -r.Drift
	}
	r.Balanced = r.Drift == 0
	return r, nil
}

func Reconcile(ctx context.Context, database *sql.DB, streamerID string) (ReconcileResult, error) {
	return New(database).Reconcile(ctx, streamerID)
}
