package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	SessionTokenPrefix = "sk_"
	OverlayTokenPrefix = "ok_"
	TokenRandomBytes   = 32
	TokenHashCost      = 12
)

var ErrInvalidTokenFormat = errors.New("invalid token format")

// GenerateSessionToken returns an opaque dashboard token containing 256 bits
// of cryptographically secure randomness.
func GenerateSessionToken() (string, error) { return generateToken(SessionTokenPrefix) }

// GenerateOverlayToken returns an opaque OBS overlay token containing 256 bits
// of cryptographically secure randomness.
func GenerateOverlayToken() (string, error) { return generateToken(OverlayTokenPrefix) }

func generateToken(prefix string) (string, error) {
	random := make([]byte, TokenRandomBytes)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(random), nil
}

// ValidSessionToken and ValidOverlayToken validate the frozen wire format.
func ValidSessionToken(token string) bool { return validToken(token, SessionTokenPrefix) }
func ValidOverlayToken(token string) bool { return validToken(token, OverlayTokenPrefix) }

func validToken(token, prefix string) bool {
	if !strings.HasPrefix(token, prefix) {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, prefix))
	return err == nil && len(raw) == TokenRandomBytes
}

// TokenLast4 returns the last four characters of the base64url portion.
func TokenLast4(token string) (string, error) {
	if !ValidSessionToken(token) && !ValidOverlayToken(token) {
		return "", ErrInvalidTokenFormat
	}
	return token[len(token)-4:], nil
}

// HashToken hashes an opaque token with bcrypt cost 12, as required by the
// frozen storage contract. It rejects malformed tokens to prevent accidentally
// storing passwords, API keys, or unprefixed values in token_hash.
func HashToken(token string) (string, error) {
	if !ValidSessionToken(token) && !ValidOverlayToken(token) {
		return "", ErrInvalidTokenFormat
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(token), TokenHashCost)
	if err != nil {
		return "", fmt.Errorf("hash token: %w", err)
	}
	return string(hash), nil
}

// VerifyToken performs bcrypt verification. A mismatch is not an operational
// error: it returns (false, nil). Malformed hashes are returned as errors.
func VerifyToken(token, hash string) (bool, error) {
	if !ValidSessionToken(token) && !ValidOverlayToken(token) {
		return false, nil
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(token))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return false, nil
	}
	return false, fmt.Errorf("verify token: %w", err)
}
