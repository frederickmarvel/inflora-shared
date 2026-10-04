package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory      = 64 * 1024
	argonIterations  = 3
	argonParallelism = 2
	argonSaltLength  = 16
	argonKeyLength   = 32
)

var ErrInvalidPasswordHash = errors.New("invalid password hash")

// HashPassword returns a standards-compatible Argon2id PHC string.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password must not be empty")
	}
	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLength)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory,
		argonIterations, argonParallelism, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// VerifyPassword parses an Argon2id PHC string and compares the derived key in
// constant time. Password mismatch returns (false, nil).
func VerifyPassword(password, encodedHash string) (bool, error) {
	memory, iterations, parallelism, salt, expected, err := parsePasswordHash(encodedHash)
	if err != nil {
		return false, err
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func parsePasswordHash(encoded string) (uint32, uint32, uint8, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return 0, 0, 0, nil, nil, ErrInvalidPasswordHash
	}
	var memory, iterations uint64
	var parallelism uint64
	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return 0, 0, 0, nil, nil, ErrInvalidPasswordHash
	}
	values := []*uint64{&memory, &iterations, &parallelism}
	wants := []string{"m=", "t=", "p="}
	for i := range values {
		if !strings.HasPrefix(params[i], wants[i]) {
			return 0, 0, 0, nil, nil, ErrInvalidPasswordHash
		}
		v, err := strconv.ParseUint(strings.TrimPrefix(params[i], wants[i]), 10, 32)
		if err != nil || v == 0 {
			return 0, 0, 0, nil, nil, ErrInvalidPasswordHash
		}
		*values[i] = v
	}
	if memory > 1024*1024 || iterations > 20 || parallelism > 255 {
		return 0, 0, 0, nil, nil, ErrInvalidPasswordHash
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil || len(salt) < 8 {
		return 0, 0, 0, nil, nil, ErrInvalidPasswordHash
	}
	key, err := b64.DecodeString(parts[5])
	if err != nil || len(key) < 16 || len(key) > 64 {
		return 0, 0, 0, nil, nil, ErrInvalidPasswordHash
	}
	return uint32(memory), uint32(iterations), uint8(parallelism), salt, key, nil
}
