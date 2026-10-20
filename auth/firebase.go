package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Firebase ID-token verification (Phase A, planning-marvel-1/01 §3):
// stdlib-only — the JWS is parsed, the RS256 signature is verified against
// Google's public securetoken JWKS (fetched over net/http and cached per kid),
// and the payload claims are pinned (sub/iss/aud/exp). No Admin SDK and no
// service account: verification is a trust boundary, not a service-account job.
const (
	// firebaseIssuerFormat is the securetoken issuer for one Firebase project.
	firebaseIssuerFormat = "https://securetoken.google.com/%s"

	// DefaultFirebaseJWKSURL serves the securetoken RSA public signing keys.
	DefaultFirebaseJWKSURL = "https://www.googleapis.com/service_accounts/v1/jwk/securetoken@system.gserviceaccount.com"

	// DefaultFirebaseJWKSCacheTTL is the per-kid JWKS cache lifetime applied
	// when FirebaseConfig.JWKSCacheTTL is unset.
	DefaultFirebaseJWKSCacheTTL = 5 * time.Minute

	maxJWKSResponseBytes = 1 << 20
	jwksFetchTimeout     = 5 * time.Second
)

// Typed verification failures. Callers map them all to 401; the distinct
// sentinels exist so logs, middleware and tests can pin the failure mode.
var (
	// ErrFirebaseMalformed marks a token that is not a well-formed 3-part
	// JWS or whose header/payload cannot be parsed.
	ErrFirebaseMalformed = errors.New("firebase: malformed token")
	// ErrFirebaseAlg marks a header alg other than RS256 (HS256, none, or
	// absent). The alg claim is pinned before any key is fetched or used.
	ErrFirebaseAlg = errors.New("firebase: unexpected signing algorithm")
	// ErrFirebaseIssuer marks an iss claim that does not match the configured
	// securetoken issuer.
	ErrFirebaseIssuer = errors.New("firebase: unexpected issuer")
	// ErrFirebaseAudience marks an aud claim that does not match the project.
	ErrFirebaseAudience = errors.New("firebase: unexpected audience")
	// ErrFirebaseExpired marks a token whose exp is not strictly after now.
	ErrFirebaseExpired = errors.New("firebase: token expired")
	// ErrFirebaseSignature marks an RS256 signature that does not verify.
	ErrFirebaseSignature = errors.New("firebase: signature verification failed")
	// ErrFirebaseKey marks a kid that cannot be resolved: absent from the
	// cached JWKS, absent from the header.
	ErrFirebaseKey = errors.New("firebase: signing key unavailable")
	// ErrFirebaseJWKS marks a failure to obtain the JWKS document itself:
	// fetch/transport errors, non-200 responses, or an unparsable document.
	// It is the outage-class signal operators act on (key rotation lag and
	// provider outages land here).
	ErrFirebaseJWKS = errors.New("firebase: jwks document unavailable")
)

// FirebaseConfig pins verification to one Firebase project. ProjectID is
// required. TokenIssuer, JWKSURL and JWKSCacheTTL fall back to the
// securetoken defaults, and Now supplies the clock for exp checks (tests).
type FirebaseConfig struct {
	ProjectID    string
	TokenIssuer  string
	JWKSURL      string
	JWKSCacheTTL time.Duration
	Now          func() time.Time
}

func (c FirebaseConfig) withDefaults() FirebaseConfig {
	if c.ProjectID == "" {
		return c
	}
	if c.TokenIssuer == "" {
		c.TokenIssuer = fmt.Sprintf(firebaseIssuerFormat, c.ProjectID)
	}
	if c.JWKSURL == "" {
		c.JWKSURL = DefaultFirebaseJWKSURL
	}
	if c.JWKSCacheTTL <= 0 {
		c.JWKSCacheTTL = DefaultFirebaseJWKSCacheTTL
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return c
}

// FirebaseClaims are the verified claims of a Firebase ID token. Raw always
// carries the unmodified payload map, so callers can read claims this struct
// does not name. AuthTime and EmailVerified are optional fields in Firebase
// payloads and stay zero-valued when absent; ExpiresAt is exp in unix seconds.
type FirebaseClaims struct {
	Subject       string
	Issuer        string
	Audience      string
	ExpiresAt     int64
	IssuedAt      int64
	AuthTime      int64
	Email         string
	EmailVerified bool
	Raw           map[string]any
}

// VerifyIDToken verifies a Firebase ID token. Check order: parse the 3-part
// JWS; pin the header alg to RS256 (HS256/none/absent are rejected before any
// key is used); resolve the kid from the cached JWKS (fetch + parse the
// base64url MODULUS/EXPONENT, cache per kid with TTL); verify the RS256
// signature over header.payload with rsa.VerifyPKCS1v15; then decode the
// payload and pin sub (non-empty), iss (configured issuer), aud (project id)
// and exp (strictly after now).
func VerifyIDToken(ctx context.Context, token string, cfg FirebaseConfig) (*FirebaseClaims, error) {
	cfg = cfg.withDefaults()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrFirebaseMalformed
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrFirebaseMalformed
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, ErrFirebaseMalformed
	}
	if header.Alg != "RS256" {
		return nil, ErrFirebaseAlg
	}
	if header.Kid == "" {
		return nil, ErrFirebaseKey
	}
	key, err := signingKey(ctx, cfg, header.Kid)
	if err != nil {
		return nil, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrFirebaseMalformed
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return nil, ErrFirebaseSignature
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrFirebaseMalformed
	}
	payload := make(map[string]any)
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, ErrFirebaseMalformed
	}
	claims, err := claimsFromPayload(payload)
	if err != nil {
		return nil, err
	}
	if claims.Subject == "" {
		return nil, ErrFirebaseMalformed
	}
	if claims.Issuer != cfg.TokenIssuer {
		return nil, ErrFirebaseIssuer
	}
	if claims.Audience != cfg.ProjectID {
		return nil, ErrFirebaseAudience
	}
	if !time.Unix(claims.ExpiresAt, 0).After(cfg.Now()) {
		return nil, ErrFirebaseExpired
	}
	return claims, nil
}

// claimsFromPayload projects the named claims out of the decoded payload.
func claimsFromPayload(payload map[string]any) (*FirebaseClaims, error) {
	c := &FirebaseClaims{Raw: payload}
	c.Subject, _ = payload["sub"].(string)
	c.Issuer, _ = payload["iss"].(string)
	c.Audience, _ = payload["aud"].(string)
	c.Email, _ = payload["email"].(string)
	c.EmailVerified, _ = payload["email_verified"].(bool)
	c.ExpiresAt, _ = jsonInt(payload["exp"])
	c.IssuedAt, _ = jsonInt(payload["iat"])
	c.AuthTime, _ = jsonInt(payload["auth_time"])
	if c.ExpiresAt == 0 {
		return nil, ErrFirebaseMalformed
	}
	return c, nil
}

// jsonInt coerces the JSON-number claims (decoded as float64) to unix seconds.
func jsonInt(v any) (int64, bool) {
	f, ok := v.(float64)
	if !ok || f < 0 || f > float64(1<<62) {
		return 0, false
	}
	return int64(f), true
}

// cachedJWKS is one fetched key set: the kid->key map plus the fetch metadata
// used for the per-kid TTL check.
type cachedJWKS struct {
	fetchedAt time.Time
	ttl       time.Duration
	keys      map[string]*rsa.PublicKey
}

// jwksCache is the process-wide kid->key cache, keyed by JWKS URL, so all
// callers of one project share one fetch per TTL window.
var jwksCache = struct {
	sync.Mutex
	sets map[string]*cachedJWKS
}{sets: make(map[string]*cachedJWKS)}

// signingKey resolves kid to a public RSA key, fetching and caching the JWKS
// document when the entry is missing or past its TTL. A failed fetch never
// repopulates a stale entry: verification fails closed (01-firebase-rbac §8).
func signingKey(ctx context.Context, cfg FirebaseConfig, kid string) (*rsa.PublicKey, error) {
	jwksCache.Lock()
	set := jwksCache.sets[cfg.JWKSURL]
	if set == nil {
		set = &cachedJWKS{keys: make(map[string]*rsa.PublicKey)}
		jwksCache.sets[cfg.JWKSURL] = set
	}
	stale := time.Since(set.fetchedAt) >= set.ttl
	jwksCache.Unlock()

	if stale {
		keys, err := fetchJWKS(ctx, cfg.JWKSURL)
		if err != nil {
			return nil, err
		}
		jwksCache.Lock()
		set.keys = keys
		set.fetchedAt = time.Now()
		set.ttl = cfg.JWKSCacheTTL
		jwksCache.Unlock()
	}

	jwksCache.Lock()
	key, ok := set.keys[kid]
	jwksCache.Unlock()
	if !ok {
		return nil, fmt.Errorf("%w: kid %q not in jwks", ErrFirebaseKey, kid)
	}
	return key, nil
}

// fetchJWKS downloads and parses the JWKS document into kid->public-RSA-key.
func fetchJWKS(ctx context.Context, url string) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build jwks request: %v", ErrFirebaseJWKS, err)
	}
	client := &http.Client{Timeout: jwksFetchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: fetch jwks: %v", ErrFirebaseJWKS, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxJWKSResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: read jwks: %v", ErrFirebaseJWKS, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: jwks status %d", ErrFirebaseJWKS, resp.StatusCode)
	}
	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%w: parse jwks: %v", ErrFirebaseJWKS, err)
	}
	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		pub, err := rsaKeyFromJWK(k.N, k.E)
		if err != nil {
			return nil, fmt.Errorf("%w: kid %q: %v", ErrFirebaseJWKS, k.Kid, err)
		}
		keys[k.Kid] = pub
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("%w: jwks contains no RSA keys", ErrFirebaseJWKS)
	}
	return keys, nil
}

// rsaKeyFromJWK builds an *rsa.PublicKey from the base64url MODULUS/EXPONENT
// members of an RSA JWK.
func rsaKeyFromJWK(modulus, exponentB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(modulus)
	if err != nil || len(nBytes) == 0 {
		return nil, errors.New("bad jwk modulus")
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(exponentB64)
	if err != nil || len(eBytes) == 0 || len(eBytes) > 4 {
		return nil, errors.New("bad jwk exponent")
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	if e < 2 || e%2 == 0 {
		return nil, errors.New("bad jwk exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}
