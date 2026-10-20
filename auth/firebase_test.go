package auth

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Static 2048-bit RSA key (PKCS#1 PEM) so the JWKS under test is served from
// a static key: no network, no runtime key generation.
const testFirebaseStaticPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAuFJT01LWznhotbbVI3oTp8J3PU4F4X8iR/iDwDDi5PqZfJk+
eA4OuQtH7LMEyQaIKa8qRF8cxX01tSKBGoVmoy1XwhdlXQHvkCexOsyC0o3QTL70
Zkg8aKBRRBZtMe1uGBR87qD+9eiiwf9MvrkJT3maYMr/Z8MuRCSDaqx+CNtWemUk
073BL4A57cNS3aEuy1mNCXsUzcHT5M0jZnLcPUKpwSNU7qY5RRQsKs6QmTQ6mjVE
KwWT5bVfqOl69HbenkVjZoCeBQu3iISuWSBAxoae8IwjsTVxvZB2G4URglNIlPsF
yGla6/tfWawO/UvU5UtoVyJlC4HF199dXQ6ZqQIDAQABAoIBABwEl61lyDU2ZGRK
2Hc+FULI8qsk8wvl8WYTRqOuSq8X0MD54l8Jc/940YQPakjMb2I2uCwQcSUr9UAF
cK6BXKqtN8vPFp9erfWxsuUHNVOgLuubkzcA//2/cNrytcKDHtsWMi9r1ZW3uRXY
IuhS5DFqgKb59HznVF8TVc6fTcE2o8C1rF8hhi4o/g0oA8sOX6nfMrEc05AEXy4k
XnIDNp5buGWimFWyWbnjPtp+7fBYdMnQTgjmisJUCTMSjygdjy5cjwOdfzvM6d9J
8Rw0zjd4tZHvZE5/W4U0IBMme018ft+0lGkuWUXEMmBWHpvGbiFtVDYMhEQHlngv
CBTX9yECgYEA76hlPaTuYsWOGo4BsN3kFtOToCWnBn9qwNIbjUdv7z21pX6Fq6VY
ohGXoP172E4JDBdiuVqHycU/ntLBf/pZgbN+J2u8Eg2Kmx8xe7rL6KrKQCALNgRO
0qrrtL1Utn/K6Sa5Dl8NsRWi7kgicDTu5BY0pY7mgOFIfGRxbqFlDnUCgYEAxOPz
ilee3ozqdY9l61ADicAxU7lWOQuUTpNZBGfY62p+6jgoOUnZz9UmscgUeBFxDNcV
J4oDejxzMhVTw2a+98XvPGLhqoOAWTT1PKMCNB/gAw6qzzNpN2Timjkrg0DUORWq
jYsa8fEi1SxAGcY6qZQnAtYx3HEM3llVW9k2n+UCgYAw/3XLd6gBa7CU/0mbNpKG
lDVPfcM7UduqlopphRx+hlhYAJklscWARSaCFLE4VSQGY9WcXcT7XKeYOjCzkKSy
HteBCcbGzhvzgRol2NgNtrVpDOQd5B2VdrqAFmF2eoQqzO7y4D41YV4RZ5Aa7seU
vHGBE8lOukCOph6G5tp5bQKBgEhHkrF3xyfWPYBY20aOgi4As1sUD/YzLQfHigdX
BEgwOVVMtl+Jn/9vNZxQYktNw92929M/FFW8eWGqeKykPHkDXfciKcwKKIUXLuxK
v1VZWkhlCwmzyu1+CL4rPLEbjI1ESe9aYNLu/iWZ8BJGNmTosYWWEonlqp651t84
F5iNAoGBAJXNKfQorEmEW82Ks9oH/G8oA22XuQwLMbP/A6Ui3LrkwLs1VA6ZbFLa
5+KAz2tan6wiNOBCU6SVz97NJNriXVmNmkdn1r2a+7zm3UxlISI9KGwlYb/ZHjZM
Bl4k0VKfLZYDgbreNOaxQH5pCGxgYGWLfwx7Lg/B+Hqy8HPz7SMJ
-----END RSA PRIVATE KEY-----`

const (
	testFirebaseKid    = "test-kid"
	testFirebaseNow    = int64(1780000000)
	testFirebaseExpOK  = testFirebaseNow + 3600
	testFirebaseExpBad = testFirebaseNow - 60
)

func testFirebaseKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	block, _ := pem.Decode([]byte(testFirebaseStaticPEM))
	if block == nil {
		t.Fatal("test PEM did not decode")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// newTestJWKSServer serves the static key's public half under kid test-kid and
// counts requests, so alg-rejection tests can pin "rejected before any key".
func newTestJWKSServer(t *testing.T, key *rsa.PrivateKey) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	pub := &key.PublicKey
	jwks := map[string]any{"keys": []any{map[string]any{
		"kty": "RSA", "alg": "RS256", "use": "sig", "kid": testFirebaseKid,
		"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}}
	raw, err := json.Marshal(jwks)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func testFirebaseConfig(jwksURL string) FirebaseConfig {
	return FirebaseConfig{
		ProjectID:    "inflora-test",
		JWKSURL:      jwksURL,
		JWKSCacheTTL: time.Minute,
		Now:          func() time.Time { return time.Unix(testFirebaseNow, 0) },
	}
}

// signTestToken assembles and RS256-signs a JWS with the given header/payload.
func signTestToken(t *testing.T, key *rsa.PrivateKey, header, payload map[string]any) string {
	t.Helper()
	hb, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	input := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(pb)
	digest := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func validTestClaims() map[string]any {
	return map[string]any{
		"sub":            "firebase-uid-123",
		"iss":            "https://securetoken.google.com/inflora-test",
		"aud":            "inflora-test",
		"exp":            float64(testFirebaseExpOK),
		"iat":            float64(testFirebaseNow - 100),
		"auth_time":      float64(testFirebaseNow - 300),
		"email":          "creator@example.com",
		"email_verified": true,
	}
}

func rs256Header() map[string]any {
	return map[string]any{"alg": "RS256", "typ": "JWT", "kid": testFirebaseKid}
}

func TestVerifyIDTokenValid(t *testing.T) {
	key := testFirebaseKey(t)
	srv, _ := newTestJWKSServer(t, key)
	token := signTestToken(t, key, rs256Header(), validTestClaims())
	claims, err := VerifyIDToken(context.Background(), token, testFirebaseConfig(srv.URL))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Subject != "firebase-uid-123" || claims.Email != "creator@example.com" || !claims.EmailVerified {
		t.Fatalf("claims: %#v", claims)
	}
	if claims.AuthTime != testFirebaseNow-300 || claims.IssuedAt != testFirebaseNow-100 || claims.ExpiresAt != testFirebaseExpOK {
		t.Fatalf("timestamps: %#v", claims)
	}
	if claims.Raw == nil || claims.Raw["email"] != "creator@example.com" {
		t.Fatalf("raw claims missing: %#v", claims.Raw)
	}
}

func TestVerifyIDTokenExpired(t *testing.T) {
	key := testFirebaseKey(t)
	srv, _ := newTestJWKSServer(t, key)
	claims := validTestClaims()
	claims["exp"] = float64(testFirebaseExpBad)
	token := signTestToken(t, key, rs256Header(), claims)
	if _, err := VerifyIDToken(context.Background(), token, testFirebaseConfig(srv.URL)); !errors.Is(err, ErrFirebaseExpired) {
		t.Fatalf("err = %v, want ErrFirebaseExpired", err)
	}
	// exp == now is expired too (strictly-after check).
	claims["exp"] = float64(testFirebaseNow)
	token = signTestToken(t, key, rs256Header(), claims)
	if _, err := VerifyIDToken(context.Background(), token, testFirebaseConfig(srv.URL)); !errors.Is(err, ErrFirebaseExpired) {
		t.Fatalf("exp==now err = %v, want ErrFirebaseExpired", err)
	}
}

func TestVerifyIDTokenWrongAudience(t *testing.T) {
	key := testFirebaseKey(t)
	srv, _ := newTestJWKSServer(t, key)
	claims := validTestClaims()
	claims["aud"] = "other-project"
	token := signTestToken(t, key, rs256Header(), claims)
	if _, err := VerifyIDToken(context.Background(), token, testFirebaseConfig(srv.URL)); !errors.Is(err, ErrFirebaseAudience) {
		t.Fatalf("err = %v, want ErrFirebaseAudience", err)
	}
}

func TestVerifyIDTokenWrongIssuer(t *testing.T) {
	key := testFirebaseKey(t)
	srv, _ := newTestJWKSServer(t, key)
	claims := validTestClaims()
	claims["iss"] = "https://securetoken.google.com/attacker-project"
	token := signTestToken(t, key, rs256Header(), claims)
	if _, err := VerifyIDToken(context.Background(), token, testFirebaseConfig(srv.URL)); !errors.Is(err, ErrFirebaseIssuer) {
		t.Fatalf("err = %v, want ErrFirebaseIssuer", err)
	}
}

// TestVerifyIDTokenAlgNone pins that alg=none is rejected before any key is
// fetched: the JWKS server must never be called.
func TestVerifyIDTokenAlgNone(t *testing.T) {
	key := testFirebaseKey(t)
	srv, calls := newTestJWKSServer(t, key)
	hb, _ := json.Marshal(map[string]any{"alg": "none", "kid": testFirebaseKid})
	pb, _ := json.Marshal(validTestClaims())
	token := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(pb) + "."
	if _, err := VerifyIDToken(context.Background(), token, testFirebaseConfig(srv.URL)); !errors.Is(err, ErrFirebaseAlg) {
		t.Fatalf("err = %v, want ErrFirebaseAlg", err)
	}
	if n := atomic.LoadInt32(calls); n != 0 {
		t.Fatalf("jwks fetched %d times for alg=none token", n)
	}
}

// TestVerifyIDTokenHS256 pins that an HS256 header is rejected before any key
// is used — even one with a valid kid and an HMAC-looking signature.
func TestVerifyIDTokenHS256(t *testing.T) {
	key := testFirebaseKey(t)
	srv, calls := newTestJWKSServer(t, key)
	hb, _ := json.Marshal(map[string]any{"alg": "HS256", "typ": "JWT", "kid": testFirebaseKid})
	pb, _ := json.Marshal(validTestClaims())
	input := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(pb)
	mac := hmac.New(sha256.New, []byte("attacker-secret"))
	_, _ = mac.Write([]byte(input))
	token := input + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if _, err := VerifyIDToken(context.Background(), token, testFirebaseConfig(srv.URL)); !errors.Is(err, ErrFirebaseAlg) {
		t.Fatalf("err = %v, want ErrFirebaseAlg", err)
	}
	if n := atomic.LoadInt32(calls); n != 0 {
		t.Fatalf("jwks fetched %d times for HS256 token", n)
	}
}

// TestVerifyIDTokenMissingAlg: a header without alg is rejected like HS256/none.
func TestVerifyIDTokenMissingAlg(t *testing.T) {
	key := testFirebaseKey(t)
	srv, calls := newTestJWKSServer(t, key)
	token := signTestToken(t, key, map[string]any{"typ": "JWT", "kid": testFirebaseKid}, validTestClaims())
	if _, err := VerifyIDToken(context.Background(), token, testFirebaseConfig(srv.URL)); !errors.Is(err, ErrFirebaseAlg) {
		t.Fatalf("err = %v, want ErrFirebaseAlg", err)
	}
	if n := atomic.LoadInt32(calls); n != 0 {
		t.Fatalf("jwks fetched %d times for alg-less token", n)
	}
}

func TestVerifyIDTokenGarbage(t *testing.T) {
	key := testFirebaseKey(t)
	srv, _ := newTestJWKSServer(t, key)
	cfg := testFirebaseConfig(srv.URL)
	for _, token := range []string{"", "garbage", "a.b", "a.b.c", "!!!...", "a.b.c.d"} {
		if _, err := VerifyIDToken(context.Background(), token, cfg); !errors.Is(err, ErrFirebaseMalformed) {
			t.Fatalf("token %q: err = %v, want ErrFirebaseMalformed", token, err)
		}
	}
}

func TestVerifyIDTokenTruncated(t *testing.T) {
	key := testFirebaseKey(t)
	srv, _ := newTestJWKSServer(t, key)
	token := signTestToken(t, key, rs256Header(), validTestClaims())
	for _, cut := range []int{len(token) / 2, len(token) - 3, len(token) - 16} {
		_, err := VerifyIDToken(context.Background(), token[:cut], testFirebaseConfig(srv.URL))
		if err == nil {
			t.Fatalf("truncated at %d verified", cut)
		}
		if !errors.Is(err, ErrFirebaseMalformed) && !errors.Is(err, ErrFirebaseSignature) {
			t.Fatalf("truncated at %d: err = %v", cut, err)
		}
	}
}

func TestVerifyIDTokenBadSignature(t *testing.T) {
	key := testFirebaseKey(t)
	srv, _ := newTestJWKSServer(t, key)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	// Re-signed with the wrong key: valid structure, invalid signature.
	wrong := signTestToken(t, other, rs256Header(), validTestClaims())
	if _, err := VerifyIDToken(context.Background(), wrong, testFirebaseConfig(srv.URL)); !errors.Is(err, ErrFirebaseSignature) {
		t.Fatalf("err = %v, want ErrFirebaseSignature", err)
	}
	// Random signature bytes over a well-formed header/payload.
	parts := strings.Split(signTestToken(t, key, rs256Header(), validTestClaims()), ".")
	tampered := parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(make([]byte, 256))
	if _, err := VerifyIDToken(context.Background(), tampered, testFirebaseConfig(srv.URL)); !errors.Is(err, ErrFirebaseSignature) {
		t.Fatalf("random sig err = %v, want ErrFirebaseSignature", err)
	}
}

func TestVerifyIDTokenKidMiss(t *testing.T) {
	key := testFirebaseKey(t)
	srv, _ := newTestJWKSServer(t, key)
	token := signTestToken(t, key, map[string]any{"alg": "RS256", "kid": "unknown-kid"}, validTestClaims())
	if _, err := VerifyIDToken(context.Background(), token, testFirebaseConfig(srv.URL)); !errors.Is(err, ErrFirebaseKey) {
		t.Fatalf("err = %v, want ErrFirebaseKey", err)
	}
}

func TestVerifyIDTokenJWKSUnavailable(t *testing.T) {
	key := testFirebaseKey(t)
	token := signTestToken(t, key, rs256Header(), validTestClaims())
	// A 500 from the JWKS endpoint is the outage-class failure: the document
	// is unreachable so the kid can never resolve, distinct from a healthy
	// document that simply does not carry the kid (ErrFirebaseKey).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	if _, err := VerifyIDToken(context.Background(), token, testFirebaseConfig(srv.URL)); !errors.Is(err, ErrFirebaseJWKS) {
		t.Fatalf("err = %v, want ErrFirebaseJWKS", err)
	}
}

func TestVerifyIDTokenMissingSub(t *testing.T) {
	key := testFirebaseKey(t)
	srv, _ := newTestJWKSServer(t, key)
	claims := validTestClaims()
	delete(claims, "sub")
	token := signTestToken(t, key, rs256Header(), claims)
	if _, err := VerifyIDToken(context.Background(), token, testFirebaseConfig(srv.URL)); !errors.Is(err, ErrFirebaseMalformed) {
		t.Fatalf("err = %v, want ErrFirebaseMalformed", err)
	}
}

func TestVerifyIDTokenMissingExp(t *testing.T) {
	key := testFirebaseKey(t)
	srv, _ := newTestJWKSServer(t, key)
	claims := validTestClaims()
	delete(claims, "exp")
	token := signTestToken(t, key, rs256Header(), claims)
	if _, err := VerifyIDToken(context.Background(), token, testFirebaseConfig(srv.URL)); !errors.Is(err, ErrFirebaseMalformed) {
		t.Fatalf("err = %v, want ErrFirebaseMalformed", err)
	}
}

// TestVerifyIDTokenCachesJWKS pins the per-kid TTL cache: two verifications in
// one window cause exactly one JWKS fetch.
func TestVerifyIDTokenCachesJWKS(t *testing.T) {
	key := testFirebaseKey(t)
	srv, calls := newTestJWKSServer(t, key)
	cfg := testFirebaseConfig(srv.URL)
	for i := 0; i < 3; i++ {
		if _, err := VerifyIDToken(context.Background(), signTestToken(t, key, rs256Header(), validTestClaims()), cfg); err != nil {
			t.Fatalf("verify %d: %v", i, err)
		}
	}
	if n := atomic.LoadInt32(calls); n != 1 {
		t.Fatalf("jwks fetched %d times, want 1", n)
	}
}
