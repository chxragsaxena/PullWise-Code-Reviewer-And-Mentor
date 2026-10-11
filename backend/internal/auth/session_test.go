package auth

import "testing"

func TestNewSessionTokenIsUniqueAndHashMatches(t *testing.T) {
	token1, hash1, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	token2, hash2, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}

	if token1 == token2 || hash1 == hash2 {
		t.Fatal("two calls produced the same token")
	}
	if HashSessionToken(token1) != hash1 {
		t.Fatal("HashSessionToken did not reproduce the token's own hash")
	}
	if HashSessionToken(token2) != hash2 {
		t.Fatal("HashSessionToken did not reproduce the token's own hash")
	}
}

func TestHashSessionTokenIsDeterministic(t *testing.T) {
	if HashSessionToken("x") != HashSessionToken("x") {
		t.Fatal("same input produced different hashes")
	}
	if HashSessionToken("x") == HashSessionToken("y") {
		t.Fatal("different input produced the same hash")
	}
}
