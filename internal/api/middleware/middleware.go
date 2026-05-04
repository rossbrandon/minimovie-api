package middleware

import (
	"context"
	"net/http"

	"github.com/rossbrandon/minimovie-api/internal/auth"
	"github.com/rossbrandon/minimovie-api/internal/httputil"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/rs/zerolog/log"
)

type contextKey string

const UserContextKey contextKey = "user"
const SessionHashContextKey contextKey = "session_hash"

func GetUser(ctx context.Context) *store.User {
	u, _ := ctx.Value(UserContextKey).(*store.User)
	return u
}

func GetSessionHash(ctx context.Context) string {
	h, _ := ctx.Value(SessionHashContextKey).(string)
	return h
}

func RequireSession(sessionStore store.SessionRepository, sessionSecret []byte, cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractSessionCookie(r, cookieName)
			if token == "" {
				httputil.Error(w, http.StatusUnauthorized, "missing session")
				return
			}

			tokenHash := auth.HashToken(token, sessionSecret)
			sw, err := sessionStore.GetByTokenHash(r.Context(), tokenHash)
			if err != nil {
				log.Error().Err(err).Msg("session lookup error")
				httputil.Error(w, http.StatusInternalServerError, "session lookup failed")
				return
			}
			if sw == nil {
				httputil.Error(w, http.StatusUnauthorized, "invalid or expired session")
				return
			}

			ctx := context.WithValue(r.Context(), UserContextKey, &sw.User)
			ctx = context.WithValue(ctx, SessionHashContextKey, tokenHash)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func NoStoreCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// RequireOrigin rejects mutating requests whose Origin header is missing or
// not in the allowlist.
// This is Defense in depth on top of SameSite=Lax and protects
// against requests with forged credentials from origins we don't control.
// Safe methods (GET/HEAD/OPTIONS) pass through unchanged for read requests.
// CORS preflight already enforces origin separately.
func RequireOrigin(allowed []string) func(http.Handler) http.Handler {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		allowedSet[o] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			origin := r.Header.Get("Origin")
			if origin == "" {
				httputil.Error(w, http.StatusForbidden, "missing origin header")
				return
			}
			if _, ok := allowedSet[origin]; !ok {
				httputil.Error(w, http.StatusForbidden, "origin not allowed")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func extractSessionCookie(r *http.Request, name string) string {
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}
