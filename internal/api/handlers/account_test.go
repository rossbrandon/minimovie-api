package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportUserData_Success(t *testing.T) {
	td := newTestHandlers(t)
	td.userStore.canExport = true

	r := authedRequest(t, http.MethodGet, "/account/export", nil)
	w := httptest.NewRecorder()

	td.handlers.ExportUserData(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `attachment; filename="minimovie-data-export.json"`, w.Header().Get("Content-Disposition"))

	var resp map[string]any
	decodeJSON(t, w, &resp)
	assert.Contains(t, resp, "account")
	assert.Contains(t, resp, "watchlist")
	assert.Contains(t, resp, "watchEvents")
	assert.Contains(t, resp, "achievements")
}

func TestExportUserData_RateLimited(t *testing.T) {
	td := newTestHandlers(t)
	td.userStore.canExport = false

	r := authedRequest(t, http.MethodGet, "/account/export", nil)
	w := httptest.NewRecorder()

	td.handlers.ExportUserData(w, r)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestDeleteAccount_Success(t *testing.T) {
	td := newTestHandlers(t)

	body := strings.NewReader(`{"confirm":"delete"}`)
	r := authedRequest(t, http.MethodDelete, "/account", body)
	w := httptest.NewRecorder()

	td.handlers.DeleteAccount(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]bool
	decodeJSON(t, w, &resp)
	assert.True(t, resp["deleted"])
}
