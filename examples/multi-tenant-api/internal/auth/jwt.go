package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidToken   = errors.New("invalid token")
	ErrExpiredToken   = errors.New("token expired")
	ErrInvalidSignature = errors.New("invalid signature")
)

// Claims holds the JWT payload fields. The fields are populated from
// the JSON-decoded JWT payload and are also threaded into the
// shenguard.ParsedClaims composite by the middleware. The spec
// requires `iss` and `aud` to be non-empty (see
// specs/core.shen jwt-issuer / jwt-audience); the constants below
// match the demo defaults baked into `NewToken`.
type Claims struct {
	Sub   string `json:"sub"`   // user ID
	Email string `json:"email"`
	Exp   int64  `json:"exp"`   // unix timestamp
	Iat   int64  `json:"iat"`   // issued at
	Iss   string `json:"iss"`   // issuer — required non-empty by spec
	Aud   string `json:"aud"`   // audience — required non-empty by spec
}

// Default issuer/audience baked into demo tokens. These are part of
// the spec's structural requirement that `parsed-claims` carry a
// non-empty `jwt-issuer` and `jwt-audience` (see specs/core.shen).
// Production deployments set these via configuration.
const (
	DefaultIssuer   = "multi-tenant-api"
	DefaultAudience = "users"
)

var jwtHeader = base64URLEncode([]byte(`{"alg":"HS256","typ":"JWT"}`))

// Sign creates an HMAC-SHA256 JWT for the given claims.
func Sign(claims Claims, secret []byte) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	encodedPayload := base64URLEncode(payload)
	signingInput := jwtHeader + "." + encodedPayload
	sig := hmacSHA256([]byte(signingInput), secret)
	return signingInput + "." + base64URLEncode(sig), nil
}

// ParseResult contains the validated claims and metadata derived from
// the JWT. `Signature` is the raw third segment of the token (already
// HMAC-verified by `Parse`) — the middleware threads it into
// `shenguard.NewVerifiedJwt` so the structural type `VerifiedJwt`
// carries the verified signature alongside the parsed claims.
type ParseResult struct {
	Claims    Claims
	Signature string  // raw signature segment, post-verification
	Exp       float64 // unix seconds, for NewTokenExpiry
	Now       float64
}

// Parse validates a JWT string and returns the claims.
// It checks the signature and expiry. It does NOT construct guard types —
// that is the caller's responsibility.
func Parse(tokenStr string, secret []byte) (ParseResult, error) {
	parts := strings.SplitN(tokenStr, ".", 3)
	if len(parts) != 3 {
		return ParseResult{}, ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	sigBytes, err := base64URLDecode(parts[2])
	if err != nil {
		return ParseResult{}, ErrInvalidToken
	}

	expected := hmacSHA256([]byte(signingInput), secret)
	if !hmac.Equal(sigBytes, expected) {
		return ParseResult{}, ErrInvalidSignature
	}

	payload, err := base64URLDecode(parts[1])
	if err != nil {
		return ParseResult{}, ErrInvalidToken
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ParseResult{}, ErrInvalidToken
	}

	now := float64(time.Now().Unix())
	if float64(claims.Exp) <= now {
		return ParseResult{}, ErrExpiredToken
	}

	return ParseResult{
		Claims:    claims,
		Signature: parts[2],
		Exp:       float64(claims.Exp),
		Now:       now,
	}, nil
}

// NewToken creates a signed JWT for a user with a given TTL.
func NewToken(userID, email string, ttl time.Duration, secret []byte) (string, error) {
	now := time.Now()
	claims := Claims{
		Sub:   userID,
		Email: email,
		Exp:   now.Add(ttl).Unix(),
		Iat:   now.Unix(),
		Iss:   DefaultIssuer,
		Aud:   DefaultAudience,
	}
	return Sign(claims, secret)
}

func hmacSHA256(data, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
