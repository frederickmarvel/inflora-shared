package auth

import (
	"encoding/base64"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestGenerateTokensFormatAndUniqueness(t *testing.T) {
	tests := []struct {
		name, prefix string
		generate     func() (string, error)
		valid        func(string) bool
	}{{"session", SessionTokenPrefix, GenerateSessionToken, ValidSessionToken}, {"overlay", OverlayTokenPrefix, GenerateOverlayToken, ValidOverlayToken}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := map[string]bool{}
			for i := 0; i < 20; i++ {
				token, err := tt.generate()
				if err != nil {
					t.Fatal(err)
				}
				if !tt.valid(token) || !strings.HasPrefix(token, tt.prefix) {
					t.Fatalf("invalid token %q", token)
				}
				raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, tt.prefix))
				if err != nil || len(raw) != 32 {
					t.Fatalf("payload: length=%d err=%v", len(raw), err)
				}
				if len(token) != 46 {
					t.Fatalf("length=%d", len(token))
				}
				if seen[token] {
					t.Fatal("duplicate token")
				}
				seen[token] = true
			}
		})
	}
}

func TestHashTokenRoundTrip(t *testing.T) {
	token, _ := GenerateSessionToken()
	hash, err := HashToken(token)
	if err != nil {
		t.Fatal(err)
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil || cost != TokenHashCost {
		t.Fatalf("cost=%d err=%v", cost, err)
	}
	ok, err := VerifyToken(token, hash)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	other, _ := GenerateSessionToken()
	ok, err = VerifyToken(other, hash)
	if err != nil || ok {
		t.Fatalf("wrong token: ok=%v err=%v", ok, err)
	}
	if _, err := HashToken("not-a-token"); err != ErrInvalidTokenFormat {
		t.Fatalf("err=%v", err)
	}
	if ok, err := VerifyToken(token, "bad hash"); err == nil || ok {
		t.Fatalf("bad hash: ok=%v err=%v", ok, err)
	}
}

func TestTokenLast4(t *testing.T) {
	token, _ := GenerateOverlayToken()
	got, err := TokenLast4(token)
	if err != nil || got != token[len(token)-4:] {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if _, err := TokenLast4("ok_short"); err != ErrInvalidTokenFormat {
		t.Fatalf("err=%v", err)
	}
}
