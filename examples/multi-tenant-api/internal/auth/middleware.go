package auth

import (
	"context"
	"net/http"
	"strings"

	"multi-tenant-api/internal/shenguard"
)

type contextKey string

const authUserKey contextKey = "authenticated-user"

// Middleware extracts a Bearer JWT from the Authorization header,
// validates it, and constructs an AuthenticatedUser guard type.
// The proof chain is: raw token → JwtToken → TokenExpiry → AuthenticatedUser.
func Middleware(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := extractBearer(r)
			if raw == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			// Parse and validate the JWT (checks signature + expiry)
			result, err := Parse(raw, secret)
			if err != nil {
				http.Error(w, "invalid token: "+err.Error(), http.StatusUnauthorized)
				return
			}

			// Build proof chain using guard type constructors
			jwtToken, err := shenguard.NewJwtToken(raw)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			expiry, err := shenguard.NewTokenExpiry(result.Exp, result.Now)
			if err != nil {
				http.Error(w, "token expired", http.StatusUnauthorized)
				return
			}

			userID := shenguard.NewUserId(result.Claims.Sub)
			authUser := shenguard.NewAuthenticatedUser(jwtToken, expiry, userID)
			principal := shenguard.NewHumanPrincipal(authUser)

			ctx := context.WithValue(r.Context(), authUserKey, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// PrincipalFromContext retrieves the AuthenticatedPrincipal from the request context.
// Returns nil and false if not present.
func PrincipalFromContext(ctx context.Context) (shenguard.AuthenticatedPrincipal, bool) {
	u, ok := ctx.Value(authUserKey).(shenguard.AuthenticatedPrincipal)
	return u, ok
}

// HumanFromContext retrieves the HumanPrincipal from the request context.
// Returns the zero value and false if the principal is not a HumanPrincipal.
func HumanFromContext(ctx context.Context) (shenguard.HumanPrincipal, bool) {
	u, ok := ctx.Value(authUserKey).(shenguard.HumanPrincipal)
	return u, ok
}

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}
