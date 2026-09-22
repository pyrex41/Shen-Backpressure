package auth

import (
	"context"
	"net/http"
	"strings"

	"multi-tenant-api/internal/apibrand"
	"multi-tenant-api/internal/shenguard"
)

type contextKey string

const authUserKey contextKey = "authenticated-user"

// Middleware extracts a Bearer JWT from the Authorization header,
// validates it, and constructs an AuthenticatedUser guard type.
//
// The proof chain (post-W2.1):
//
//	raw token
//	  → auth.Parse (HMAC-SHA256 + JSON-decode + expiry; TCB)
//	  → shenguard.NewJwtIssuer / NewJwtAudience (non-empty)
//	  → shenguard.NewParsedClaims (Exp > 0, types enforced)
//	  → shenguard.NewVerifiedJwt   (non-empty signature)
//	  → shenguard.NewAuthenticatedUser (User == sub(Claims) — STRUCTURAL)
//	  → shenguard.NewHumanPrincipal
//
// The structural premise `(= User (head (head Jwt)))` in
// specs/core.shen is what makes the user-id–token binding type-enforced
// instead of convention. If a refactor accidentally threaded a
// mismatched UserId into NewAuthenticatedUser, the constructor returns
// an error and the middleware returns 401.
func Middleware(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := extractBearer(r)
			if raw == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			// Parse and validate the JWT (HMAC verify + expiry check).
			// `result.Signature` is the raw signature segment after a
			// successful constant-time hmac.Equal check — see jwt.go.
			result, err := Parse(raw, secret)
			if err != nil {
				http.Error(w, "invalid token: "+err.Error(), http.StatusUnauthorized)
				return
			}

			principal, err := buildPrincipal(result)
			if err != nil {
				http.Error(w, "invalid token: "+err.Error(), http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), authUserKey, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// buildPrincipal threads the parsed claims through the shenguard
// constructor chain. Every constructor that returns an error here is
// part of the type-system anchor: bypassing any of them is a
// compile-time error in handler code (the parameter types demand
// shenguard types, not raw strings).
// The brand is apibrand.API: the principal leaves this function
// through context.Value, which erases type parameters, so the HTTP
// boundary has to name a concrete brand. See internal/apibrand.
func buildPrincipal(result ParseResult) (shenguard.HumanPrincipal[apibrand.API], error) {
	iss, err := shenguard.NewJwtIssuer(result.Claims.Iss)
	if err != nil {
		return shenguard.HumanPrincipal[apibrand.API]{}, err
	}
	aud, err := shenguard.NewJwtAudience(result.Claims.Aud)
	if err != nil {
		return shenguard.HumanPrincipal[apibrand.API]{}, err
	}
	userID := shenguard.NewUserId(result.Claims.Sub)

	claims, err := shenguard.NewParsedClaims[apibrand.API](userID, result.Exp, iss, aud)
	if err != nil {
		return shenguard.HumanPrincipal[apibrand.API]{}, err
	}
	verifiedJwt, err := shenguard.NewVerifiedJwt(claims, result.Signature)
	if err != nil {
		return shenguard.HumanPrincipal[apibrand.API]{}, err
	}

	// W2.1 hardening: the structural premise (= User (head (head Jwt)))
	// is enforced inside NewAuthenticatedUser. We thread the SAME UserId
	// value through both parsed-claims (as `sub`) and authenticated-user
	// (as `user`) — the constructor returns an error if they ever
	// disagree. A handler that pairs token-A with user-B literally
	// cannot construct an AuthenticatedUser.
	authUser, err := shenguard.NewAuthenticatedUser(verifiedJwt, userID)
	if err != nil {
		return shenguard.HumanPrincipal[apibrand.API]{}, err
	}
	return shenguard.NewHumanPrincipal(authUser), nil
}

// PrincipalFromContext retrieves the AuthenticatedPrincipal from the request context.
// Returns nil and false if not present.
func PrincipalFromContext(ctx context.Context) (shenguard.AuthenticatedPrincipal[apibrand.API], bool) {
	u, ok := ctx.Value(authUserKey).(shenguard.AuthenticatedPrincipal[apibrand.API])
	return u, ok
}

// HumanFromContext retrieves the HumanPrincipal from the request context.
// Returns the zero value and false if the principal is not a HumanPrincipal.
func HumanFromContext(ctx context.Context) (shenguard.HumanPrincipal[apibrand.API], bool) {
	u, ok := ctx.Value(authUserKey).(shenguard.HumanPrincipal[apibrand.API])
	return u, ok
}

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}
