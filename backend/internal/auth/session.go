package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// SessionTokenHash is a session token's lookup key, stored in the database.
// The raw token lives only in the user's browser cookie — this way, a
// database leak does not hand over usable sessions.
type SessionTokenHash [32]byte

// NewSessionToken returns a new random session token (for the cookie) and
// its hash (for the database).
func NewSessionToken() (token string, hash SessionTokenHash, err error) {
	b := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", hash, fmt.Errorf("generate session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashSessionToken(token), nil
}

// HashSessionToken re-derives a token's hash, e.g. to look up the token read
// back from a cookie.
func HashSessionToken(token string) SessionTokenHash {
	return sha256.Sum256([]byte(token))
}
