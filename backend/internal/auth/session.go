package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// SessionTokenHash is a session token's lookup key, stored in the database.
type SessionTokenHash [32]byte

func NewSessionToken() (token string, hash SessionTokenHash, err error) {
	b := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", hash, fmt.Errorf("generate session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashSessionToken(token), nil
}

// HashSessionToken re-derives a token's hash, e.g.
func HashSessionToken(token string) SessionTokenHash {
	return sha256.Sum256([]byte(token))
}
