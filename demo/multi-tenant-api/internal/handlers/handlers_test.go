package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"multi-tenant-api/internal/auth"
	"multi-tenant-api/internal/db"
)

var testSecret = []byte("test-secret-key")

// setupTestServer creates a Server with a seeded in-memory SQLite database.
func setupTestServer(t *testing.T) *Server {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Seed(database); err != nil {
		t.Fatalf("seed db: %v", err)
	}
	return &Server{DB: database, Secret: testSecret}
}

func newTestMux(srv *Server) *http.ServeMux {
	mux := http.NewServeMux()
	srv.Register(mux)
	return mux
}

func issueToken(t *testing.T, userID, email string, ttl time.Duration) string {
	t.Helper()
	token, err := auth.NewToken(userID, email, ttl, testSecret)
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	return token
}

// --- Integration tests for the proof chain ---

func TestValidAccessAccepted(t *testing.T) {
	srv := setupTestServer(t)
	mux := newTestMux(srv)

	// Alice is a member of t-acme and should be able to list resources
	token := issueToken(t, "u-alice", "alice@acme.com", time.Hour)

	req := httptest.NewRequest("GET", "/tenants/t-acme/resources", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resources []map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resources); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resources) != 2 {
		t.Errorf("got %d resources, want 2 (Acme has r-1 and r-2)", len(resources))
	}
}

func TestValidResourceAccessAccepted(t *testing.T) {
	srv := setupTestServer(t)
	mux := newTestMux(srv)

	// Alice can access r-1 which belongs to t-acme
	token := issueToken(t, "u-alice", "alice@acme.com", time.Hour)

	req := httptest.NewRequest("GET", "/tenants/t-acme/resources/r-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resource map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resource); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resource["id"] != "r-1" {
		t.Errorf("resource id = %q, want %q", resource["id"], "r-1")
	}
	if resource["tenant_id"] != "t-acme" {
		t.Errorf("tenant_id = %q, want %q", resource["tenant_id"], "t-acme")
	}
}

func TestCrossTenantAccessRejected(t *testing.T) {
	srv := setupTestServer(t)
	mux := newTestMux(srv)

	// Alice is a member of t-acme but NOT t-globex
	token := issueToken(t, "u-alice", "alice@acme.com", time.Hour)

	req := httptest.NewRequest("GET", "/tenants/t-globex/resources", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCrossTenantResourceAccessRejected(t *testing.T) {
	srv := setupTestServer(t)
	mux := newTestMux(srv)

	// Bob is a member of t-globex. r-1 belongs to t-acme.
	// Even if Bob tries to access r-1 through t-globex, it should fail
	// because the resource is not owned by that tenant.
	token := issueToken(t, "u-bob", "bob@globex.com", time.Hour)

	req := httptest.NewRequest("GET", "/tenants/t-globex/resources/r-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body: %s", rec.Code, rec.Body.String())
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	srv := setupTestServer(t)
	mux := newTestMux(srv)

	// Create a token that expired an hour ago
	claims := auth.Claims{
		Sub:   "u-alice",
		Email: "alice@acme.com",
		Exp:   time.Now().Add(-time.Hour).Unix(),
		Iat:   time.Now().Add(-2 * time.Hour).Unix(),
	}
	token, err := auth.Sign(claims, testSecret)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	req := httptest.NewRequest("GET", "/tenants/t-acme/resources", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401; body: %s", rec.Code, rec.Body.String())
	}
}

func TestNonMemberRejected(t *testing.T) {
	srv := setupTestServer(t)
	mux := newTestMux(srv)

	// Bob is a member of t-globex but NOT t-acme
	token := issueToken(t, "u-bob", "bob@globex.com", time.Hour)

	req := httptest.NewRequest("GET", "/tenants/t-acme/resources", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body: %s", rec.Code, rec.Body.String())
	}
}

func TestMissingTokenRejected(t *testing.T) {
	srv := setupTestServer(t)
	mux := newTestMux(srv)

	// No Authorization header at all
	req := httptest.NewRequest("GET", "/tenants/t-acme/resources", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401; body: %s", rec.Code, rec.Body.String())
	}
}

func TestInvalidSignatureRejected(t *testing.T) {
	srv := setupTestServer(t)
	mux := newTestMux(srv)

	// Token signed with a different secret
	token, err := auth.NewToken("u-alice", "alice@acme.com", time.Hour, []byte("wrong-secret"))
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}

	req := httptest.NewRequest("GET", "/tenants/t-acme/resources", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateResourceRequiresTenantAccess(t *testing.T) {
	srv := setupTestServer(t)
	mux := newTestMux(srv)

	// Bob (t-globex member) cannot create resources in t-acme
	token := issueToken(t, "u-bob", "bob@globex.com", time.Hour)

	body := `{"title":"Sneaky Doc","body":"Should not be created"}`
	req := httptest.NewRequest("POST", "/tenants/t-acme/resources", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateResourceValidAccess(t *testing.T) {
	srv := setupTestServer(t)
	mux := newTestMux(srv)

	// Alice (t-acme member) can create resources in t-acme
	token := issueToken(t, "u-alice", "alice@acme.com", time.Hour)

	body := `{"title":"New Doc","body":"Test content"}`
	req := httptest.NewRequest("POST", "/tenants/t-acme/resources", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}

	var created map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created["tenant_id"] != "t-acme" {
		t.Errorf("tenant_id = %q, want %q", created["tenant_id"], "t-acme")
	}
	if created["title"] != "New Doc" {
		t.Errorf("title = %q, want %q", created["title"], "New Doc")
	}
}

func TestLoginAndUseToken(t *testing.T) {
	srv := setupTestServer(t)
	mux := newTestMux(srv)

	// Login as Alice
	loginBody := `{"email":"alice@acme.com","password":"alice123"}`
	loginReq := httptest.NewRequest("POST", "/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200; body: %s", loginRec.Code, loginRec.Body.String())
	}

	var loginResp map[string]string
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	token := loginResp["token"]
	if token == "" {
		t.Fatal("login returned empty token")
	}

	// Use the token to access resources
	req := httptest.NewRequest("GET", "/tenants/t-acme/resources", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("resource list status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}
