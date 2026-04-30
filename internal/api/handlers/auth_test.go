package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rossbrandon/minimovie-api/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExchangeToken_Valid(t *testing.T) {
	td := newTestHandlers(t)
	td.authCodeStore.authCode = &store.AuthCode{
		ID:              "code-123",
		TokenHash:       "hash",
		UserID:          "user-1",
		RawSessionToken: "raw-session-token-abc",
		ExpiresAt:       time.Now().Add(time.Minute),
	}

	body := strings.NewReader(`{"code":"code-123"}`)
	r := httptest.NewRequest(http.MethodPost, "/auth/token", body)
	w := httptest.NewRecorder()

	td.handlers.ExchangeToken(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp tokenResponse
	decodeJSON(t, w, &resp)
	assert.Equal(t, "raw-session-token-abc", resp.SessionToken)
}

func TestExchangeToken_MissingCode(t *testing.T) {
	td := newTestHandlers(t)

	body := strings.NewReader(`{"code":""}`)
	r := httptest.NewRequest(http.MethodPost, "/auth/token", body)
	w := httptest.NewRecorder()

	td.handlers.ExchangeToken(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExchangeToken_InvalidCode(t *testing.T) {
	td := newTestHandlers(t)
	td.authCodeStore.authCode = nil

	body := strings.NewReader(`{"code":"bad-code"}`)
	r := httptest.NewRequest(http.MethodPost, "/auth/token", body)
	w := httptest.NewRecorder()

	td.handlers.ExchangeToken(w, r)

	assert.Equal(t, http.StatusGone, w.Code)
}

func TestGetSession_Valid(t *testing.T) {
	td := newTestHandlers(t)
	td.achievementStore.unseenCount = 2

	username := "filmfan"
	avatar := "https://example.com/avatar.png"
	r := authedRequest(t, http.MethodGet, "/auth/session", nil)
	// Override user in context with username/avatar
	r = r.WithContext(setTestUser(r.Context(), &store.User{
		ID:        "u-1",
		Username:  &username,
		AvatarURL: &avatar,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))
	w := httptest.NewRecorder()

	td.handlers.GetSession(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	decodeJSON(t, w, &resp)

	user := resp["user"].(map[string]any)
	assert.Equal(t, "u-1", user["id"])
	assert.Equal(t, "filmfan", user["username"])
	assert.Equal(t, "https://example.com/avatar.png", user["avatarUrl"])
	assert.Equal(t, float64(2), resp["unseenAchievementCount"])
}

func TestLogout_Success(t *testing.T) {
	td := newTestHandlers(t)

	r := authedRequest(t, http.MethodPost, "/auth/logout", nil)
	w := httptest.NewRecorder()

	td.handlers.Logout(w, r)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.True(t, td.sessionStore.deleteCalled)
}
