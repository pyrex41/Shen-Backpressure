package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"multi-tenant-api/internal/shenguard"
)

func TestMiddlewareValidToken(t *testing.T) {
	token, err := NewToken("u-alice", "alice@acme.com", time.Hour, testSecret)
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}

	var gotHuman shenguard.HumanPrincipal
	var gotOK bool

	handler := Middleware(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHuman, gotOK = HumanFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !gotOK {
		t.Fatal("HumanPrincipal not found in context")
	}
	if gotHuman.Auth().User().Val() != "u-alice" {
		t.Errorf("user = %q, want %q", gotHuman.Auth().User().Val(), "u-alice")
	}
}

func TestMiddlewareMissingHeader(t *testing.T) {
	handler := Middleware(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMiddlewareExpiredToken(t *testing.T) {
	claims := Claims{
		Sub:   "u-bob",
		Email: "bob@globex.com",
		Exp:   time.Now().Add(-time.Hour).Unix(),
		Iat:   time.Now().Add(-2 * time.Hour).Unix(),
	}
	token, _ := Sign(claims, testSecret)

	handler := Middleware(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMiddlewareInvalidSignature(t *testing.T) {
	token, _ := NewToken("u-alice", "alice@acme.com", time.Hour, []byte("other-secret"))

	handler := Middleware(testSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
