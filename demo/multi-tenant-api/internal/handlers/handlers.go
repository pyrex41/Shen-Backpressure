package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"multi-tenant-api/internal/auth"
	"multi-tenant-api/internal/shenguard"
)

type Server struct {
	DB     *sql.DB
	Secret []byte
}

// Register sets up all routes on the given mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/login", s.handleLogin)

	authMW := auth.Middleware(s.Secret)
	mux.Handle("GET /tenants/{tid}/resources", authMW(http.HandlerFunc(s.handleListResources)))
	mux.Handle("GET /tenants/{tid}/resources/{rid}", authMW(http.HandlerFunc(s.handleGetResource)))
	mux.Handle("POST /tenants/{tid}/resources", authMW(http.HandlerFunc(s.handleCreateResource)))

	// Admin dashboard (no auth — internal/demo use)
	mux.HandleFunc("GET /admin", s.handleAdmin)
	mux.HandleFunc("GET /admin/tenants/{tid}/resources", s.handleAdminTenantResources)
	mux.HandleFunc("GET /admin/logs", s.handleAdminLogsPartial)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var userID, storedHash string
	err := s.DB.QueryRow(
		"SELECT id, password_hash FROM users WHERE email = ?", req.Email,
	).Scan(&userID, &storedHash)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	// Seed data uses $plain$ prefix for demo passwords
	expected := "$plain$" + req.Password
	if storedHash != expected {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	tokenStr, err := auth.NewToken(userID, req.Email, 24*time.Hour, s.Secret)
	if err != nil {
		http.Error(w, "failed to issue token", http.StatusInternalServerError)
		return
	}

	_ = auth.LogAccess(s.DB, userID, "", "", "login", true)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token":   tokenStr,
		"user_id": userID,
	})
}

func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request) {
	authUser, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	tenantID := shenguard.NewTenantId(r.PathValue("tid"))
	access, err := auth.CheckTenantAccess(s.DB, authUser, tenantID)
	if err != nil {
		_ = auth.LogAccess(s.DB, authUser.User().Val(), tenantID.Val(), "", "list_resources", false)
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	_ = auth.LogAccess(s.DB, authUser.User().Val(), access.Tenant().Val(), "", "list_resources", true)

	rows, err := s.DB.Query(
		"SELECT id, title, body, created_at FROM resources WHERE tenant_id = ?",
		access.Tenant().Val(),
	)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type resource struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
	}
	var resources []resource
	for rows.Next() {
		var res resource
		if err := rows.Scan(&res.ID, &res.Title, &res.Body, &res.CreatedAt); err != nil {
			http.Error(w, "scan failed", http.StatusInternalServerError)
			return
		}
		resources = append(resources, res)
	}
	if resources == nil {
		resources = []resource{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resources)
}

func (s *Server) handleGetResource(w http.ResponseWriter, r *http.Request) {
	authUser, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	tenantID := shenguard.NewTenantId(r.PathValue("tid"))
	access, err := auth.CheckTenantAccess(s.DB, authUser, tenantID)
	if err != nil {
		_ = auth.LogAccess(s.DB, authUser.User().Val(), tenantID.Val(), r.PathValue("rid"), "get_resource", false)
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	resourceID := shenguard.NewResourceId(r.PathValue("rid"))
	ra, err := auth.CheckResourceAccess(s.DB, access, resourceID)
	if err != nil {
		_ = auth.LogAccess(s.DB, authUser.User().Val(), access.Tenant().Val(), resourceID.Val(), "get_resource", false)
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	_ = auth.LogAccess(s.DB, authUser.User().Val(), ra.Access().Tenant().Val(), ra.Resource().Val(), "get_resource", true)

	var title, body, createdAt string
	err = s.DB.QueryRow(
		"SELECT title, body, created_at FROM resources WHERE id = ? AND tenant_id = ?",
		ra.Resource().Val(), ra.Access().Tenant().Val(),
	).Scan(&title, &body, &createdAt)
	if err != nil {
		http.Error(w, "resource not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"id":         ra.Resource().Val(),
		"tenant_id":  ra.Access().Tenant().Val(),
		"title":      title,
		"body":       body,
		"created_at": createdAt,
	})
}

func (s *Server) handleCreateResource(w http.ResponseWriter, r *http.Request) {
	authUser, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	tenantID := shenguard.NewTenantId(r.PathValue("tid"))
	access, err := auth.CheckTenantAccess(s.DB, authUser, tenantID)
	if err != nil {
		_ = auth.LogAccess(s.DB, authUser.User().Val(), tenantID.Val(), "", "create_resource", false)
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	var req struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}

	id := "r-" + randomHex(8)
	_, err = s.DB.Exec(
		"INSERT INTO resources (id, tenant_id, title, body) VALUES (?, ?, ?, ?)",
		id, access.Tenant().Val(), req.Title, req.Body,
	)
	if err != nil {
		http.Error(w, "failed to create resource", http.StatusInternalServerError)
		return
	}

	_ = auth.LogAccess(s.DB, authUser.User().Val(), access.Tenant().Val(), id, "create_resource", true)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"id":        id,
		"tenant_id": access.Tenant().Val(),
		"title":     req.Title,
		"body":      req.Body,
	})
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
