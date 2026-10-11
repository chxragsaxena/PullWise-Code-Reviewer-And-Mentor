package auth

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"strings"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	enc, err := NewEncryptor(testKey(t))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := enc.Encrypt("ghu_sometoken")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ciphertext), "ghu_sometoken") {
		t.Fatal("ciphertext contains the plaintext token")
	}

	got, err := enc.Decrypt(ciphertext)
	if err != nil || got != "ghu_sometoken" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestEncryptIsNotDeterministic(t *testing.T) {
	enc, _ := NewEncryptor(testKey(t))
	a, _ := enc.Encrypt("same input")
	b, _ := enc.Encrypt("same input")
	if string(a) == string(b) {
		t.Fatal("two encryptions of the same plaintext produced identical ciphertext (nonce reuse?)")
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	enc, _ := NewEncryptor(testKey(t))
	ciphertext, _ := enc.Encrypt("secret")
	ciphertext[len(ciphertext)-1] ^= 0xFF // flip a bit

	if _, err := enc.Decrypt(ciphertext); err == nil {
		t.Fatal("expected tampering to be detected")
	}
}

func TestDecryptRejectsTruncatedCiphertext(t *testing.T) {
	enc, _ := NewEncryptor(testKey(t))
	if _, err := enc.Decrypt([]byte("short")); err == nil {
		t.Fatal("expected an error for data shorter than a nonce")
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	enc1, _ := NewEncryptor(testKey(t))
	enc2, _ := NewEncryptor(testKey(t))
	ciphertext, _ := enc1.Encrypt("secret")
	if _, err := enc2.Decrypt(ciphertext); err == nil {
		t.Fatal("expected decryption with a different key to fail")
	}
}

func TestNewEncryptorRejectsWrongKeySize(t *testing.T) {
	if _, err := NewEncryptor([]byte("too short")); err == nil {
		t.Fatal("expected an error for a key that isn't 32 bytes")
	}
}

func TestDecodeKey(t *testing.T) {
	raw := testKey(t)
	encoded := base64.StdEncoding.EncodeToString(raw)

	got, err := DecodeKey(encoded)
	if err != nil || string(got) != string(raw) {
		t.Fatalf("got %x, %v; want %x", got, err, raw)
	}

	if _, err := DecodeKey("not-valid-base64!!!"); err == nil {
		t.Fatal("expected an error for invalid base64")
	}
	if _, err := DecodeKey(base64.StdEncoding.EncodeToString([]byte("tooshort"))); err == nil {
		t.Fatal("expected an error for a key that doesn't decode to 32 bytes")
	}
}
