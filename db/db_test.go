package db

import (
	"database/sql"
	"testing"
)

func TestUpSection(t *testing.T) {
	got := upSection("-- +goose Up\nCREATE TABLE x();\n-- +goose Down\nDROP TABLE x;")
	if got != "CREATE TABLE x();" {
		t.Fatalf("got %q", got)
	}
	plain := "CREATE TABLE y();"
	if got = upSection(plain); got != plain {
		t.Fatalf("plain migration changed: %q", got)
	}
}

func TestOpenRequiresDSN(t *testing.T) {
	if _, err := Open(t.Context(), Config{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestInTxRejectsNil(t *testing.T) {
	if err := InTx(t.Context(), nil, func(_ *sql.Tx) error { return nil }); err == nil {
		t.Fatal("expected error")
	}
}
