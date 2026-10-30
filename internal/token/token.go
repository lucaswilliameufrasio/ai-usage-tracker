package token

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

func New(prefix string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("gerar token: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

func Hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func Match(value string, hash string) bool {
	got, err := hex.DecodeString(Hash(value))
	want, err2 := hex.DecodeString(hash)
	if err != nil || err2 != nil || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
