// Package auth implements GitHub OAuth login, token encryption at rest, and sessions.
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// keySize is AES-256's key length.
const keySize = 32

// DecodeKey decodes the base64 value of SESSION_SECRET into a 32-byte AES key.
func DecodeKey(base64Value string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(base64Value)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}
	if len(key) != keySize {
		return nil, fmt.Errorf("key must decode to %d bytes, got %d", keySize, len(key))
	}
	return key, nil
}

type Encryptor struct {
	gcm cipher.AEAD
}

// NewEncryptor builds an Encryptor from a 32-byte AES-256 key.
func NewEncryptor(key []byte) (*Encryptor, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("encryption key must be %d bytes, got %d", keySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	return &Encryptor{gcm: gcm}, nil
}

func (e *Encryptor) Encrypt(plaintext string) ([]byte, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return e.gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Decrypt reverses Encrypt.
func (e *Encryptor) Decrypt(data []byte) (string, error) {
	ns := e.gcm.NonceSize()
	if len(data) < ns {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := data[:ns], data[ns:]
	plaintext, err := e.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}
