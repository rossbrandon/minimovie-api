package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/jwtauth/v5"
	"github.com/rossbrandon/minimovie-api/internal/auth"
	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

type fakeSessionStore struct {
	session *store.SessionWithUser
	err     error
}

func (f *fakeSessionStore) GetByTokenHash(ctx context.Context, tokenHash string) (*store.SessionWithUser, error) {
	return f.session, f.err
}

func (f *fakeSessionStore) Create(ctx context.Context, tokenHash, userID string) (*store.Session, error) {
	return nil, nil
}

func (f *fakeSessionStore) Delete(ctx context.Context, tokenHash string) error { return nil }

func (f *fakeSessionStore) DeleteByUserID(ctx context.Context, userID string) error { return nil }

func guestCatalogAuth(secret string, next http.Handler) http.Handler {
	ta := jwtauth.New("HS256", []byte(secret), nil)
	return jwtauth.Verifier(ta)(jwtauth.Authenticator(ta)(next))
}

func TestGuestCatalogJWT_ValidToken(t *testing.T) {
	secret := "test-guest-key-abc123"
	ta := jwtauth.New("HS256", []byte(secret), nil)
	_, tokenStr, err := ta.Encode(map[string]interface{}{})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	guestCatalogAuth(secret, okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestGuestCatalogJWT_MissingAuth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	guestCatalogAuth("secret", okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestGuestCatalogJWT_WrongToken(t *testing.T) {
	goodSecret := "correct-secret"
	wrongTA := jwtauth.New("HS256", []byte("other-signing-key"), nil)
	_, wrongTok, err := wrongTA.Encode(map[string]interface{}{})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+wrongTok)
	rec := httptest.NewRecorder()

	guestCatalogAuth(goodSecret, okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestGuestCatalogJWT_NoBearerPrefix(t *testing.T) {
	secret := "test-secret"
	ta := jwtauth.New("HS256", []byte(secret), nil)
	_, tokenStr, err := ta.Encode(map[string]interface{}{})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", tokenStr)
	rec := httptest.NewRecorder()

	guestCatalogAuth(secret, okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ---------- RequireSession ----------

var testSessionSecret = []byte("01234567890123456789012345678901")

func TestRequireSession_ValidSession(t *testing.T) {
	token := "raw-session-token"
	expectedHash := auth.HashToken(token, testSessionSecret)

	username := "testuser"
	sw := &store.SessionWithUser{
		Session: store.Session{
			ID:        "sess-1",
			TokenHash: expectedHash,
			UserID:    "user-1",
			ExpiresAt: time.Now().Add(24 * time.Hour),
			CreatedAt: time.Now(),
		},
		User: store.User{
			ID:        "user-1",
			Username:  &username,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	fake := &fakeSessionStore{session: sw}
	mw := RequireSession(fake, testSessionSecret, "mm_session_dev")

	var capturedUser *store.User
	var capturedHash string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser = GetUser(r.Context())
		capturedHash = GetSessionHash(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "mm_session_dev", Value: token})
	rec := httptest.NewRecorder()

	mw(inner).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, capturedUser)
	assert.Equal(t, "user-1", capturedUser.ID)
	assert.Equal(t, expectedHash, capturedHash)
}

func TestRequireSession_MissingCookie(t *testing.T) {
	fake := &fakeSessionStore{}
	mw := RequireSession(fake, testSessionSecret, "mm_session_dev")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireSession_ExpiredSession(t *testing.T) {
	fake := &fakeSessionStore{session: nil, err: nil}
	mw := RequireSession(fake, testSessionSecret, "mm_session_dev")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "mm_session_dev", Value: "some-token"})
	rec := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	var body map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	assert.Contains(t, body["message"], "invalid or expired session")
}

func TestRequireSession_StoreError(t *testing.T) {
	fake := &fakeSessionStore{session: nil, err: errors.New("db connection lost")}
	mw := RequireSession(fake, testSessionSecret, "mm_session_dev")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "mm_session_dev", Value: "some-token"})
	rec := httptest.NewRecorder()

	mw(okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ---------- NoStoreCache ----------

func TestNoStoreCache(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	NoStoreCache(okHandler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}
