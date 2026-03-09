package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gaskaj/OpenAgentFramework/web/auth"
	"github.com/gaskaj/OpenAgentFramework/web/store"
)

type contextKey string

const (
	userContextKey contextKey = "user"
	orgContextKey  contextKey = "org"
)

// AuthContext holds the authenticated user info extracted from JWT.
type AuthContext struct {
	UserID uuid.UUID
	Email  string
	Orgs   []auth.OrgClaim
}

// OrgContext holds the resolved organization info.
type OrgContext struct {
	OrgID     uuid.UUID
	Slug      string
	Role      string
	AgentName string
	AgentType string
}

// RequireAuth is middleware that validates JWT tokens from the Authorization header.
// It also supports a "token" query parameter as a fallback for WebSocket connections
// where browsers cannot set custom headers.
func RequireAuth(jwtMgr *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenStr string

			// Try Authorization header first
			header := r.Header.Get("Authorization")
			if header != "" {
				parts := strings.SplitN(header, " ", 2)
				if len(parts) == 2 && parts[0] == "Bearer" {
					tokenStr = parts[1]
				}
			}

			// Fallback to query parameter (for WebSocket connections)
			if tokenStr == "" {
				tokenStr = r.URL.Query().Get("token")
			}

			if tokenStr == "" {
				http.Error(w, `{"error":"missing authorization"}`, http.StatusUnauthorized)
				return
			}

			claims, err := jwtMgr.ValidateToken(tokenStr)
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userContextKey, &AuthContext{
				UserID: claims.UserID,
				Email:  claims.Email,
				Orgs:   claims.Orgs,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireOrgAccess resolves the org from URL and validates membership.
func RequireOrgAccess(orgStore *store.PgOrgStore, roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCtx := GetUserFromContext(r.Context())
			if authCtx == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			orgSlug := chi.URLParam(r, "orgSlug")
			if orgSlug == "" {
				http.Error(w, `{"error":"missing org slug"}`, http.StatusBadRequest)
				return
			}

			org, err := orgStore.GetBySlug(r.Context(), orgSlug)
			if err != nil || org == nil {
				http.Error(w, `{"error":"organization not found"}`, http.StatusNotFound)
				return
			}

			// Check membership from JWT claims
			var userRole string
			for _, orgClaim := range authCtx.Orgs {
				if orgClaim.ID == org.ID {
					userRole = orgClaim.Role
					break
				}
			}

			if userRole == "" {
				// Fallback: check database
				member, err := orgStore.GetMembership(r.Context(), org.ID, authCtx.UserID)
				if err != nil || member == nil {
					http.Error(w, `{"error":"not a member of this organization"}`, http.StatusForbidden)
					return
				}
				userRole = member.Role
			}

			if len(roles) > 0 {
				allowed := false
				for _, r := range roles {
					if r == userRole {
						allowed = true
						break
					}
				}
				if !allowed {
					http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
					return
				}
			}

			ctx := context.WithValue(r.Context(), orgContextKey, &OrgContext{
				OrgID: org.ID,
				Slug:  org.Slug,
				Role:  userRole,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserFromContext extracts the AuthContext from the request context.
func GetUserFromContext(ctx context.Context) *AuthContext {
	val, _ := ctx.Value(userContextKey).(*AuthContext)
	return val
}

// GetOrgFromContext extracts the OrgContext from the request context.
func GetOrgFromContext(ctx context.Context) *OrgContext {
	val, _ := ctx.Value(orgContextKey).(*OrgContext)
	return val
}
