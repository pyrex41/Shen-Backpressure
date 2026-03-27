package auth

import (
	"testing"
	"time"
)

var testSecret = []byte("test-secret-key-for-hmac-256")

func TestSignAndParse(t *testing.T) {
	token, err := NewToken("u-alice", "alice@acme.com", time.Hour, testSecret)
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}

	result, err := Parse(token, testSecret)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if result.Claims.Sub != "u-alice" {
		t.Errorf("Sub = %q, want %q", result.Claims.Sub, "u-alice")
	}
	if result.Claims.Email != "alice@acme.com" {
		t.Errorf("Email = %q, want %q", result.Claims.Email, "alice@acme.com")
	}
	if result.Exp <= result.Now {
		t.Errorf("Exp (%f) should be > Now (%f)", result.Exp, result.Now)
	}
}

func TestParseExpiredToken(t *testing.T) {
	claims := Claims{
		Sub:   "u-bob",
		Email: "bob@globex.com",
		Exp:   time.Now().Add(-time.Hour).Unix(),
		Iat:   time.Now().Add(-2 * time.Hour).Unix(),
	}
	token, err := Sign(claims, testSecret)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	_, err = Parse(token, testSecret)
	if err != ErrExpiredToken {
		t.Errorf("got %v, want ErrExpiredToken", err)
	}
}

func TestParseWrongSecret(t *testing.T) {
	token, err := NewToken("u-alice", "alice@acme.com", time.Hour, testSecret)
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}

	_, err = Parse(token, []byte("wrong-secret"))
	if err != ErrInvalidSignature {
		t.Errorf("got %v, want ErrInvalidSignature", err)
	}
}

func TestParseMalformedToken(t *testing.T) {
	cases := []string{
		"",
		"not-a-jwt",
		"two.parts",
		"three.parts.butgarbage!!!",
	}
	for _, tc := range cases {
		_, err := Parse(tc, testSecret)
		if err == nil {
			t.Errorf("Parse(%q) = nil error, want error", tc)
		}
	}
}

func TestParseTamperedPayload(t *testing.T) {
	token, err := NewToken("u-alice", "alice@acme.com", time.Hour, testSecret)
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}

	// Tamper with the payload (second segment)
	parts := splitToken(token)
	parts[1] = base64URLEncode([]byte(`{"sub":"u-evil","email":"evil@evil.com","exp":9999999999,"iat":0}`))
	tampered := parts[0] + "." + parts[1] + "." + parts[2]

	_, err = Parse(tampered, testSecret)
	if err != ErrInvalidSignature {
		t.Errorf("got %v, want ErrInvalidSignature", err)
	}
}

func splitToken(t string) [3]string {
	var parts [3]string
	i := 0
	for j, b := range t {
		if b == '.' {
			i++
			continue
		}
		parts[i] += string(t[j])
		_ = b
	}
	return parts
}
