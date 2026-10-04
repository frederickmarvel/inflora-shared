package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestPasswordHashPHCAndRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("unexpected PHC: %s", hash)
	}
	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("wrong", hash)
	if err != nil || ok {
		t.Fatalf("wrong password: ok=%v err=%v", ok, err)
	}
	hash2, _ := HashPassword("correct horse battery staple")
	if hash == hash2 {
		t.Fatal("hashes should use independent salts")
	}
}

func TestPasswordHashRejectsInvalidInput(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("expected empty-password error")
	}
	invalid := []string{"", "$argon2i$v=19$m=65536,t=3,p=2$abc$abc", "$argon2id$v=18$m=65536,t=3,p=2$abc$abc", "$argon2id$v=19$m=0,t=3,p=2$abc$abc", "$argon2id$v=19$m=99999999,t=3,p=2$YWJjZGVmZ2g$YWJjZGVmZ2hpamtsbW5vcA"}
	for _, hash := range invalid {
		if _, err := VerifyPassword("password", hash); !errors.Is(err, ErrInvalidPasswordHash) {
			t.Errorf("hash=%q err=%v", hash, err)
		}
	}
}
